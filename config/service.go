package config

import (
	"github.com/rmorlok/authproxy/util"
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
	Port() uint64
	HealthCheckPort() uint64
	IsHttps() bool
	Domain() string
	GetBaseUrl() string
	SupportsSession() bool
	GetId() ServiceId
	SessionTimeout() time.Duration
	CookieDomain() string
	XsrfRequestQueueDepth() int
}
