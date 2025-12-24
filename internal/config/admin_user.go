package config

type AdminUser struct {
	Username    string   `json:"username" yaml:"username"`
	Email       string   `json:"email" yaml:"email"`
	Key         *Key     `json:"key" yaml:"key"`
	Permissions []string `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}
