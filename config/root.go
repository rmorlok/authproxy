package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type Root struct {
	AdminApi   ServiceAdminApi `json:"admin_api" yaml:"admin_api"`
	Api        ServiceApi      `json:"api" yaml:"api"`
	Public     ServicePublic   `json:"public" yaml:"public"`
	Worker     ServiceWorker   `json:"worker" yaml:"worker"`
	SystemAuth SystemAuth      `json:"system_auth" yaml:"system_auth"`
	Database   Database        `json:"database" yaml:"database"`
	Redis      Redis           `json:"redis" yaml:"redis"`
	Oauth      OAuth           `json:"oauth" yaml:"oauth"`
	ErrorPages ErrorPages      `json:"error_pages" yaml:"error_pages"`
	Connectors []Connector     `json:"connectors" yaml:"connectors"`
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
