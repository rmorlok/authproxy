package iface

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type EncryptionKey interface {
	GetId() apid.ID
	GetNamespace() string
	GetState() database.EncryptionKeyState
	GetLabels() map[string]string
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}

type ListEncryptionKeysExecutor interface {
	FetchPage(context.Context) pagination.PageResult[EncryptionKey]
	Enumerate(context.Context, func(pagination.PageResult[EncryptionKey]) (keepGoing bool, err error)) error
}

type ListEncryptionKeysBuilder interface {
	ListEncryptionKeysExecutor
	Limit(int32) ListEncryptionKeysBuilder
	ForNamespaceMatcher(matcher string) ListEncryptionKeysBuilder
	ForNamespaceMatchers(matchers []string) ListEncryptionKeysBuilder
	ForState(database.EncryptionKeyState) ListEncryptionKeysBuilder
	OrderBy(database.EncryptionKeyOrderByField, pagination.OrderBy) ListEncryptionKeysBuilder
	IncludeDeleted() ListEncryptionKeysBuilder
	ForLabelSelector(selector string) ListEncryptionKeysBuilder
}
