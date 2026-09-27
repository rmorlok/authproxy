// Command demo-seed prepares the disposable OAuth test provider and invokes ap
// apply for AuthProxy resources. Resource reconciliation belongs to the CLI;
// provider clients, users, and policies are not AuthProxy resources.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rmorlok/authproxy/internal/util"
)

// SeedConfig contains only external test-provider setup. AuthProxy resources
// live in a separate multi-document manifest consumed directly by ap apply.
type SeedConfig struct {
	OAuth2TestProvider *OAuth2TestProviderSeed `yaml:"oauth2TestProvider"`
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
	resourcePath        string
}

const (
	seedRetryTimeout  = 5 * time.Minute
	seedRetryInterval = 5 * time.Second
)

type seedAction string

const (
	seedCreated        seedAction = "created"
	seedAlreadyPresent seedAction = "already-present"
)

func loadSettings() (settings, error) {
	s := settings{
		adminApiUrl:         strings.TrimRight(os.Getenv("ADMIN_API_URL"), "/"),
		adminUsername:       os.Getenv("ADMIN_USERNAME"),
		adminPrivateKeyPath: os.Getenv("ADMIN_PRIVATE_KEY_PATH"),
		configPath:          os.Getenv("SEED_CONFIG_PATH"),
		resourcePath:        os.Getenv("RESOURCE_MANIFEST_PATH"),
	}
	for name, value := range map[string]string{
		"ADMIN_API_URL":          s.adminApiUrl,
		"ADMIN_USERNAME":         s.adminUsername,
		"ADMIN_PRIVATE_KEY_PATH": s.adminPrivateKeyPath,
		"SEED_CONFIG_PATH":       s.configPath,
		"RESOURCE_MANIFEST_PATH": s.resourcePath,
	} {
		if value == "" {
			return s, fmt.Errorf("missing required env var %s", name)
		}
	}
	return s, nil
}

func loadConfig(path string) (*SeedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provider seed config: %w", err)
	}
	var cfg SeedConfig
	if err := util.DecodeYAMLStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse provider seed config: %w", err)
	}
	return &cfg, nil
}

// retry accommodates initial API/provider readiness and bootstrap actor sync.
// Each apply attempt resolves resources by stable namespace/name identities;
// no pruning or destructive replacement is enabled on deployment retries.
func retry(ctx context.Context, logger *slog.Logger, operation string, fn func() error) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		logger.Warn(operation+" failed; retrying", "err", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w (last error: %v)", operation, ctx.Err(), err)
		case <-time.After(seedRetryInterval):
		}
	}
}

func applyArgs(s settings, cliConfig string) []string {
	return []string{
		"apply", "--admin",
		"--adminApiUrl", s.adminApiUrl,
		"--actorId", s.adminUsername,
		"--privateKeyPath", s.adminPrivateKeyPath,
		"--config", cliConfig,
		"--request-timeout", "30s",
		"-f", s.resourcePath,
	}
}

func run(logger *slog.Logger) error {
	s, err := loadSettings()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(s.resourcePath); err != nil {
		return err
	}
	binary, err := exec.LookPath("ap")
	if err != nil {
		return fmt.Errorf("find ap CLI: %w", err)
	}

	// Pin an empty CLI config so container home-directory defaults cannot
	// redirect the target cluster or change the signing identity.
	cliConfig, err := os.CreateTemp("", "demo-cli-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(cliConfig.Name())
	_, writeErr := cliConfig.WriteString("{}\n")
	closeErr := cliConfig.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}

	if cfg.OAuth2TestProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), seedRetryTimeout)
		defer cancel()
		client := resty.New().SetTimeout(30 * time.Second)
		if err := retry(ctx, logger, "provider seed", func() error {
			return seedOAuth2TestProvider(client, *cfg.OAuth2TestProvider)
		}); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedRetryTimeout)
	defer cancel()
	return retry(ctx, logger, "ap apply", func() error {
		cmd := exec.CommandContext(ctx, binary, applyArgs(s, cliConfig.Name())...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	})
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("seed failed", "err", err)
		os.Exit(1)
	}
	logger.Info("seed complete")
}
