package config

import (
	"context"
	"fmt"
	"net/url"

	"github.com/rmorlok/authproxy/config/common"
	"github.com/rmorlok/authproxy/util"
	"gopkg.in/yaml.v3"
)

type ServiceAdminApi struct {
	ServiceHttp
	Ui                       *ServiceAdminUi                   `json:"ui" yaml:"ui"`
	SessionTimeoutVal        *HumanDuration                    `json:"session_timeout" yaml:"session_timeout"`
	XsrfRequestQueueDepthVal *int                              `json:"xsrf_request_queue_depth" yaml:"xsrf_request_queue_depth"`
	StaticVal                *ServicePublicStaticContentConfig `json:"static,omitempty" yaml:"static,omitempty"`
	CookieVal                *CookieConfig                     `json:"cookie,omitempty" yaml:"cookie,omitempty"`
}

func (s *ServiceAdminApi) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("service worker expected a mapping node, got %s", KindToString(value.Kind))
	}

	hs, err := httpServiceUnmarshalYAML(value)
	if err != nil {
		return err
	}

	// Let the rest unmarshall normally
	type RawType ServiceAdminApi
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ServiceHttp = hs

	return nil
}

func (s *ServiceAdminApi) SupportsUi() bool {
	if s == nil {
		return false
	}

	if s.Ui == nil {
		return false
	}

	return s.Ui.Enabled
}

func (s *ServiceAdminApi) UiBaseUrl() string {
	if !s.SupportsUi() {
		return ""
	}

	if !s.Ui.BaseUrl.HasValue(context.Background()) {
		return ""
	}

	return util.Must(s.Ui.BaseUrl.GetValue(context.Background()))
}

func (s *ServiceAdminApi) SupportsSession() bool {
	return s.SupportsUi()
}

func (s *ServiceAdminApi) GetId() ServiceId {
	return ServiceIdAdminApi
}

type ServiceAdminUi struct {
	Enabled bool        `json:"enabled" yaml:"enabled"`
	BaseUrl StringValue `json:"base_url" yaml:"base_url"`

	// InitiateSessionUrl is the URL that will be redirected to in order to establish a session for an actor. This
	// happens if the admin portal is accessed without coming from a pre-authorized context. This URL should
	// take a `redirect_url` query parameter where the actor should be redirected to following successful authentication.
	// When redirecting to `redirect_url`, the host application should append an `auth_token` query param with a signed
	// JWT for authenticating the user. This JWT should use a nonce and expiration to protect against session
	// hijacking
	InitiateSessionUrl string `json:"initiate_session_url" yaml:"initiate_session_url"`
}

func (s *ServiceAdminUi) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("admin ui expected a mapping node, got %s", KindToString(value.Kind))
	}

	var baseUrlVal StringValue = nil

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "base_url":
			if baseUrlVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
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
	type RawType ServiceAdminUi
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.BaseUrl = baseUrlVal

	return nil
}

func (s *ServiceAdminUi) GetInitiateSessionUrl(returnTo string) string {
	u, err := url.Parse(s.InitiateSessionUrl)
	if err != nil {
		return s.InitiateSessionUrl
	}

	q := u.Query()
	q.Set("return_to", returnTo)
	u.RawQuery = q.Encode()

	return u.String()
}

var _ HttpService = (*ServiceAdminApi)(nil)
