package config

import (
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type AdminUser struct {
	Username    string               `json:"username" yaml:"username"`
	Email       string               `json:"email" yaml:"email"`
	Key         *Key                 `json:"key" yaml:"key"`
	Permissions []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}
