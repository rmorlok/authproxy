package config

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const grafanaDefaultExpiresIn = 90 * 24 * time.Hour

// Resolver is an interface that will pull config information from a combination of defaults, config file, and
// command line arguments. These are the config settings for how the client interacts with AuthProxy.
type Resolver struct {
	root           *Root
	admin          bool
	actorId        string
	privateKeyPath string
	secretKeyPath  string
	apis           string
	configFile     string
	apiUrl         string
	adminApiUrl    string
	authUrl        string
	marketplaceUrl string
	adminUiUrl     string

	expiresIn       string
	noExpiry        bool
	grafanaPreset   string
	permissionsFile string
}

func WithConfigParams(cmd *cobra.Command) *Resolver {
	r := Resolver{}

	cmd.Flags().BoolVar(&r.admin, "admin", false, "Sign the request as an admin")
	cmd.Flags().StringVar(&r.actorId, "actorId", "", "ActorID/username to sign the request as. For admin requests, defaults to current OS user")
	cmd.Flags().StringVar(&r.apis, "apis", "all", fmt.Sprintf("Service identifiers to sign the token for. Comma separted list. Possibly values: %s or 'all' for all services", strings.Join(config.AllServiceIdStrings(), ", ")))

	cmd.Flags().StringVar(&r.privateKeyPath, "privateKeyPath", "", "Private key to use to sign request")
	cmd.Flags().StringVar(&r.secretKeyPath, "secretKeyPath", "", "Secret key to use to sign request")
	cmd.MarkFlagsMutuallyExclusive("privateKeyPath", "secretKeyPath")

	cmd.Flags().StringVar(&r.authUrl, "authUrl", "", "Auth service base URL")
	cmd.Flags().StringVar(&r.apiUrl, "apiUrl", "", "API service base URL")
	cmd.Flags().StringVar(&r.adminApiUrl, "adminApiUrl", "", "Admin API service base URL")
	cmd.Flags().StringVar(&r.marketplaceUrl, "marketplaceUrl", "", "Marketplace service base URL")
	cmd.Flags().StringVar(&r.adminUiUrl, "adminUiUrl", "", "Admin UI service base URL")

	cmd.Flags().StringVar(&r.configFile, "config", "", ".authproxy.yaml config file to use.")

	return &r
}

func (j *Resolver) AddTokenScopeParams(cmd *cobra.Command) {
	cmd.Flags().StringVar(&j.expiresIn, "expires-in", "", "Token lifetime, e.g. 24h, 90d, 2160h")
	cmd.Flags().BoolVar(&j.noExpiry, "no-expiry", false, "Do not set an expiration on the signed token")
	cmd.Flags().StringVar(&j.grafanaPreset, "grafana-preset", "", "Grafana datasource permission preset: aggregate or logs")
	cmd.Flags().StringVar(&j.permissionsFile, "permissions-file", "", "YAML or JSON file containing token permission restrictions")
	cmd.MarkFlagsMutuallyExclusive("expires-in", "no-expiry")
}

func (j *Resolver) resolveRoot() (*Root, error) {
	if j.root != nil {
		return j.root, nil
	}

	explicitConfig := false
	configPath := "~/.authproxy.yaml"

	if j.configFile != "" {
		explicitConfig = true
		configPath = j.configFile
	}

	_, err := os.Stat(configPath)
	if err != nil {
		// attempt home path expansion
		configPath, err = homedir.Expand(configPath)
		if err != nil {
			if explicitConfig {
				return nil, fmt.Errorf("invalid config file path '%s': %w", j.configFile, err)
			} else {
				// Was not explicitly configured, so ignore
				return nil, nil
			}
		}
	}

	_, err = os.Stat(configPath)
	if err != nil {
		if explicitConfig {
			return nil, fmt.Errorf("config file '%s' does not exist: %w", j.configFile, err)
		} else {
			// Ignore
			return nil, nil
		}
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", j.configFile, err)
	}

	root, err := UnmarshallYamlRoot(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse yaml config file '%s': %w", j.configFile, err)
	}

	j.root = root
	return root, nil
}

func (j *Resolver) ResolveBuilder() (jwt.TokenBuilder, error) {
	actorId := j.actorId

	if actorId == "" {
		root, err := j.resolveRoot()
		if err != nil {
			return nil, err
		}
		if root.AdminUsername() != "" {
			actorId = root.AdminUsername()
		}
	}

	if j.admin && actorId == "" {
		u, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve current user to sign admin jwt: %w", err)
		}

		actorId = u.Username
	}

	if actorId == "" {
		return nil, fmt.Errorf("must specify user id to sign JWT")
	}

	privateKeyPath := j.privateKeyPath
	secretKeyPath := j.secretKeyPath

	if privateKeyPath == "" && secretKeyPath == "" {
		root, err := j.resolveRoot()
		if err != nil {
			return nil, err
		}

		privateKeyPath = root.AdminPrivateKeyPath()
		secretKeyPath = root.AdminSharedKeyPath()
	}

	if privateKeyPath == "" && secretKeyPath == "" {
		return nil, fmt.Errorf("must specify private key or secret key to sign JWT")
	}

	if j.apis == "" {
		return nil, fmt.Errorf("must specify apis to sign JWT")
	}

	serviceStrings := strings.Split(j.apis, ",")
	serviceIds := make([]config.ServiceId, 0, len(serviceStrings))

	if len(serviceStrings) == 1 && serviceStrings[0] == "all" {
		serviceIds = config.AllServiceIds()
	} else {
		for _, serviceString := range serviceStrings {
			serviceId := config.ServiceId(serviceString)
			if !config.IsValidServiceId(serviceId) {
				return nil, fmt.Errorf("invalid service id: %s", serviceString)
			}
			serviceIds = append(serviceIds, serviceId)
		}
	}

	b := jwt.NewJwtTokenBuilder().
		WithActorExternalId(actorId).
		WithActorSigned().
		WithServiceIds(serviceIds)

	permissions, err := j.resolveTokenPermissions()
	if err != nil {
		return nil, err
	}
	if len(permissions) > 0 {
		b = b.WithPermissions(permissions)
	}

	expiresIn, hasExpiration, err := j.resolveTokenExpiration()
	if err != nil {
		return nil, err
	}
	if hasExpiration {
		b = b.WithExpiresIn(expiresIn)
	}

	if privateKeyPath != "" {
		b = b.WithPrivateKeyPath(privateKeyPath)
	} else {
		b = b.WithSecretKeyPath(secretKeyPath)
	}

	return b, nil
}

func (j *Resolver) resolveTokenExpiration() (time.Duration, bool, error) {
	if j.noExpiry {
		return 0, false, nil
	}
	if j.expiresIn != "" {
		d, err := parseTokenDuration(j.expiresIn)
		if err != nil {
			return 0, false, err
		}
		return d, true, nil
	}
	if j.grafanaPreset != "" {
		return grafanaDefaultExpiresIn, true, nil
	}
	return 0, false, nil
}

func parseTokenDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, fmt.Errorf("duration is required")
	}
	if strings.HasSuffix(raw, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(raw, "d") + "h")
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
		}
		return days * 24, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	return d, nil
}

func (j *Resolver) resolveTokenPermissions() ([]aschema.Permission, error) {
	var permissions []aschema.Permission

	if j.grafanaPreset != "" {
		presetPermissions, err := grafanaPresetPermissions(j.grafanaPreset)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, presetPermissions...)
	}

	if j.permissionsFile != "" {
		filePermissions, err := readPermissionsFile(j.permissionsFile)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, filePermissions...)
	}

	for _, permission := range permissions {
		if err := permission.Validate(); err != nil {
			return nil, fmt.Errorf("invalid token permissions: %w", err)
		}
	}

	return permissions, nil
}

func grafanaPresetPermissions(preset string) ([]aschema.Permission, error) {
	switch preset {
	case "aggregate":
		return grafanaAggregatePermissions(), nil
	case "logs":
		return append(grafanaAggregatePermissions(), aschema.Permission{
			Namespace: "root.**",
			Resources: []string{"request-events"},
			Verbs:     []string{"list"},
		}), nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("invalid grafana preset %q; expected aggregate or logs", preset)
	}
}

func grafanaAggregatePermissions() []aschema.Permission {
	return []aschema.Permission{
		{
			Namespace: "root.**",
			Resources: []string{"app-metrics"},
			Verbs:     []string{"schema", "query"},
		},
		{
			Namespace: "root.**",
			Resources: []string{"namespaces", "connectors", "connections", "actors", "rate_limits"},
			Verbs:     []string{"list"},
		},
		{
			Namespace: "root.**",
			Resources: []string{"connectors"},
			Verbs:     []string{"list/versions"},
		},
	}
}

func readPermissionsFile(path string) ([]aschema.Permission, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read permissions file %q: %w", path, err)
	}

	var direct []aschema.Permission
	if err := yaml.Unmarshal(data, &direct); err == nil && len(direct) > 0 {
		return direct, nil
	}

	var wrapped struct {
		Permissions []aschema.Permission `json:"permissions" yaml:"permissions"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("failed to parse permissions file %q: %w", path, err)
	}
	if len(wrapped.Permissions) == 0 {
		return nil, fmt.Errorf("permissions file %q must contain a permissions array", path)
	}
	return wrapped.Permissions, nil
}

func (j *Resolver) ResolveToken() (string, error) {
	b, err := j.ResolveBuilder()
	if err != nil {
		return "", err
	}

	return b.Token()
}

func (j *Resolver) ResolveSigner() (jwt.Signer, error) {
	b, err := j.ResolveBuilder()
	if err != nil {
		return nil, err
	}

	return b.Signer()
}

func (j *Resolver) ResolveAdminApiUrl() (string, error) {
	if j.adminApiUrl != "" {
		return j.adminApiUrl, nil
	}

	root, err := j.resolveRoot()
	if err != nil {
		return "", err
	}

	return root.AdminApiUrl(), nil
}

func (j *Resolver) ResolveApiUrl() (string, error) {
	if j.apiUrl != "" {
		return j.apiUrl, nil
	}

	root, err := j.resolveRoot()
	if err != nil {
		return "", err
	}

	return root.ApiUrl(), nil
}

func (j *Resolver) ResolveAuthUrl() (string, error) {
	if j.authUrl != "" {
		return j.authUrl, nil
	}

	root, err := j.resolveRoot()
	if err != nil {
		return "", err
	}

	return root.AuthUrl(), nil
}

func (j *Resolver) ResolveMarketplaceUrl() (string, error) {
	if j.marketplaceUrl != "" {
		return j.marketplaceUrl, nil
	}

	root, err := j.resolveRoot()
	if err != nil {
		return "", err
	}

	return root.MarketplaceUrl(), nil
}

func (j *Resolver) ResolveAdminUiUrl() (string, error) {
	if j.adminUiUrl != "" {
		return j.adminUiUrl, nil
	}

	root, err := j.resolveRoot()
	if err != nil {
		return "", err
	}

	return root.AdminUiUrl(), nil
}

// ResolveSigningProxyPort returns the configured signing-proxy port from the
// CLI config file, or 0 if unset. Callers should treat 0 as "fall back to the
// command's own --port default."
func (j *Resolver) ResolveSigningProxyPort() (int, error) {
	root, err := j.resolveRoot()
	if err != nil {
		return 0, err
	}

	return root.SigningProxyPort()
}
