package config

import (
	"crypto/tls"
	"github.com/rmorlok/authproxy/internal/util"
	"net/http"
	"time"
)

type ServiceId string

const (
	ServiceIdAdminApi ServiceId = "admin-api"
	ServiceIdApi      ServiceId = "api"
	ServiceIdPublic   ServiceId = "public"
	ServiceIdWorker   ServiceId = "worker"
)

func AllServiceIds() []ServiceId {
	return []ServiceId{
		ServiceIdAdminApi,
		ServiceIdApi,
		ServiceIdPublic,
		ServiceIdWorker,
	}
}

func AllServiceIdStrings() []string {
	return util.Map(AllServiceIds(), func(s ServiceId) string { return string(s) })
}

func IsValidServiceId(id ServiceId) bool {
	for _, serviceId := range AllServiceIds() {
		if id == serviceId {
			return true
		}
	}

	return false
}

func AllValidServiceIds(ids []string) bool {
	for _, id := range ids {
		if !IsValidServiceId(ServiceId(id)) {
			return false
		}
	}

	return true
}

type Service interface {
	GetId() ServiceId
	HealthCheckPort() uint64
}

type HttpService interface {
	Service
	Port() uint64
	IsHttps() bool
	TlsConfig() (*tls.Config, error)
	Domain() string
	GetBaseUrl() string
	SupportsSession() bool
	GetServerAndHealthChecker(
		server http.Handler,
		healthChecker http.Handler,
	) (httpServer *http.Server, httpHealthChecker *http.Server, err error)
}

type HttpServiceWithSession interface {
	HttpService
	SessionTimeout() time.Duration
	CookieDomain() string
	CookieSameSite() http.SameSite
	XsrfRequestQueueDepth() int
}
