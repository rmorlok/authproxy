package main

import (
	"context"
	"errors"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/service"
)

// Bootstrap only after schema verification/migration, before listeners start.
// Explicit --apply-actor identities continue to use normal actor provisioning.
func prepareSystemApplyActor(ctx context.Context) error {
	dm := service.NewDependencyManager("startup-apply", cfg)
	defer dm.ShutdownMigrationResources()
	db, err := dm.GetDatabaseWithError()
	if err != nil {
		return err
	}
	return ensureSystemApplyActor(ctx, db)
}

type startupActorStore interface {
	GetActorByExternalId(context.Context, string, string) (*database.Actor, error)
	CreateActor(context.Context, *database.Actor) error
}

func ensureSystemApplyActor(ctx context.Context, db startupActorStore) error {
	_, err := db.GetActorByExternalId(ctx, namespace.Root, "system")
	if !errors.Is(err, database.ErrNotFound) {
		return err
	}
	err = db.CreateActor(ctx, &database.Actor{
		Id:          apid.New(apid.PrefixActor),
		Name:        "system",
		ExternalId:  "system",
		Namespace:   namespace.Root,
		Permissions: auth.AllPermissions(),
	})
	if errors.Is(err, database.ErrDuplicate) {
		// Another server may have created it concurrently. Never upsert: an
		// existing identity's permissions, signing key, and metadata must survive.
		_, err = db.GetActorByExternalId(ctx, namespace.Root, "system")
	}
	return err
}
