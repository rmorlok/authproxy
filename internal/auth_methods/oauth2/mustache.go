package oauth2

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/aptmpl"
)

// getMustacheContext builds the mustache template data context from the connection's configuration.
// Returns {"configuration": <decrypted configuration map>}. If no configuration is set, returns
// an empty context so templates with missing variables render as empty strings.
func (o *oAuth2Connection) getMustacheContext(ctx context.Context) (map[string]any, error) {
	data := map[string]any{}

	if o.connection == nil {
		return data, nil
	}

	config, err := o.connection.GetConfiguration(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection configuration for mustache context: %w", err)
	}

	if config != nil {
		data["configuration"] = config
	}

	return data, nil
}

// renderMustache renders a mustache template string using the connection's configuration.
// If the string contains no mustache syntax, it is returned unchanged without fetching configuration.
func (o *oAuth2Connection) renderMustache(ctx context.Context, template string) (string, error) {
	if !aptmpl.ContainsMustache(template) {
		return template, nil
	}

	data, err := o.getMustacheContext(ctx)
	if err != nil {
		return "", err
	}

	return aptmpl.RenderMustache(template, data)
}
