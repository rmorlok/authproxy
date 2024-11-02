package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type Root struct {
	AdminApi     ApiHost       `json:"admin_api" yaml:"admin_api"`
	Api          ApiHost       `json:"api" yaml:"api"`
	Auth         ApiHost       `json:"auth" yaml:"auth"`
	SystemAuth   SystemAuth    `json:"system_auth" yaml:"system_auth"`
	Integrations []Integration `json:"integrations" yaml:"integrations"`
}

func (r *Root) MustApiHostForService(serviceId ServiceId) *ApiHost {
	if serviceId == ServiceIdAdminApi {
		return &r.AdminApi
	}

	panic(fmt.Sprintf("invalid service id %s", serviceId))
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
