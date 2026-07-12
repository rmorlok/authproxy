package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

const (
	notificationCacheTTL        = 30 * time.Second
	notificationCacheVersionKey = "notifications:version"
)

type notificationAuthCacheFingerprint struct {
	ActorID            apid.ID              `json:"actor_id"`
	ExternalID         string               `json:"external_id"`
	Namespace          string               `json:"namespace"`
	Labels             map[string]string    `json:"labels,omitempty"`
	Annotations        map[string]string    `json:"annotations,omitempty"`
	ActorPermissions   []aschema.Permission `json:"actor_permissions,omitempty"`
	RequestPermissions []aschema.Permission `json:"request_permissions,omitempty"`
}

// ListActorNotifications returns actor-visible notifications with the actor's
// viewed/action state. The result is cached because the list is read often, but
// the cache key includes the actor permission fingerprint and global
// notification version so permission or notification changes cannot share a
// stale entry.
func (s *service) ListActorNotifications(
	ctx context.Context,
	ra *authcore.RequestAuth,
	opts database.ListNotificationsOptions,
) ([]iface.ActorNotification, error) {
	if ra == nil || !ra.IsAuthenticated() {
		return nil, httperr.Forbidden("permission denied")
	}

	actor := ra.MustGetActor()
	opts.ActorId = actor.GetId()

	if cacheKey, ok := s.notificationListCacheKey(ctx, ra, opts); ok {
		if cached, hit := s.notificationListCacheGet(ctx, cacheKey); hit {
			return cached, nil
		}

		result, err := s.listActorNotificationsFromDB(ctx, ra, opts)
		if err != nil {
			return nil, err
		}
		s.notificationListCacheSet(ctx, cacheKey, result)
		return result, nil
	}

	return s.listActorNotificationsFromDB(ctx, ra, opts)
}

func (s *service) listActorNotificationsFromDB(
	ctx context.Context,
	ra *authcore.RequestAuth,
	opts database.ListNotificationsOptions,
) ([]iface.ActorNotification, error) {
	notifications, err := s.db.ListNotifications(ctx, opts)
	if err != nil {
		return nil, err
	}

	ids := make([]apid.ID, 0, len(notifications))
	for _, n := range notifications {
		ids = append(ids, n.Id)
	}

	viewed, err := s.db.NotificationViewedMap(ctx, ra.MustGetActor().GetId(), ids)
	if err != nil {
		return nil, err
	}

	result := make([]iface.ActorNotification, 0, len(notifications))
	for _, n := range notifications {
		if !notificationPermissionsAllow(ra, n.ViewPermissions, n) {
			continue
		}
		_, isViewed := viewed[n.Id]
		result = append(result, iface.ActorNotification{
			Notification: n,
			Viewed:       isViewed,
			CanAction:    notificationPermissionsAllow(ra, n.ActionPermissions, n),
		})
	}

	return result, nil
}

// MarkActorNotificationViewed records viewed state for a visible notification
// and bumps the global notification cache version.
func (s *service) MarkActorNotificationViewed(
	ctx context.Context,
	ra *authcore.RequestAuth,
	id apid.ID,
) error {
	return s.MarkActorNotificationsViewed(ctx, ra, []apid.ID{id})
}

func (s *service) MarkActorNotificationsViewed(
	ctx context.Context,
	ra *authcore.RequestAuth,
	ids []apid.ID,
) error {
	if ra == nil || !ra.IsAuthenticated() {
		return httperr.Forbidden("permission denied")
	}

	notificationIDs, err := normalizeActorNotificationIDs(ids)
	if err != nil {
		return err
	}

	for _, id := range notificationIDs {
		notification, err := s.db.GetNotification(ctx, id)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				return httperr.NotFound("notification not found", httperr.WithInternalErr(err))
			}
			return err
		}
		if !notificationPermissionsAllow(ra, notification.ViewPermissions, *notification) {
			return httperr.Forbidden("permission denied")
		}
	}

	if err := s.db.MarkNotificationsViewed(ctx, notificationIDs, ra.MustGetActor().GetId()); err != nil {
		return err
	}
	s.bumpNotificationCacheVersion(ctx)
	return nil
}

func (s *service) upsertNotification(
	ctx context.Context,
	upsert database.NotificationUpsert,
) (*database.Notification, error) {
	notification, err := s.db.UpsertNotification(ctx, upsert)
	if err != nil {
		return nil, err
	}
	s.bumpNotificationCacheVersion(ctx)
	return notification, nil
}

func (s *service) resolveNotificationsForResourceKeys(
	ctx context.Context,
	resourceType string,
	resourceID apid.ID,
	keys []string,
) error {
	if len(keys) == 0 {
		return nil
	}
	if err := s.db.ResolveNotificationsForResourceKeys(ctx, resourceType, resourceID, keys); err != nil {
		return err
	}
	s.bumpNotificationCacheVersion(ctx)
	return nil
}

// normalizeActorNotificationIDs validates that the appropriate types of ids are
// being used, and dedupes the set of ids.
func normalizeActorNotificationIDs(ids []apid.ID) ([]apid.ID, error) {
	if len(ids) == 0 {
		return nil, httperr.BadRequest("ids are required")
	}
	result := make([]apid.ID, 0, len(ids))
	seen := make(map[apid.ID]struct{}, len(ids))

	for _, id := range ids {
		if id == apid.Nil {
			return nil, httperr.BadRequest("notification id is required")
		}

		if err := id.ValidatePrefix(apid.PrefixNotification); err != nil {
			return nil, httperr.BadRequest("invalid notification id", httperr.WithInternalErr(err))
		}

		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		result = append(result, id)
	}

	return result, nil
}

func (s *service) notificationListCacheKey(
	ctx context.Context,
	ra *authcore.RequestAuth,
	opts database.ListNotificationsOptions,
) (string, bool) {
	if s.r == nil {
		return "", false
	}

	version, err := s.notificationCacheVersion(ctx)
	if err != nil {
		s.warnNotificationCache("notification cache disabled after version lookup failed", err)
		return "", false
	}

	permissionFingerprint, err := notificationPermissionFingerprint(ra)
	if err != nil {
		s.warnNotificationCache("notification cache disabled after permission fingerprint failed", err)
		return "", false
	}

	queryFingerprint, err := json.Marshal(opts)
	if err != nil {
		s.warnNotificationCache("notification cache disabled after query fingerprint failed", err)
		return "", false
	}

	return fmt.Sprintf(
		"notifications:list:v1:%s:%s:%s:%s",
		ra.MustGetActor().GetId(),
		version,
		sha256Hex(permissionFingerprint),
		sha256Hex(queryFingerprint),
	), true
}

func notificationPermissionFingerprint(ra *authcore.RequestAuth) ([]byte, error) {
	actor := ra.MustGetActor()
	return json.Marshal(notificationAuthCacheFingerprint{
		ActorID:            actor.GetId(),
		ExternalID:         actor.GetExternalId(),
		Namespace:          actor.GetNamespace(),
		Labels:             actor.GetLabels(),
		Annotations:        actor.GetAnnotations(),
		ActorPermissions:   actor.GetPermissions(),
		RequestPermissions: ra.GetPermissions(),
	})
}

func (s *service) notificationCacheVersion(ctx context.Context) (string, error) {
	value, err := s.r.Get(ctx, notificationCacheVersionKey).Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	return value, err
}

func (s *service) notificationListCacheGet(
	ctx context.Context,
	cacheKey string,
) ([]iface.ActorNotification, bool) {
	raw, err := s.r.Get(ctx, cacheKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		s.warnNotificationCache("notification cache read failed", err)
		return nil, false
	}

	var result []iface.ActorNotification
	if err := json.Unmarshal(raw, &result); err != nil {
		s.warnNotificationCache("notification cache payload was invalid", err)
		_ = s.r.Del(ctx, cacheKey).Err()
		return nil, false
	}

	return result, true
}

func (s *service) notificationListCacheSet(
	ctx context.Context,
	cacheKey string,
	result []iface.ActorNotification,
) {
	raw, err := json.Marshal(result)
	if err != nil {
		s.warnNotificationCache("notification cache payload marshal failed", err)
		return
	}
	if err := s.r.Set(ctx, cacheKey, raw, notificationCacheTTL).Err(); err != nil {
		s.warnNotificationCache("notification cache write failed", err)
	}
}

func (s *service) bumpNotificationCacheVersion(ctx context.Context) {
	if s.r == nil {
		return
	}
	if err := s.r.Incr(ctx, notificationCacheVersionKey).Err(); err != nil {
		s.warnNotificationCache("notification cache version bump failed", err)
	}
}

func notificationPermissionsAllow(
	ra *authcore.RequestAuth,
	permissions []aschema.Permission,
	n database.Notification,
) bool {
	if ra == nil || !ra.IsAuthenticated() || len(permissions) == 0 {
		return false
	}
	for _, p := range permissions {
		resources := p.Resources
		if len(resources) == 0 {
			resources = []string{n.ResourceType}
		}
		verbs := p.Verbs
		if len(verbs) == 0 {
			verbs = []string{"get"}
		}
		resourceIds := p.ResourceIds
		if len(resourceIds) == 0 {
			resourceIds = []string{n.ResourceId.String()}
		}
		for _, resource := range resources {
			for _, verb := range verbs {
				for _, resourceId := range resourceIds {
					if ra.Allows(n.Namespace, resource, verb, resourceId) {
						return true
					}
				}
			}
		}
	}
	return false
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *service) warnNotificationCache(msg string, err error) {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn(msg, "error", err)
}
