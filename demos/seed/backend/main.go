// Command demo-seed bootstraps the demo environment's actors + connectors
// against a running AuthProxy admin API. Run as a Helm post-install /
// post-upgrade hook from the authproxy-demo umbrella chart.
//
// Idempotency model: for each desired actor, first GET it by
// external_id; if AuthProxy returns 404, POST it. For each desired
// connector, list by the stable demo seed label, create it when absent,
// or publish a new version when the definition changes. Re-running the
// seed job is a no-op once the state matches.
//
// Auth: signs requests as the demo-shell admin actor using the same
// keypair the demo-shell itself uses. AuthProxy already trusts that
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
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// SeedConfig is the YAML shape the binary consumes.
type SeedConfig struct {
	Actors             []ActorSeed             `yaml:"actors"`
	OAuth2TestProvider *OAuth2TestProviderSeed `yaml:"oauth2_test_provider"`
	Connectors         []ConnectorSeed         `yaml:"connectors"`
}

type ActorSeed struct {
	ExternalId  string            `yaml:"external_id"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type ConnectorSeed struct {
	// Key is the stable seed identity. AuthProxy generates connector IDs
	// for API-created connectors, so the seed job stores this key as an
	// API label and uses it for future idempotent upgrades.
	Key         string            `yaml:"key"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Definition  config.Connector  `yaml:"definition"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type OAuth2TestProviderSeed struct {
	BaseUrl                string                     `yaml:"base_url"`
	Clients                []OAuth2TestProviderClient `yaml:"clients,omitempty"`
	Users                  []OAuth2TestProviderUser   `yaml:"users,omitempty"`
	ResourcePolicies       []OAuth2ResourcePolicy     `yaml:"resource_policies,omitempty"`
	APIKeyResourcePolicies []APIKeyResourcePolicy     `yaml:"api_key_resource_policies,omitempty"`
}

type OAuth2TestProviderClient struct {
	Key                     string `json:"key" yaml:"key"`
	Secret                  string `json:"secret,omitempty" yaml:"secret,omitempty"`
	RedirectURI             string `json:"redirect_uri,omitempty" yaml:"redirect_uri,omitempty"`
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty" yaml:"token_endpoint_auth_method,omitempty"`
	RequirePKCE             bool   `json:"require_pkce,omitempty" yaml:"require_pkce,omitempty"`
	Scope                   string `json:"scope,omitempty" yaml:"scope,omitempty"`
}

type OAuth2TestProviderUser struct {
	Username    string `json:"username" yaml:"username"`
	Password    string `json:"password,omitempty" yaml:"password,omitempty"`
	Role        string `json:"role,omitempty" yaml:"role,omitempty"`
	Email       string `json:"email,omitempty" yaml:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Sub         string `json:"sub,omitempty" yaml:"sub,omitempty"`
}

type OAuth2ResourcePolicy struct {
	Path          string `json:"path" yaml:"path"`
	RequiredScope string `json:"required_scope" yaml:"required_scope"`
}

type APIKeyResourcePolicy struct {
	Path       string `json:"path" yaml:"path"`
	Key        string `json:"key" yaml:"key"`
	Placement  string `json:"placement,omitempty" yaml:"placement,omitempty"`
	HeaderName string `json:"header_name,omitempty" yaml:"header_name,omitempty"`
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
	seedLabelKey      = "demo.authproxy.net/seed-key"
	defaultNamespace  = "root"
)

type seedAction string

const (
	seedCreated        seedAction = "created"
	seedAlreadyPresent seedAction = "already-present"
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
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse seed config %q: %w", path, err)
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

// upsertActor creates the actor if it doesn't already exist by
// external_id. Returns true when a create was performed, false on
// no-op.
func upsertActor(c *resty.Client, baseUrl string, a ActorSeed) (created bool, err error) {
	// GET by external_id (with optional namespace).
	getReq := c.R().SetHeader("Accept", "application/json")
	if a.Namespace != "" {
		getReq.SetQueryParam("namespace", a.Namespace)
	}
	getResp, err := getReq.Get(fmt.Sprintf("%s/api/v1/actors/external-id/%s", baseUrl, a.ExternalId))
	if err != nil {
		return false, fmt.Errorf("GET actor %q: %w", a.ExternalId, err)
	}

	switch getResp.StatusCode() {
	case http.StatusOK:
		return false, nil
	case http.StatusNotFound:
		// fall through to create
	default:
		return false, fmt.Errorf("GET actor %q returned %d: %s", a.ExternalId, getResp.StatusCode(), getResp.String())
	}

	body := api.CreateActorRequestJson{
		ExternalId:  a.ExternalId,
		Namespace:   a.Namespace,
		Labels:      a.Labels,
		Annotations: a.Annotations,
	}
	postResp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(fmt.Sprintf("%s/api/v1/actors", baseUrl))
	if err != nil {
		return false, fmt.Errorf("POST actor %q: %w", a.ExternalId, err)
	}
	if postResp.StatusCode() >= 400 {
		return false, fmt.Errorf("POST actor %q returned %d: %s", a.ExternalId, postResp.StatusCode(), postResp.String())
	}
	return true, nil
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

func connectorNamespace(seed ConnectorSeed) string {
	if seed.Namespace != "" {
		return seed.Namespace
	}
	if seed.Definition.Namespace != nil && *seed.Definition.Namespace != "" {
		return *seed.Definition.Namespace
	}
	return defaultNamespace
}

func connectorLabels(seed ConnectorSeed) map[string]string {
	labels := make(map[string]string, len(seed.Labels)+1)
	for k, v := range seed.Labels {
		labels[k] = v
	}
	labels[seedLabelKey] = seed.Key
	return labels
}

func connectorDefinitionsEqual(want config.Connector, got api.ConnectorVersionJson) bool {
	namespace := got.Namespace
	normalizedWant := want
	normalizedWant.Id = got.Id
	normalizedWant.Version = got.Version
	normalizedWant.Namespace = &namespace
	normalizedWant.State = string(got.State)

	normalizedGot := got.Definition
	normalizedGot.Id = got.Id
	normalizedGot.Version = got.Version
	normalizedGot.Namespace = &namespace
	normalizedGot.State = string(got.State)

	return reflect.DeepEqual(normalizeForJSON(normalizedWant), normalizeForJSON(normalizedGot))
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

func listSeededConnector(c *resty.Client, baseUrl string, seed ConnectorSeed) (*api.ConnectorJson, error) {
	var list api.ListConnectorsResponseJson
	resp, err := c.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("namespace", connectorNamespace(seed)).
		SetQueryParam("label_selector", fmt.Sprintf("%s=%s", seedLabelKey, seed.Key)).
		SetQueryParam("limit", "1").
		SetResult(&list).
		Get(fmt.Sprintf("%s/api/v1/connectors", baseUrl))
	if err != nil {
		return nil, fmt.Errorf("GET connector seed %q: %w", seed.Key, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("GET connector seed %q returned %d: %s", seed.Key, resp.StatusCode(), resp.String())
	}
	if len(list.Items) == 0 {
		return nil, nil
	}
	return &list.Items[0], nil
}

func getConnectorVersion(c *resty.Client, baseUrl string, connector api.ConnectorJson) (*api.ConnectorVersionJson, error) {
	var version api.ConnectorVersionJson
	resp, err := c.R().
		SetHeader("Accept", "application/json").
		SetResult(&version).
		Get(fmt.Sprintf("%s/api/v1/connectors/%s/versions/%d", baseUrl, connector.Id, connector.Version))
	if err != nil {
		return nil, fmt.Errorf("GET connector version %s:%d: %w", connector.Id, connector.Version, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("GET connector version %s:%d returned %d: %s", connector.Id, connector.Version, resp.StatusCode(), resp.String())
	}
	return &version, nil
}

func createConnector(c *resty.Client, baseUrl string, seed ConnectorSeed) (*api.ConnectorVersionJson, error) {
	body := api.CreateConnectorRequestJson{
		Namespace:   connectorNamespace(seed),
		Definition:  seed.Definition,
		Labels:      connectorLabels(seed),
		Annotations: seed.Annotations,
	}
	var created api.ConnectorVersionJson
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&created).
		Post(fmt.Sprintf("%s/api/v1/connectors", baseUrl))
	if err != nil {
		return nil, fmt.Errorf("POST connector seed %q: %w", seed.Key, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("POST connector seed %q returned %d: %s", seed.Key, resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func createConnectorDraft(c *resty.Client, baseUrl string, connector api.ConnectorJson, seed ConnectorSeed) (*api.ConnectorVersionJson, error) {
	labels := connectorLabels(seed)
	body := api.CreateConnectorVersionRequestJson{
		Definition:  &seed.Definition,
		Labels:      &labels,
		Annotations: &seed.Annotations,
	}
	var created api.ConnectorVersionJson
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&created).
		Post(fmt.Sprintf("%s/api/v1/connectors/%s/versions", baseUrl, connector.Id))
	if err != nil {
		return nil, fmt.Errorf("POST connector seed %q version: %w", seed.Key, err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("POST connector seed %q version returned %d: %s", seed.Key, resp.StatusCode(), resp.String())
	}
	return &created, nil
}

func forceConnectorPrimary(c *resty.Client, baseUrl string, version api.ConnectorVersionJson) error {
	resp, err := c.R().
		SetHeader("Content-Type", "application/json").
		SetBody(api.ForceConnectorVersionStateRequestJson{State: string(api.ConnectorVersionStatePrimary)}).
		Put(fmt.Sprintf("%s/api/v1/connectors/%s/versions/%d/_force_state", baseUrl, version.Id, version.Version))
	if err != nil {
		return fmt.Errorf("PUT connector seed %s:%d primary: %w", version.Id, version.Version, err)
	}
	if resp.StatusCode() >= 400 {
		return fmt.Errorf("PUT connector seed %s:%d primary returned %d: %s", version.Id, version.Version, resp.StatusCode(), resp.String())
	}
	return nil
}

func upsertConnector(c *resty.Client, baseUrl string, seed ConnectorSeed) (connectorAction, error) {
	if seed.Key == "" {
		return "", fmt.Errorf("connector seed key is required")
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
		if created.State != api.ConnectorVersionStatePrimary {
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

	if connectorDefinitionsEqual(seed.Definition, *version) &&
		stringMapsEqual(connectorLabels(seed), version.Labels) &&
		stringMapsEqual(seed.Annotations, version.Annotations) {
		if version.State != api.ConnectorVersionStatePrimary {
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

	for _, a := range cfg.Actors {
		deadline := time.Now().Add(seedRetryTimeout)
		var created bool
		for attempt := 1; ; attempt++ {
			created, err = upsertActor(client, s.adminApiUrl, a)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("upsert actor %q after %s: %w", a.ExternalId, seedRetryTimeout, err)
			}
			logger.Warn("actor seed attempt failed; retrying",
				"external_id", a.ExternalId,
				"namespace", a.Namespace,
				"attempt", attempt,
				"err", err,
			)
			time.Sleep(seedRetryInterval)
		}
		if created {
			logger.Info("actor created", "external_id", a.ExternalId, "namespace", a.Namespace)
		} else {
			logger.Info("actor already present", "external_id", a.ExternalId, "namespace", a.Namespace)
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
				return fmt.Errorf("upsert connector %q after %s: %w", connector.Key, seedRetryTimeout, err)
			}
			logger.Warn("connector seed attempt failed; retrying",
				"key", connector.Key,
				"namespace", connectorNamespace(connector),
				"attempt", attempt,
				"err", err,
			)
			time.Sleep(seedRetryInterval)
		}

		switch action {
		case connectorCreated:
			logger.Info("connector created", "key", connector.Key, "namespace", connectorNamespace(connector))
		case connectorUpdated:
			logger.Info("connector updated", "key", connector.Key, "namespace", connectorNamespace(connector))
		case connectorAlreadyPresent:
			logger.Info("connector already present", "key", connector.Key, "namespace", connectorNamespace(connector))
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
