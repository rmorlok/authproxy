package seeder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const (
	defaultProviderBaseURL = "http://go-oauth2-server:8080"
	defaultProgressEvery   = 1000
)

type Options struct {
	Profile              Profile
	DB                   database.DB
	Encrypt              encrypt.E
	ProviderBaseURL      string
	OAuthExpiringPercent *int
	PeriodicProbePercent *int
	VerifySamples        int
	ProgressEvery        int
	Now                  time.Time
	Logf                 func(format string, args ...any)
}

type Result struct {
	ProfileName               string             `json:"profile_name"`
	ProviderBaseURL           string             `json:"provider_base_url"`
	BaseNamespace             string             `json:"base_namespace"`
	ConnectorID               apid.ID            `json:"connector_id"`
	ConnectorVersion          uint64             `json:"connector_version"`
	RequestedTenantNamespaces int                `json:"requested_tenant_namespaces"`
	RequestedConnections      int                `json:"requested_connections"`
	OAuthExpiringPercent      int                `json:"oauth_expiring_percent"`
	PeriodicProbePercent      int                `json:"periodic_probe_percent"`
	StartedAt                 time.Time          `json:"started_at"`
	FinishedAt                time.Time          `json:"finished_at"`
	CreatedNamespaces         int                `json:"created_namespaces"`
	UpsertedActors            int                `json:"upserted_actors"`
	CreatedConnections        int                `json:"created_connections"`
	ExistingConnections       int                `json:"existing_connections"`
	UpsertedOAuthTokens       int                `json:"upserted_oauth_tokens"`
	ProbeEnabledConnections   int                `json:"probe_enabled_connections"`
	VerifiedSamples           []VerifiedSample   `json:"verified_samples"`
	Namespaces                []NamespaceRecord  `json:"namespaces"`
	Actors                    []ActorRecord      `json:"actors"`
	Connections               []ConnectionRecord `json:"connections"`
}

type NamespaceRecord struct {
	Namespace string `json:"namespace"`
}

type ActorRecord struct {
	ActorID    apid.ID `json:"actor_id"`
	Namespace  string  `json:"namespace"`
	ExternalID string  `json:"external_id"`
}

type ConnectionRecord struct {
	ConnectionID         apid.ID   `json:"connection_id"`
	Namespace            string    `json:"namespace"`
	ActorID              apid.ID   `json:"actor_id"`
	ConnectorID          apid.ID   `json:"connector_id"`
	ConnectorVersion     uint64    `json:"connector_version"`
	RefreshToken         string    `json:"refresh_token"`
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
	ProbeEnabled         bool      `json:"probe_enabled"`
	OAuthTokenID         *apid.ID  `json:"oauth_token_id,omitempty"`
}

type VerifiedSample struct {
	ConnectionID apid.ID `json:"connection_id"`
	Namespace    string  `json:"namespace"`
	ActorID      apid.ID `json:"actor_id"`
	OAuthTokenID apid.ID `json:"oauth_token_id"`
}

func Seed(ctx context.Context, opts Options) (*Result, error) {
	if opts.DB == nil {
		return nil, fmt.Errorf("database is required")
	}
	if opts.Encrypt == nil {
		return nil, fmt.Errorf("encrypt service is required")
	}
	if opts.Profile.Name == "" {
		return nil, fmt.Errorf("profile is required")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	providerBaseURL := strings.TrimRight(opts.ProviderBaseURL, "/")
	if providerBaseURL == "" {
		providerBaseURL = defaultProviderBaseURL
	}
	oauthExpiringPercent := opts.Profile.DefaultOAuthExpiringPercent()
	if opts.OAuthExpiringPercent != nil {
		oauthExpiringPercent = *opts.OAuthExpiringPercent
	}
	oauthExpiringPercent = clampPercent(oauthExpiringPercent)
	periodicProbePercent := opts.Profile.DefaultPeriodicProbePercent()
	if opts.PeriodicProbePercent != nil {
		periodicProbePercent = *opts.PeriodicProbePercent
	}
	periodicProbePercent = clampPercent(periodicProbePercent)
	verifySamples := opts.VerifySamples
	if verifySamples < 0 {
		verifySamples = 0
	}
	progressEvery := opts.ProgressEvery
	if progressEvery <= 0 {
		progressEvery = defaultProgressEvery
	}

	slug := slugForID(opts.Profile.Name)
	baseNamespace := nschema.PathFromRoot("loadtest", slugForNamespace(opts.Profile.Name))
	connectorID := apid.ID(fmt.Sprintf("%slt_%s_oauth2", apid.PrefixConnectorVersion, slug))
	connectorVersion := uint64(1)
	tenantCount := opts.Profile.TenantNamespaceCount()
	connectionCount := opts.Profile.Objects.Connections
	probeEnabledCount := percentCount(connectionCount, periodicProbePercent)
	expiringCount := percentCount(connectionCount, oauthExpiringPercent)

	result := &Result{
		ProfileName:               opts.Profile.Name,
		ProviderBaseURL:           providerBaseURL,
		BaseNamespace:             baseNamespace,
		ConnectorID:               connectorID,
		ConnectorVersion:          connectorVersion,
		RequestedTenantNamespaces: tenantCount,
		RequestedConnections:      connectionCount,
		OAuthExpiringPercent:      oauthExpiringPercent,
		PeriodicProbePercent:      periodicProbePercent,
		StartedAt:                 now,
	}

	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	logf("ensuring namespace tree under %s", baseNamespace)
	if err := opts.DB.EnsureNamespaceByPath(ctx, baseNamespace); err != nil {
		return nil, fmt.Errorf("ensure base namespace: %w", err)
	}

	tenantNamespaces := make([]string, 0, tenantCount)
	for i := 1; i <= tenantCount; i++ {
		ns := fmt.Sprintf("%s.tenant%06d", baseNamespace, i)
		if err := opts.DB.EnsureNamespaceByPath(ctx, ns); err != nil {
			return nil, fmt.Errorf("ensure tenant namespace %s: %w", ns, err)
		}
		tenantNamespaces = append(tenantNamespaces, ns)
		result.Namespaces = append(result.Namespaces, NamespaceRecord{Namespace: ns})
	}
	result.CreatedNamespaces = len(tenantNamespaces)

	logf("upserting %d actors", len(tenantNamespaces))
	actors := make([]ActorRecord, 0, len(tenantNamespaces))
	for i, ns := range tenantNamespaces {
		actor := &database.Actor{
			Id:         apid.ID(fmt.Sprintf("%slt_%s_%06d", apid.PrefixActor, slug, i+1)),
			Namespace:  ns,
			ExternalId: fmt.Sprintf("loadtest-%s-%06d", slug, i+1),
			Permissions: database.Permissions{
				{
					Namespace: ns,
					Resources: []string{aschema.PermissionWildcard},
					Verbs:     []string{aschema.PermissionWildcard},
				},
			},
			Labels: baseLabels(opts.Profile.Name),
			Annotations: database.Annotations{
				"loadtest.authproxy.io/generated-by": "loadtest-seeder",
			},
		}
		upserted, err := opts.DB.UpsertActor(ctx, actor)
		if err != nil {
			return nil, fmt.Errorf("upsert actor %s: %w", actor.Id, err)
		}
		record := ActorRecord{ActorID: upserted.Id, Namespace: upserted.Namespace, ExternalID: upserted.ExternalId}
		actors = append(actors, record)
		result.Actors = append(result.Actors, record)
	}
	result.UpsertedActors = len(actors)

	logf("upserting load-test OAuth2 connector %s", connectorID)
	connectorDef := connectorDefinition(connectorID, connectorVersion, baseNamespace, opts.Profile.Name, providerBaseURL, periodicProbePercent > 0)
	connectorJSON, err := json.Marshal(connectorDef)
	if err != nil {
		return nil, fmt.Errorf("marshal connector definition: %w", err)
	}
	encryptedDefinition, err := opts.Encrypt.EncryptStringGlobal(ctx, string(connectorJSON))
	if err != nil {
		return nil, fmt.Errorf("encrypt connector definition: %w", err)
	}
	connectorHash := connectorDef.Hash()
	existingConnector, err := opts.DB.GetConnectorVersion(ctx, connectorID, connectorVersion)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return nil, fmt.Errorf("get connector version: %w", err)
	}
	if existingConnector != nil && existingConnector.Hash != connectorHash {
		return nil, fmt.Errorf("connector %s:%d already exists with a different hash", connectorID, connectorVersion)
	}
	if existingConnector == nil {
		err = opts.DB.UpsertConnectorVersion(ctx, &database.ConnectorVersion{
			Id:                  connectorID,
			Version:             connectorVersion,
			Namespace:           baseNamespace,
			State:               database.ConnectorVersionStatePrimary,
			Hash:                connectorHash,
			EncryptedDefinition: encryptedDefinition,
			Labels:              baseLabels(opts.Profile.Name),
			Annotations: database.Annotations{
				"loadtest.authproxy.io/provider-base-url": providerBaseURL,
			},
		})
	}
	if err != nil {
		return nil, fmt.Errorf("upsert connector version: %w", err)
	}

	if connectionCount > 0 && len(tenantNamespaces) == 0 {
		return nil, fmt.Errorf("profile requests connections but no namespaces")
	}

	logf("seeding %d connections", connectionCount)
	for i := 1; i <= connectionCount; i++ {
		ns := tenantNamespaces[(i-1)%len(tenantNamespaces)]
		actor := actors[(i-1)%len(actors)]
		connectionID := apid.ID(fmt.Sprintf("%slt_%s_%09d", apid.PrefixConnection, slug, i))
		probeEnabled := i <= probeEnabledCount
		expiresAt := now.Add(24 * time.Hour)
		if i <= expiringCount {
			expiresAt = now.Add(1 * time.Minute)
		}

		connection := &database.Connection{
			Id:               connectionID,
			Namespace:        ns,
			State:            database.ConnectionStateConfigured,
			HealthState:      database.ConnectionHealthStateHealthy,
			ConnectorId:      connectorID,
			ConnectorVersion: connectorVersion,
			Labels:           connectionLabels(opts.Profile.Name, i, probeEnabled),
			Annotations: database.Annotations{
				"loadtest.authproxy.io/generated-by": "loadtest-seeder",
			},
		}
		existing, err := opts.DB.GetConnection(ctx, connectionID)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			return nil, fmt.Errorf("get connection %s: %w", connectionID, err)
		}
		if existing == nil {
			if err := opts.DB.CreateConnection(ctx, connection); err != nil {
				return nil, fmt.Errorf("create connection %s: %w", connectionID, err)
			}
			result.CreatedConnections++
		} else {
			result.ExistingConnections++
		}

		refreshToken := fmt.Sprintf("rt_%s", connectionID)
		accessToken := fmt.Sprintf("at_%s", connectionID)
		encryptedRefreshToken, err := opts.Encrypt.EncryptStringForNamespace(ctx, ns, refreshToken)
		if err != nil {
			return nil, fmt.Errorf("encrypt refresh token for %s: %w", connectionID, err)
		}
		encryptedAccessToken, err := opts.Encrypt.EncryptStringForNamespace(ctx, ns, accessToken)
		if err != nil {
			return nil, fmt.Errorf("encrypt access token for %s: %w", connectionID, err)
		}
		token, err := opts.DB.InsertOAuth2Token(
			ctx,
			connectionID,
			nil,
			encryptedRefreshToken,
			encryptedAccessToken,
			&expiresAt,
			"loadtest.read",
			"loadtest.read",
			&actor.ActorID,
		)
		if err != nil {
			return nil, fmt.Errorf("insert OAuth2 token for %s: %w", connectionID, err)
		}
		result.UpsertedOAuthTokens++
		if probeEnabled {
			result.ProbeEnabledConnections++
		}
		tokenID := token.Id
		result.Connections = append(result.Connections, ConnectionRecord{
			ConnectionID:         connectionID,
			Namespace:            ns,
			ActorID:              actor.ActorID,
			ConnectorID:          connectorID,
			ConnectorVersion:     connectorVersion,
			RefreshToken:         refreshToken,
			AccessToken:          accessToken,
			AccessTokenExpiresAt: expiresAt,
			ProbeEnabled:         probeEnabled,
			OAuthTokenID:         &tokenID,
		})

		if i%progressEvery == 0 {
			logf("seeded %d/%d connections", i, connectionCount)
		}
	}

	if verifySamples > 0 {
		samples, err := verifySeededSamples(ctx, opts.DB, result.Connections, verifySamples)
		if err != nil {
			return nil, err
		}
		result.VerifiedSamples = samples
	}

	result.FinishedAt = time.Now().UTC()
	logf("seeded %d connections (%d new, %d existing)", connectionCount, result.CreatedConnections, result.ExistingConnections)
	return result, nil
}

func verifySeededSamples(ctx context.Context, db database.DB, connections []ConnectionRecord, sampleCount int) ([]VerifiedSample, error) {
	if len(connections) == 0 || sampleCount == 0 {
		return nil, nil
	}
	indexes := sampleIndexes(len(connections), sampleCount)
	samples := make([]VerifiedSample, 0, len(indexes))
	for _, idx := range indexes {
		record := connections[idx]
		conn, err := db.GetConnection(ctx, record.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("verify connection %s: %w", record.ConnectionID, err)
		}
		token, err := db.GetOAuth2Token(ctx, record.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("verify OAuth2 token for %s: %w", record.ConnectionID, err)
		}
		samples = append(samples, VerifiedSample{
			ConnectionID: conn.Id,
			Namespace:    conn.Namespace,
			ActorID:      record.ActorID,
			OAuthTokenID: token.Id,
		})
	}
	return samples, nil
}

func sampleIndexes(length, requested int) []int {
	if requested >= length {
		indexes := make([]int, length)
		for i := range indexes {
			indexes[i] = i
		}
		return indexes
	}
	seen := make(map[int]struct{}, requested)
	indexes := make([]int, 0, requested)
	for i := 0; i < requested; i++ {
		idx := 0
		if requested > 1 {
			idx = i * (length - 1) / (requested - 1)
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		indexes = append(indexes, idx)
	}
	return indexes
}

func connectorDefinition(id apid.ID, version uint64, namespace, profileName, providerBaseURL string, includeProbe bool) *cschema.Connector {
	refreshInBackground := true
	def := &cschema.Connector{
		Id:          id,
		Version:     version,
		Namespace:   &namespace,
		State:       string(database.ConnectorVersionStatePrimary),
		DisplayName: fmt.Sprintf("Load Test OAuth2 (%s)", profileName),
		Highlight:   "Synthetic OAuth2 connector for AuthProxy load tests.",
		Description: "Generated by the AuthProxy load-test seeder.",
		Auth: &cschema.Auth{InnerVal: &cschema.AuthOAuth2{
			Type:         cschema.AuthTypeOAuth2,
			ClientId:     common.NewStringValueDirectInline("loadtest-client"),
			ClientSecret: common.NewStringValueDirectInline("loadtest-secret"),
			Scopes: []cschema.Scope{
				{Id: "loadtest.read", Reason: "Exercise AuthProxy load-test proxy and refresh paths."},
			},
			Authorization: cschema.AuthOauth2Authorization{
				Endpoint: providerBaseURL + "/oauth/authorize",
			},
			Token: cschema.AuthOauth2Token{
				Endpoint:                providerBaseURL + "/oauth/token",
				RefreshInBackground:     &refreshInBackground,
				RefreshTimeBeforeExpiry: &common.HumanDuration{Duration: 15 * time.Minute},
			},
		}},
		Labels: baseLabels(profileName),
	}
	if includeProbe {
		def.Probes = []cschema.Probe{
			{
				Id:     "load-sink",
				Period: &common.HumanDuration{Duration: 5 * time.Minute},
				ProxyHttp: &cschema.ProbeHttp{
					Method: "GET",
					URL:    providerBaseURL + "/test/load/resource/probe",
				},
			},
		}
	}
	return def
}

func baseLabels(profileName string) database.Labels {
	return database.Labels{
		"loadtest.authproxy.io/profile": slugForLabelValue(profileName),
		"loadtest.authproxy.io/suite":   "authproxy-load",
		"loadtest":                      "true",
	}
}

func connectionLabels(profileName string, index int, probeEnabled bool) database.Labels {
	labels := baseLabels(profileName)
	labels["loadtest.authproxy.io/connection-index"] = fmt.Sprintf("%d", index)
	if probeEnabled {
		labels["loadtest.authproxy.io/probe"] = "true"
	} else {
		labels["loadtest.authproxy.io/probe"] = "false"
	}
	return labels
}

func percentCount(total, pct int) int {
	if total <= 0 || pct <= 0 {
		return 0
	}
	if pct >= 100 {
		return total
	}
	return total * pct / 100
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

var nonIDChar = regexp.MustCompile(`[^a-z0-9]+`)

func slugForID(value string) string {
	slug := strings.ToLower(value)
	slug = nonIDChar.ReplaceAllString(slug, "_")
	slug = strings.Trim(slug, "_")
	if slug == "" {
		return "profile"
	}
	return slug
}

func slugForNamespace(value string) string {
	slug := slugForID(value)
	if slug[0] >= '0' && slug[0] <= '9' {
		return "p" + slug
	}
	return slug
}

func slugForLabelValue(value string) string {
	slug := slugForID(value)
	if len(slug) > database.LabelValueMaxLength {
		return slug[:database.LabelValueMaxLength]
	}
	return slug
}
