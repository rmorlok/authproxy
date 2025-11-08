package config

import (
	"fmt"
	"log/slog"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type Root struct {
	AdminApi        ServiceAdminApi `json:"admin_api" yaml:"admin_api"`
	Api             ServiceApi      `json:"api" yaml:"api"`
	Public          ServicePublic   `json:"public" yaml:"public"`
	Worker          ServiceWorker   `json:"worker" yaml:"worker"`
	Marketplace     *Marketplace    `json:"marketplace,omitempty" yaml:"marketplace,omitempty"`
	HostApplication HostApplication `json:"host_application" yaml:"host_application"`
	SystemAuth      SystemAuth      `json:"system_auth" yaml:"system_auth"`
	Database        Database        `json:"database" yaml:"database"`
	Logging         LoggingConfig   `json:"logging,omitempty" yaml:"logging,omitempty"`
	Redis           Redis           `json:"redis" yaml:"redis"`
	Oauth           OAuth           `json:"oauth" yaml:"oauth"`
	ErrorPages      ErrorPages      `json:"error_pages,omitempty" yaml:"error_pages,omitempty"`
	Connectors      *Connectors     `json:"connectors" yaml:"connectors"`
	HttpLogging     *HttpLogging    `json:"http_logging,omitempty" yaml:"http_logging,omitempty"`
	DevSettings     *DevSettings    `json:"dev_settings,omitempty" yaml:"dev_settings,omitempty"`
}

func (r *Root) GetRootLogger() *slog.Logger {
	if r == nil || r.Logging == nil {
		return util.ToPtr(LoggingConfigNone{Type: LoggingConfigTypeNone}).GetRootLogger()
	}

	return r.Logging.GetRootLogger()
}

func (r *Root) Validate() error {
	vc := &common.ValidationContext{Path: "$"}
	result := &multierror.Error{}

	if r.Connectors == nil {
		result = multierror.Append(result, vc.NewError("connectors block is required"))
	} else if err := r.Connectors.Validate(vc.PushField("connectors")); err != nil {
		result = multierror.Append(result, err)
	}

	if err := r.HostApplication.Validate(vc.PushField("host_application")); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func (r *Root) MustGetService(serviceId ServiceId) Service {
	switch serviceId {
	case ServiceIdApi:
		return &r.Api
	case ServiceIdAdminApi:
		return &r.AdminApi
	case ServiceIdPublic:
		return &r.Public
	case ServiceIdWorker:
		return &r.Worker
	}

	panic(fmt.Sprintf("invalid service id %s", serviceId))
}

func (sa *Root) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("root expected a mapping node, got %s", KindToString(value.Kind))
	}

	var database Database
	var redis Redis
	var logging LoggingConfig

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "database":
			if database, err = databaseUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "redis":
			if redis, err = redisUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "logging":
			if logging, err = loggingUnmarshalYAML(valueNode); err != nil {
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
	type RawType Root
	raw := (*RawType)(sa)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Database = database
	raw.Redis = redis
	raw.Logging = logging

	return nil
}

func UnmarshallYamlRootString(data string) (*Root, error) {
	return UnmarshallYamlRoot([]byte(data))
}

func UnmarshallYamlRoot(data []byte) (*Root, error) {
	var root Root
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	return &root, nil
}
