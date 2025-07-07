package config

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/config/common"
	"gopkg.in/yaml.v3"
	"html/template"
	"net/url"
)

type ErrorPage string

const (
	ErrorPageNotFound      ErrorPage = "not_found"
	ErrorPageUnauthorized  ErrorPage = "unauthorized"
	ErrorPageInternalError ErrorPage = "internal_error"
)

type ErrorPages struct {
	NotFound      string      `json:"not_found,omitempty" yaml:"not_found,omitempty"`
	Unauthorized  string      `json:"unauthorized,omitempty" yaml:"unauthorized,omitempty"`
	InternalError string      `json:"internal_error,omitempty" yaml:"internal_error,omitempty"`
	Template      StringValue `json:"template,omitempty" yaml:"template,omitempty"`
}

func (ep *ErrorPages) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("error_pages expected a mapping node, got %s", KindToString(value.Kind))
	}

	var t StringValue

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "template":
			if t, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType ErrorPages
	raw := (*RawType)(ep)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Template = t

	return nil
}

// UnmarshalJSON implements custom JSON unmarshalling for the ErrorPages struct
func (ep *ErrorPages) UnmarshalJSON(data []byte) error {
	// Define a temporary struct with the same fields as ErrorPages
	// but with Template as json.RawMessage to capture its raw JSON
	type TempErrorPages struct {
		NotFound      string          `json:"not_found,omitempty"`
		Unauthorized  string          `json:"unauthorized,omitempty"`
		InternalError string          `json:"internal_error,omitempty"`
		Template      json.RawMessage `json:"template,omitempty"`
	}

	var temp TempErrorPages

	// Unmarshal into the temporary struct
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy the simple fields
	ep.NotFound = temp.NotFound
	ep.Unauthorized = temp.Unauthorized
	ep.InternalError = temp.InternalError

	// Handle Template if it's not null
	if len(temp.Template) > 0 && string(temp.Template) != "null" {
		var err error
		ep.Template, err = StringValueUnmarshalJSON(temp.Template)
		if err != nil {
			return err
		}
	}

	return nil
}

const (
	defaultErrorTitle       = "Error Occurred"
	defaultErrorDescription = "An unexpected error has occurred. Please try again later."
)

const errorTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
</head>
<body>
    <h1>{{.Title}}</h1>
    <p>{{.Description}}</p>
</body>
</html>
`

type ErrorTemplateValues struct {
	Error       ErrorPage
	Title       string
	Description string
}

func (ep *ErrorPages) urlForError(error ErrorPage, publicBaseUrl string) string {
	if ep != nil {
		switch error {
		case ErrorPageNotFound:
			return ep.NotFound
		case ErrorPageUnauthorized:
			return ep.Unauthorized
		case ErrorPageInternalError:
			return ep.InternalError
		}
	}

	parsedUrl, err := url.Parse(publicBaseUrl)
	if err != nil {
		return publicBaseUrl + "/error?error=" + string(error)
	}

	query := parsedUrl.Query()
	query.Set("error", string(error))
	parsedUrl.Path = "/error"
	parsedUrl.RawQuery = query.Encode()

	return parsedUrl.String()
}

func (ep *ErrorPages) RenderRenderOrRedirect(gctx *gin.Context, vals ErrorTemplateValues) {
	switch vals.Error {
	case ErrorPageNotFound:
		if ep.NotFound != "" {
			gctx.Redirect(302, ep.NotFound)
		}
		return
	case ErrorPageUnauthorized:
		if ep.Unauthorized != "" {
			gctx.Redirect(302, ep.Unauthorized)
		}
		return
	case ErrorPageInternalError:
		if ep.InternalError != "" {
			gctx.Redirect(302, ep.InternalError)
		}
		return
	}

	// Either error isn't populated or there isn't a configured redirect. Do render instead.
	ep.RenderErrorPage(gctx, vals)
}

func (ep *ErrorPages) RenderErrorPage(gctx *gin.Context, vals ErrorTemplateValues) {
	switch vals.Error {
	case ErrorPageNotFound:
		gctx.Status(404)
		if vals.Title == "" {
			vals.Title = "Page Not Found"
		}
		if vals.Description == "" {
			vals.Description = "The page you requested could not be found."
		}
		break
	case ErrorPageUnauthorized:
		gctx.Status(401)
		if vals.Title == "" {
			vals.Title = "Unauthorized"
		}
		if vals.Description == "" {
			vals.Description = "You are not authorized to access this page."
		}
	case ErrorPageInternalError:
		gctx.Status(500)
		if vals.Title == "" {
			vals.Title = "Internal Error"
		}
		if vals.Description == "" {
			vals.Description = "An internal error has occurred. Please try again later."
		}
		break
	default:
		gctx.Status(500)
		vals.Error = ErrorPageInternalError
	}

	if vals.Title == "" {
		vals.Title = defaultErrorTitle
	}
	if vals.Description == "" {
		vals.Description = defaultErrorDescription
	}

	var err error
	t := errorTemplate
	if ep.Template != nil && ep.Template.HasValue(gctx) {
		t, err = ep.Template.GetValue(gctx)
		if err != nil {
			t = errorTemplate
		}
	}

	tmpl := template.Must(template.New("error").Parse(t))

	gctx.Header("Content-Type", "text/html")
	tmpl.Execute(gctx.Writer, vals)
}
