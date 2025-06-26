package config

import "gopkg.in/yaml.v3"

type Root struct {
	AdminUsernameVal       string `json:"admin_username" yaml:"admin_username"`
	AdminPrivateKeyPathVal string `json:"admin_private_key_path" yaml:"admin_private_key_path"`
	AdminSharedKeyPathVal  string `json:"admin_shared_key_path" yaml:"admin_shared_key_path"`
	ServerVal              struct {
		ApiVal         string `json:"api" yaml:"api"`
		AdminApiVal    string `json:"admin_api" yaml:"admin_api"`
		AuthVal        string `json:"auth" yaml:"auth"`
		MarketplaceVal string `json:"marketplace" yaml:"marketplace"`
	} `json:"server" yaml:"server"`
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

func (r *Root) AdminUsername() string {
	if r == nil {
		return ""
	}

	return r.AdminUsernameVal
}

func (r *Root) AdminPrivateKeyPath() string {
	if r == nil {
		return ""
	}

	return r.AdminPrivateKeyPathVal
}

func (r *Root) AdminSharedKeyPath() string {
	if r == nil {
		return ""
	}

	return r.AdminSharedKeyPathVal
}

func (r *Root) ApiUrl() string {
	if r == nil {
		return ""
	}

	return r.ServerVal.ApiVal
}

func (r *Root) AdminApiUrl() string {
	if r == nil {
		return ""
	}

	return r.ServerVal.AdminApiVal
}

func (r *Root) AuthUrl() string {
	if r == nil {
		return ""
	}

	return r.ServerVal.AuthVal
}

func (r *Root) MarketplaceUrl() string {
	if r == nil {
		return ""
	}

	return r.ServerVal.MarketplaceVal
}
