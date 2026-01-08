package config

import "github.com/rmorlok/authproxy/internal/config/common"

type AdminUser struct {
	Username    string              `json:"username" yaml:"username"`
	Email       string              `json:"email" yaml:"email"`
	Key         *Key                `json:"key" yaml:"key"`
	Permissions []common.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}
