package config

type Service interface {
	Port() uint64
	IsHttps() bool
	Domain() string
	GetBaseUrl() string
	SupportsSession() bool
	GetId() ServiceId
}
