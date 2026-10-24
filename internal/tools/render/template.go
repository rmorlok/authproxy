package render

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Template is an immutable Tool-only Mustache program. Compilation rejects
// partials and malformed structure without evaluating invocation data. Unlike
// connector templates, interpolations never perform HTML escaping and every
// evaluated reference must exist. A Template may be reused concurrently.
type Template struct {
	nodes  []templateNode
	limits Limits
}

// templateNode retains literal text or a lookup with optional section children.
// Raw, triple-brace, and ordinary variables share the same unescaped behavior.
type templateNode struct {
	kind     byte
	text     string
	children []templateNode
}

// templateParser tracks delimiters across the entire source, including section
// boundaries. It performs no file access and retains only bounded syntax trees.
type templateParser struct {
	source      string
	position    int
	open, close string
	limits      Limits
	steps       int
}

// Compile validates and parses Tool Mustache syntax under explicit limits.
// Sections, inverted sections, comments, dotted names, custom delimiters, and
// unescaped variables are supported. Partials and inheritance are rejected even
// inside sections that would not render. Errors contain no invocation values.
func Compile(source string, limits Limits) (*Template, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if len(source) > limits.MaxBytes {
		return nil, errors.New("template source exceeds byte limit")
	}

	if !utf8.ValidString(source) {
		return nil, errors.New("template source contains invalid UTF-8")
	}
	p := templateParser{
		source: source,
		open:   "{{",
		close:  "}}",
		limits: limits,
	}

	nodes, err := p.parse("", 0)
	if err != nil {
		return nil, err
	}

	return &Template{
		nodes:  nodes,
		limits: limits,
	}, nil
}

// parse reads one section and consumes its matching closing tag. The enclosing
// section name is empty only at the root; depth counts open section frames.
func (p *templateParser) parse(section string, depth int) ([]templateNode, error) {
	var nodes []templateNode

	for p.position < len(p.source) {
		relative := strings.Index(p.source[p.position:], p.open)
		if relative < 0 {
			nodes = append(nodes, templateNode{text: p.source[p.position:]})
			p.position = len(p.source)
			break
		}

		start := p.position + relative
		contentStart, close := start+len(p.open), p.close

		triple := strings.HasPrefix(p.source[contentStart:], "{")
		if triple {
			contentStart++
			close = "}" + close
		}

		closing := strings.Index(p.source[contentStart:], close)
		if closing < 0 {
			return nil, errors.New("unclosed template tag")
		}

		end := contentStart + closing + len(close)
		content := strings.TrimSpace(p.source[contentStart : contentStart+closing])
		p.steps++

		if p.steps > p.limits.MaxSteps {
			return nil, errors.New("template syntax exceeds step limit")
		}

		if content == "" {
			return nil, errors.New("template reference must not be empty")
		}

		kind := byte('v')

		if !triple && strings.ContainsRune("#^/!&=><$", rune(content[0])) {
			kind, content = content[0], strings.TrimSpace(content[1:])
		}

		if kind == '>' || kind == '<' || kind == '$' {
			return nil, errors.New("template partials and inheritance are not supported")
		}

		textEnd, next := start, end
		if strings.ContainsRune("#^/!=", rune(kind)) {
			textEnd, next = p.standaloneBounds(start, end)
		}

		if textEnd > p.position {
			nodes = append(nodes, templateNode{text: p.source[p.position:textEnd]})
		}
		p.position = next

		switch kind {
		case '!':
			continue
		case '=':
			if err := p.setDelimiters(content); err != nil {
				return nil, err
			}
		case '/':
			if section == "" || content != section {
				return nil, errors.New("template section closing tag does not match")
			}
			return nodes, nil
		default:
			if content == "" {
				return nil, errors.New("template reference must not be empty")
			}
			node := templateNode{kind: kind, text: content}
			if kind == '#' || kind == '^' {
				if depth >= p.limits.MaxDepth {
					return nil, errors.New("template sections exceed depth limit")
				}
				var err error
				node.children, err = p.parse(content, depth+1)
				if err != nil {
					return nil, err
				}
			}
			nodes = append(nodes, node)
		}
	}

	if section != "" {
		return nil, errors.New("unclosed template section")
	}

	return nodes, nil
}

// standaloneBounds removes an otherwise blank line around a structural tag,
// including CRLF and the final line without a newline, as Mustache requires.
// It scans only neighboring whitespace, so adjacent inline tags cannot cause
// repeated scans of an increasingly long line prefix or suffix.
func (p *templateParser) standaloneBounds(start, end int) (int, int) {
	lineStart := start
	for lineStart > 0 && (p.source[lineStart-1] == ' ' || p.source[lineStart-1] == '\t') {
		lineStart--
	}

	if lineStart > 0 && p.source[lineStart-1] != '\n' {
		return start, end
	}
	next := end

	for next < len(p.source) && (p.source[next] == ' ' || p.source[next] == '\t') {
		next++
	}

	if next < len(p.source) && p.source[next] == '\r' {
		next++
	}

	if next == len(p.source) {
		return lineStart, next
	}

	if p.source[next] == '\n' {
		return lineStart, next + 1
	}

	return start, end
}

// setDelimiters accepts the two nonempty delimiters in an equals-delimited tag.
// Equal signs and whitespace cannot occur inside a delimiter.
func (p *templateParser) setDelimiters(content string) error {
	if !strings.HasSuffix(content, "=") {
		return errors.New("malformed template delimiter change")
	}

	parts := strings.Fields(strings.TrimSuffix(content, "="))
	if len(parts) != 2 ||
		strings.Contains(parts[0], "=") ||
		strings.Contains(parts[1], "=") {
		return errors.New("template delimiter change requires two delimiters without equals signs")
	}

	p.open, p.close = parts[0], parts[1]

	return nil
}

// Render evaluates a compiled template against a detached invocation Context.
// Cancellation and output/work limits return an error without partial output.
func (t *Template) Render(ctx context.Context, data *Context) (string, error) {
	if t == nil || data == nil || ctx == nil {
		return "", errors.New("template, context, and invocation data are required")
	}

	return t.renderWithBudget(data, newBudget(ctx, t.limits))
}

// renderWithBudget lets structured value rendering share its cumulative limits
// across every embedded string. It charges bytes before writing them.
func (t *Template) renderWithBudget(data *Context, b *budget) (string, error) {
	if t == nil || data == nil {
		return "", errors.New("template and invocation data are required")
	}

	if err := b.step(); err != nil {
		return "", err
	}

	var output strings.Builder
	if err := renderTemplateNodes(t.nodes, data, []any{data.root}, b, &output); err != nil {
		return "", err
	}

	return output.String(), nil
}

// renderTemplateNodes evaluates one scope without exposing JSON values in
// errors. Only explicit encoding views may interpolate objects or arrays.
func renderTemplateNodes(
	nodes []templateNode,
	data *Context,
	scopes []any,
	b *budget,
	output *strings.Builder,
) error {
	for _, node := range nodes {
		if err := b.step(); err != nil {
			return err
		}

		text := node.text
		if node.kind != 0 {
			value, err := data.lookup(node.text, scopes)
			if err != nil {
				return err
			}

			if node.kind == '#' || node.kind == '^' {
				if err := renderTemplateSection(node, value, data, scopes, b, output); err != nil {
					return err
				}
				continue
			}

			text, err = scalarText(value)
			if err != nil {
				return fmt.Errorf("template reference %q requires a scalar or explicit encoding", node.text)
			}
		}

		if err := b.addBytes(len(text)); err != nil {
			return err
		}

		output.WriteString(text)
	}

	return nil
}

// renderTemplateSection uses Mustache truthiness: false, null, and empty arrays
// are false; other JSON values render once, with their value as the inner scope.
// Each array iteration costs a step even when its section has no children.
func renderTemplateSection(
	node templateNode,
	value any,
	data *Context,
	scopes []any,
	b *budget,
	output *strings.Builder,
) error {
	items, array := value.([]any)
	falsey := value == nil || (array && len(items) == 0)
	if boolean, ok := value.(bool); ok && !boolean {
		falsey = true
	}

	if node.kind == '^' {
		if falsey {
			return renderTemplateNodes(node.children, data, scopes, b, output)
		}
		return nil
	}

	if falsey {
		return nil
	}

	if !array {
		items = []any{value}
	}

	inner := make([]any, len(scopes)+1)
	copy(inner[1:], scopes)

	for _, item := range items {
		if err := b.step(); err != nil {
			return err
		}
		inner[0] = item

		if err := renderTemplateNodes(node.children, data, inner, b, output); err != nil {
			return err
		}
	}

	return nil
}
