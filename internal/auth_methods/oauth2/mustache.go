package oauth2

import (
	"context"

	"github.com/rmorlok/authproxy/internal/aptmpl"
)

// renderMustache renders a mustache template string using the connection's mustache context.
// If the string contains no mustache syntax, it is returned unchanged without fetching the context.
func (o *oAuth2Connection) renderMustache(ctx context.Context, template string) (string, error) {
	if !aptmpl.ContainsMustache(template) {
		return template, nil
	}

	data, err := o.connection.GetMustacheContext(ctx)
	if err != nil {
		return "", err
	}

	return aptmpl.RenderMustache(template, data)
}
