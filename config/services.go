package config

import "github.com/rmorlok/authproxy/util"

type ServiceId string

const (
	ServiceIdAdminApi ServiceId = "admin-api"
	ServiceIdApi      ServiceId = "api"
	ServiceIdAuth     ServiceId = "auth"
)

func AllServiceIds() []ServiceId {
	return []ServiceId{
		ServiceIdAdminApi,
		ServiceIdApi,
		ServiceIdAuth,
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
