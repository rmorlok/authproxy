package config

import (
	"context"
	"strconv"

	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"gopkg.in/yaml.v3"
)

type Root struct {
	AdminUsernameVal       scommon.StringValue `json:"admin_username" yaml:"admin_username"`
	AdminPrivateKeyPathVal scommon.StringValue `json:"admin_private_key_path" yaml:"admin_private_key_path"`
	AdminSharedKeyPathVal  scommon.StringValue `json:"admin_shared_key_path" yaml:"admin_shared_key_path"`
	ServerVal              struct {
		ApiVal         scommon.StringValue `json:"api" yaml:"api"`
		AdminApiVal    scommon.StringValue `json:"admin_api" yaml:"admin_api"`
		AuthVal        scommon.StringValue `json:"auth" yaml:"auth"`
		MarketplaceVal scommon.StringValue `json:"marketplace" yaml:"marketplace"`
		AdminUiVal     scommon.StringValue `json:"admin_ui" yaml:"admin_ui"`
	} `json:"server" yaml:"server"`
	SigningProxyVal struct {
		PortVal scommon.StringValue `json:"port" yaml:"port"`
	} `json:"signing_proxy" yaml:"signing_proxy"`
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

	if val, err := r.AdminUsernameVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) AdminPrivateKeyPath() string {
	if r == nil {
		return ""
	}

	if val, err := r.AdminPrivateKeyPathVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) AdminSharedKeyPath() string {
	if r == nil {
		return ""
	}

	if val, err := r.AdminSharedKeyPathVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) ApiUrl() string {
	if r == nil {
		return ""
	}

	if val, err := r.ServerVal.ApiVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) AdminApiUrl() string {
	if r == nil {
		return ""
	}

	if val, err := r.ServerVal.AdminApiVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) AuthUrl() string {
	if r == nil {
		return ""
	}

	if val, err := r.ServerVal.AuthVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) MarketplaceUrl() string {
	if r == nil {
		return ""
	}

	if val, err := r.ServerVal.MarketplaceVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

func (r *Root) AdminUiUrl() string {
	if r == nil {
		return ""
	}

	if val, err := r.ServerVal.AdminUiVal.GetValue(context.Background()); err == nil {
		return val
	}

	return ""
}

// SigningProxyPort returns the configured port for `ap signing-proxy`, or 0 if
// unset. Returns an error only if the value is set but doesn't parse as an int.
func (r *Root) SigningProxyPort() (int, error) {
	if r == nil {
		return 0, nil
	}

	if !r.SigningProxyVal.PortVal.HasValue(context.Background()) {
		return 0, nil
	}

	val, err := r.SigningProxyVal.PortVal.GetValue(context.Background())
	if err != nil {
		return 0, err
	}

	if val == "" {
		return 0, nil
	}

	return strconv.Atoi(val)
}
