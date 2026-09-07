// Command demo-seed bootstraps the demo environment's actors + connectors
// against a running AuthProxy admin API. Run as a Helm post-install /
// post-upgrade hook from the authproxy-demo umbrella chart.
//
// Idempotency model: namespaces and actors are created when absent and
// reconciled to the desired state when present. For each desired connector,
// list by its namespace/name identity, create it when absent, or publish a new
// generation when the definition changes. Re-running the seed job is a no-op
// once the state matches.
//
// Auth: signs requests as the demo-admin actor using the same keypair the
// demo-shell uses for that actor. AuthProxy already trusts that
// actor to create/list other actors via the admin-api access scope.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/config"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

// SeedConfig is the YAML shape the binary consumes.
type SeedConfig struct {
	Namespaces         []nschema.Namespace     `yaml:"namespaces"`
	Actors             []actorschema.Actor     `yaml:"actors"`
	OAuth2TestProvider *OAuth2TestProviderSeed `yaml:"oauth2TestProvider"`
	Connectors         []cschema.Connector     `yaml:"connectors"`
}

type OAuth2TestProviderSeed struct {
	BaseUrl                string                     `yaml:"baseUrl"`
	Clients                []OAuth2TestProviderClient `yaml:"clients,omitempty"`
	Users                  []OAuth2TestProviderUser   `yaml:"users,omitempty"`
	ResourcePolicies       []OAuth2ResourcePolicy     `yaml:"resourcePolicies,omitempty"`
	APIKeyResourcePolicies []APIKeyResourcePolicy     `yaml:"apiKeyResourcePolicies,omitempty"`
}

// The provider control-plane API uses snake_case JSON. The seed file remains
// camelCase YAML to match the rest of the demo configuration surface.
type OAuth2TestProviderClient struct {
	Key                     string `json:"key" yaml:"key"`
	Secret                  string `json:"secret,omitempty" yaml:"secret,omitempty"`
	RedirectURI             string `json:"redirect_uri,omitempty" yaml:"redirectUri,omitempty"`
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty" yaml:"tokenEndpointAuthMethod,omitempty"`
	RequirePKCE             bool   `json:"require_pkce,omitempty" yaml:"requirePkce,omitempty"`
	Scope                   string `json:"scope,omitempty" yaml:"scope,omitempty"`
}

type OAuth2TestProviderUser struct {
	Username    string `json:"username" yaml:"username"`
	Password    string `json:"password,omitempty" yaml:"password,omitempty"`
	Role        string `json:"role,omitempty" yaml:"role,omitempty"`
	Email       string `json:"email,omitempty" yaml:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty" yaml:"displayName,omitempty"`
	Sub         string `json:"sub,omitempty" yaml:"sub,omitempty"`
}

type OAuth2ResourcePolicy struct {
	Path          string `json:"path" yaml:"path"`
	RequiredScope string `json:"required_scope" yaml:"requiredScope"`
}

type APIKeyResourcePolicy struct {
	Path       string `json:"path" yaml:"path"`
	Key        string `json:"key" yaml:"key"`
	Placement  string `json:"placement,omitempty" yaml:"placement,omitempty"`
	HeaderName string `json:"header_name,omitempty" yaml:"headerName,omitempty"`
	Prefix     string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

type settings struct {
	adminApiUrl         string
	adminUsername       string
	adminPrivateKeyPath string
	configPath          string
}

const (
	seedRetryTimeout  = 5 * time.Minute
	seedRetryInterval = 5 * time.Second
)

type seedAction string

const (
	seedCreated        seedAction = "created"
	seedAlreadyPresent seedAction = "already-present"
	seedUpdated        seedAction = "updated"
)

func mustGetenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing required env var %s\n", key)
		os.Exit(2)
	}
	return v
}

func loadSettings() settings {
	return settings{
		adminApiUrl:         strings.TrimRight(mustGetenv("ADMIN_API_URL"), "/"),
		adminUsername:       mustGetenv("ADMIN_USERNAME"),
		adminPrivateKeyPath: mustGetenv("ADMIN_PRIVATE_KEY_PATH"),
		configPath:          mustGetenv("SEED_CONFIG_PATH"),
	}
}

func loadConfig(path string) (*SeedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed config %q: %w", path, err)
	}
	var c SeedConfig
	if err := util.DecodeYAMLStrict(data, &c); err != nil {
		return nil, fmt.Errorf("parse seed config %q: %w", path, err)
	}
	for i := range c.Namespaces {
		if err := c.Namespaces[i].ValidateFor(meta.ValidationModeCreate, nil); err != nil {
			return nil, fmt.Errorf("validate seed namespace %d: %w", i, err)
		}
	}
	for i := range c.Actors {
		if err := c.Actors[i].ValidateFor(meta.ValidationModeCreate, nil); err != nil {
			return nil, fmt.Errorf("validate seed actor %d: %w", i, err)
		}
	}
	for i := range c.Connectors {
		if err := c.Connectors[i].ValidateFor(meta.ValidationModeCreate, nil); err != nil {
			return nil, fmt.Errorf("validate seed connector %d: %w", i, err)
		}
		if c.Connectors[i].Metadata.Name == "" {
			return nil, fmt.Errorf("validate seed connector %d: metadata.name is required for idempotent seeding", i)
		}
	}
	return &c, nil
}

// newSignedClient returns a resty client that injects an admin-signed
// JWT on every request. Tokens are minted with a short TTL; the seed
// job is a one-shot, no refresh logic needed.
func newSignedClient(s settings) (*resty.Client, error) {
	signer, err := jwt.NewJwtTokenBuilder().
		WithActorExternalId(s.adminUsername).
		WithActorSigned().
		WithServiceIds(config.AllServiceIds()).
		WithExpiresIn(5 * time.Minute).
		WithPrivateKeyPath(s.adminPrivateKeyPath).
		Signer()
	if err != nil {
		return nil, fmt.Errorf("build signer: %w", err)
	}
	c := resty.New().SetTimeout(30 * time.Second)
	c.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		signer.SignRestyRequest(req)
		return nil
	})
	return c, nil
}

func upsertNamespace(c *resty.Client, baseURL string, ns nschema.Namespace) (seedAction, error) {
	path, err := nschema.PathFromMetadata(ns.Metadata)
	if err != nil {
		return "", fmt.Errorf("derive namespace path: %w", err)
	}

	var existing nschema.Namespace
	getResp, err := c.R().
		SetHeader("Accept", "application/json").
		SetResult(&existing).
		Get(fmt.Sprintf("%s/api/v1/namespaces/%s", baseURL, path))
	if err != nil {
		return "", fmt.Errorf("GET namespace %q: %w", path, err)
	}

	switch getResp.StatusCode() {
	case http.StatusOK:
		if stringMapsEqual(ns.Metadata.Labels, userLabels(existing.Metadata.Labels)) &&
			stringMapsEqual(ns.Metadata.Annotations, existing.Metadata.Annotations) {
			return seedAlreadyPresent, nil
		}
		if err := updateNamespace(c, baseURL, ns); err != nil {
			return "", err
		}
		return seedUpdated, nil
	case http.StatusNotFound:
		// Create below.
	default:
		return "", fmt.Errorf("GET namespace %q returned %d: %s", path, getResp.StatusCode(), getResp.String())
	}

	postResp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(ns).
		Post(fmt.Sprintf("%s/api/v1/namespaces", baseURL))
	if err != nil {
		return "", fmt.Errorf("POST namespace %q: %w", path, err)
	}
	if postResp.StatusCode() >= 400 {
		return "", fmt.Errorf("POST namespace %q returned %d: %s", path, postResp.StatusCode(), postResp.String())
	}
	return seedCreated, nil
}

func updateNamespace(c *resty.Client, baseURL string, ns nschema.Namespace) error {
	path, err := nschema.PathFromMetadata(ns.Metadata)
	if err != nil {
		return fmt.Errorf("derive namespace path: %w", err)
	}
	labels := ns.Metadata.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := ns.Metadata.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	patch := nschema.NewNamespacePatch()
	patch.Metadata.Labels = &labels
	patch.Metadata.Annotations = &annotations

	patchResp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(patch).
		Patch(fmt.Sprintf("%s/api/v1/namespaces/%s", baseURL, path))
	if err != nil {
		return fmt.Errorf("PATCH namespace %q: %w", path, err)
	}
	if patchResp.StatusCode() >= 400 {
		return fmt.Errorf("PATCH namespace %q returned %d: %s", path, patchResp.StatusCode(), patchResp.String())
	}
	return nil
}

// upsertActor creates the actor if absent and reconciles its mutable state.
func upsertActor(c *resty.Client, baseURL string, actor actorschema.Actor) (seedAction, error) {
	var existing actorschema.Actor
	getReq := c.R().SetHeader("Accept", "application/json").SetResult(&existing)
	getReq.SetQueryParam("namespace", actor.Metadata.Namespace)
	getResp, err := getReq.Get(fmt.Sprintf("%s/api/v1/actors/external-id/%s", baseURL, actor.Spec.ExternalId))
	if err != nil {
		return "", fmt.Errorf("GET actor %q: %w", actor.Spec.ExternalId, err)
	}

	switch getResp.StatusCode() {
	case http.StatusOK:
		if existing.Metadata.Namespace != actor.Metadata.Namespace || existing.Spec.ExternalId != actor.Spec.ExternalId {
			return "", fmt.Errorf("actor %q exists in namespace %q but does not match the configured state", actor.Spec.ExternalId, actor.Metadata.Namespace)
		}
		if existing.Metadata.Name != actor.Metadata.Name ||
			!reflect.DeepEqual(existing.Spec.Permissions, actor.Spec.Permissions) ||
			!stringMapsEqual(userLabels(existing.Metadata.Labels), actor.Metadata.Labels) ||
			!stringMapsEqual(existing.Metadata.Annotations, actor.Metadata.Annotations) {
			actor.Metadata.ID = existing.Metadata.ID
			if err := updateActor(c, baseURL, actor); err != nil {
				return "", err
			}
			return seedUpdated, nil
		}
		return seedAlreadyPresent, nil
	case http.StatusNotFound:
		// Create below.
	default:
		return "", fmt.Errorf("GET actor %q returned %d: %s", actor.Spec.ExternalId, getResp.StatusCode(), getResp.String())
	}

	postResp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(actor).
		Post(fmt.Sprintf("%s/api/v1/actors", baseURL))
	if err != nil {
		return "", fmt.Errorf("POST actor %q: %w", actor.Spec.ExternalId, err)
	}
	if postResp.StatusCode() >= 400 {
		return "", fmt.Errorf("POST actor %q returned %d: %s", actor.Spec.ExternalId, postResp.StatusCode(), postResp.String())
	}
	return seedCreated, nil
}

func updateActor(c *resty.Client, baseURL string, actor actorschema.Actor) error {
	labels := actor.Metadata.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := actor.Metadata.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	permissions := actorschema.ClonePermissions(actor.Spec.Permissions)
	patch := actorschema.NewActorPatch()
	patch.Metadata.Name = &actor.Metadata.Name
	patch.Metadata.Labels = &labels
	patch.Metadata.Annotations = &annotations
	patch.Spec.Permissions = &permissions

	patchResp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(patch).
		Patch(fmt.Sprintf("%s/api/v1/actors/%s", baseURL, actor.Metadata.ID))
	if err != nil {
		return fmt.Errorf("PATCH actor %q: %w", actor.Spec.ExternalId, err)
	}
	if patchResp.StatusCode() >= 400 {
		return fmt.Errorf("PATCH actor %q returned %d: %s", actor.Spec.ExternalId, patchResp.StatusCode(), patchResp.String())
	}
	return nil
}

func seedOAuth2TestProvider(c *resty.Client, seed OAuth2TestProviderSeed) error {
	baseUrl := strings.TrimRight(seed.BaseUrl, "/")
	if baseUrl == "" {
		return fmt.Errorf("oauth2 test provider base_url is required")
	}

	for _, client := range seed.Clients {
		if client.Key == "" {
			return fmt.Errorf("oauth2 test provider client key is required")
		}
		if _, err := postOAuth2TestProvider(c, baseUrl, "/test/clients", client); err != nil {
			return fmt.Errorf("seed oauth2 client %q: %w", client.Key, err)
		}
	}

	for _, user := range seed.Users {
		if user.Username == "" {
			return fmt.Errorf("oauth2 test provider username is required")
		}
		if _, err := postOAuth2TestProvider(c, baseUrl, "/test/users", user); err != nil {
			return fmt.Errorf("seed oauth2 user %q: %w", user.Username, err)
		}
	}

	for _, policy := range seed.ResourcePolicies {
		if policy.Path == "" {
			return fmt.Errorf("oauth2 test provider resource policy path is required")
		}
		if _, err := postOAuth2TestProvider(c, baseUrl, "/test/resource-policy", policy); err != nil {
			return fmt.Errorf("seed oauth2 resource policy %q: %w", policy.Path, err)
		}
	}

	for _, policy := range seed.APIKeyResourcePolicies {
		if policy.Path == "" {
			return fmt.Errorf("oauth2 test provider api-key resource policy path is required")
		}
		if policy.Key == "" {
			return fmt.Errorf("oauth2 test provider api-key resource policy %q key is required", policy.Path)
		}
		if _, err := postOAuth2TestProvider(c, baseUrl, "/test/api-key-resource-policy", policy); err != nil {
			return fmt.Errorf("seed oauth2 test provider api-key resource policy %q: %w", policy.Path, err)
		}
	}

	return nil
}

func postOAuth2TestProvider(c *resty.Client, baseUrl, path string, body any) (seedAction, error) {
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(baseUrl + path)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", path, err)
	}

	switch resp.StatusCode() {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return seedCreated, nil
	case http.StatusConflict:
		return seedAlreadyPresent, nil
	default:
		body := strings.ToLower(resp.String())
		if resp.StatusCode() == http.StatusBadRequest &&
			(strings.Contains(body, "already") ||
				strings.Contains(body, "exists") ||
				strings.Contains(body, "duplicate") ||
				strings.Contains(body, "taken") ||
				strings.Contains(body, "unique")) {
			return seedAlreadyPresent, nil
		}
		return "", fmt.Errorf("POST %s returned %d: %s", path, resp.StatusCode(), resp.String())
	}
}

type connectorAction string

const (
	connectorCreated        connectorAction = "created"
	connectorAlreadyPresent connectorAction = "already-present"
	connectorUpdated        connectorAction = "updated"
)

func connectorDefinitionsEqual(want cschema.ConnectorDefinition, got cschema.Connector) bool {
	return reflect.DeepEqual(normalizeForJSON(want), normalizeForJSON(got.Spec.Definition))
}

func connectorRequestForSeed(seed cschema.Connector) cschema.Connector {
	resource := seed.Clone()
	resource.Metadata.ID = ""
	resource.Metadata.Generation = 0
	resource.Metadata.CreatedAt = nil
	resource.Metadata.UpdatedAt = nil
	resource.Status = nil
	return *resource
}

func connectorObservedState(connector cschema.Connector) cschema.ConnectorReleaseState {
	if connector.Status == nil {
		return ""
	}
	return connector.Status.Release.State
}

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func userLabels(labels map[string]string) map[string]string {
	// AuthProxy materializes reserved identity and inherited labels in API
	// responses. Seed configuration owns only labels that callers can set.
	result := make(map[string]string)
	for key, value := range labels {
		if !strings.HasPrefix(key, "apxy/") {
			result[key] = value
		}
	}
	return result
}

func normalizeForJSON(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var out any
	if err := decoder.Decode(&out); err != nil {
		return v
	}
	return out
}

func listSeededConnector(c *resty.Client, baseUrl string, seed cschema.Connector) (*cschema.Connector, error) {
	var list api.ListConnectorsResponseJson
	resp, err := c.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("namespace", seed.Metadata.Namespace).
		SetQueryParam("name", string(seed.Metadata.Name)).
		SetQueryParam("limit", "2").
		SetResult(&list).
		Get(fmt.Sprintf("%s/api/v1/connectors", baseUrl))
	if err != nil {
		return nil, fmt.Errorf("GET connector seed %q: %w", seed.Metadata.Name, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("GET connector seed %q returned %d: %s", seed.Metadata.Name, resp.StatusCode(), resp.String())
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	if len(list.Items) > 1 {
		return nil, fmt.Errorf("GET connector seed %q returned %d exact-name matches", seed.Metadata.Name, len(list.Items))
	}
	return &list.Items[0], nil
}

func getConnectorVersion(c *resty.Client, baseUrl string, connector cschema.Connector) (*cschema.Connector, error) {
	var version cschema.Connector
	resp, err := c.R().
		SetHeader("Accept", "application/json").
		SetResult(&version).
		Get(fmt.Sprintf("%s/api/v1/connectors/%s/generations/%d", baseUrl, connector.GetId(), connector.Metadata.Generation))
	if err != nil {
		return nil, fmt.Errorf("GET connector version %s:%d: %w", connector.GetId(), connector.Metadata.Generation, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("GET connector version %s:%d returned %d: %s", connector.GetId(), connector.Metadata.Generation, resp.StatusCode(), resp.String())
	}
	return &version, nil
}

func createConnector(c *resty.Client, baseUrl string, seed cschema.Connector) (*cschema.Connector, error) {
	body := connectorRequestForSeed(seed)
	var created cschema.Connector
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&created).
		Post(fmt.Sprintf("%s/api/v1/connectors", baseUrl))
	if err != nil {
		return nil, fmt.Errorf("POST connector seed %q: %w", seed.Metadata.Name, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("POST connector seed %q returned %d: %s", seed.Metadata.Name, resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func createConnectorDraft(c *resty.Client, baseUrl string, connector cschema.Connector, seed cschema.Connector) (*cschema.Connector, error) {
	body := connectorRequestForSeed(seed)
	body.Metadata.Name = connector.Metadata.Name
	var created cschema.Connector
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&created).
		Post(fmt.Sprintf("%s/api/v1/connectors/%s/generations", baseUrl, connector.GetId()))
	if err != nil {
		return nil, fmt.Errorf("POST connector seed %q version: %w", seed.Metadata.Name, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("POST connector seed %q version returned %d: %s", seed.Metadata.Name, resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func forceConnectorPrimary(c *resty.Client, baseUrl string, version cschema.Connector) error {
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(api.NewConnectorForceStateRequest(
			meta.NewObjectReference(version.TypeMeta, version.Metadata),
			cschema.ConnectorReleaseStatePrimary,
		)).
		Put(fmt.Sprintf("%s/api/v1/connectors/%s/generations/%d/_forceState", baseUrl, version.GetId(), version.Metadata.Generation))
	if err != nil {
		return fmt.Errorf("PUT connector seed %s:%d primary: %w", version.GetId(), version.Metadata.Generation, err)
	}
	if resp.StatusCode() >= 400 {
		return fmt.Errorf("PUT connector seed %s:%d primary returned %d: %s", version.GetId(), version.Metadata.Generation, resp.StatusCode(), resp.String())
	}
	return nil
}

func upsertConnector(c *resty.Client, baseUrl string, seed cschema.Connector) (connectorAction, error) {
	if seed.Metadata.Name == "" {
		return "", fmt.Errorf("connector seed metadata.name is required")
	}

	existing, err := listSeededConnector(c, baseUrl, seed)
	if err != nil {
		return "", err
	}

	if existing == nil {
		created, err := createConnector(c, baseUrl, seed)
		if err != nil {
			return "", err
		}
		if connectorObservedState(*created) != cschema.ConnectorReleaseStatePrimary {
			if err := forceConnectorPrimary(c, baseUrl, *created); err != nil {
				return "", err
			}
		}
		return connectorCreated, nil
	}

	version, err := getConnectorVersion(c, baseUrl, *existing)
	if err != nil {
		return "", err
	}

	if connectorDefinitionsEqual(seed.Spec.Definition, *version) &&
		stringMapsEqual(seed.Metadata.Labels, userLabels(version.Metadata.Labels)) &&
		stringMapsEqual(seed.Metadata.Annotations, version.Metadata.Annotations) {
		if connectorObservedState(*version) != cschema.ConnectorReleaseStatePrimary {
			if err := forceConnectorPrimary(c, baseUrl, *version); err != nil {
				return "", err
			}
			return connectorUpdated, nil
		}
		return connectorAlreadyPresent, nil
	}

	created, err := createConnectorDraft(c, baseUrl, *existing, seed)
	if err != nil {
		return "", err
	}
	if err := forceConnectorPrimary(c, baseUrl, *created); err != nil {
		return "", err
	}
	return connectorUpdated, nil
}

func run(logger *slog.Logger) error {
	s := loadSettings()
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	client, err := newSignedClient(s)
	if err != nil {
		return err
	}
	providerClient := resty.New().SetTimeout(30 * time.Second)

	for _, ns := range cfg.Namespaces {
		path, pathErr := nschema.PathFromMetadata(ns.Metadata)
		if pathErr != nil {
			return fmt.Errorf("derive seed namespace path: %w", pathErr)
		}
		deadline := time.Now().Add(seedRetryTimeout)
		var action seedAction
		for attempt := 1; ; attempt++ {
			action, err = upsertNamespace(client, s.adminApiUrl, ns)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("upsert namespace %q after %s: %w", path, seedRetryTimeout, err)
			}
			logger.Warn("namespace seed attempt failed; retrying", "path", path, "attempt", attempt, "err", err)
			time.Sleep(seedRetryInterval)
		}
		logger.Info("namespace seed complete", "path", path, "action", action)
	}

	for _, a := range cfg.Actors {
		deadline := time.Now().Add(seedRetryTimeout)
		var action seedAction
		for attempt := 1; ; attempt++ {
			action, err = upsertActor(client, s.adminApiUrl, a)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("upsert actor %q after %s: %w", a.Spec.ExternalId, seedRetryTimeout, err)
			}
			logger.Warn("actor seed attempt failed; retrying",
				"external_id", a.Spec.ExternalId,
				"namespace", a.Metadata.Namespace,
				"attempt", attempt,
				"err", err,
			)
			time.Sleep(seedRetryInterval)
		}
		switch action {
		case seedCreated:
			logger.Info("actor created", "external_id", a.Spec.ExternalId, "namespace", a.Metadata.Namespace)
		case seedUpdated:
			logger.Info("actor updated", "external_id", a.Spec.ExternalId, "namespace", a.Metadata.Namespace)
		case seedAlreadyPresent:
			logger.Info("actor already present", "external_id", a.Spec.ExternalId, "namespace", a.Metadata.Namespace)
		}
	}

	if cfg.OAuth2TestProvider != nil {
		deadline := time.Now().Add(seedRetryTimeout)
		for attempt := 1; ; attempt++ {
			err = seedOAuth2TestProvider(providerClient, *cfg.OAuth2TestProvider)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("seed oauth2 test provider after %s: %w", seedRetryTimeout, err)
			}
			logger.Warn("oauth2 test provider seed attempt failed; retrying",
				"base_url", cfg.OAuth2TestProvider.BaseUrl,
				"attempt", attempt,
				"err", err,
			)
			time.Sleep(seedRetryInterval)
		}
		logger.Info("oauth2 test provider seeded",
			"base_url", cfg.OAuth2TestProvider.BaseUrl,
			"clients", len(cfg.OAuth2TestProvider.Clients),
			"users", len(cfg.OAuth2TestProvider.Users),
			"resource_policies", len(cfg.OAuth2TestProvider.ResourcePolicies),
			"api_key_resource_policies", len(cfg.OAuth2TestProvider.APIKeyResourcePolicies),
		)
	}

	for _, connector := range cfg.Connectors {
		deadline := time.Now().Add(seedRetryTimeout)
		var action connectorAction
		for attempt := 1; ; attempt++ {
			action, err = upsertConnector(client, s.adminApiUrl, connector)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("upsert connector %q after %s: %w", connector.Metadata.Name, seedRetryTimeout, err)
			}
			logger.Warn("connector seed attempt failed; retrying",
				"name", connector.Metadata.Name,
				"namespace", connector.Metadata.Namespace,
				"attempt", attempt,
				"err", err,
			)
			time.Sleep(seedRetryInterval)
		}

		switch action {
		case connectorCreated:
			logger.Info("connector created", "name", connector.Metadata.Name, "namespace", connector.Metadata.Namespace)
		case connectorUpdated:
			logger.Info("connector updated", "name", connector.Metadata.Name, "namespace", connector.Metadata.Namespace)
		case connectorAlreadyPresent:
			logger.Info("connector already present", "name", connector.Metadata.Name, "namespace", connector.Metadata.Namespace)
		}
	}
	return nil
}

func main() {
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("seed failed", "err", err)
		os.Exit(1)
	}
	logger.Info("seed complete")
}
