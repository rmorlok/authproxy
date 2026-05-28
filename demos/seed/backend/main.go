// Command demo-seed bootstraps the demo environment's actors + connectors
// against a running AuthProxy admin API. Run as a Helm post-install /
// post-upgrade hook from the authproxy-demo umbrella chart.
//
// Idempotency model: for each desired entity, first GET it by
// external_id; if AuthProxy returns 404, POST it. Re-running the seed
// job is a no-op once the state matches.
//
// Auth: signs requests as the demo-shell admin actor using the same
// keypair the demo-shell itself uses. AuthProxy already trusts that
// actor to create/list other actors via the admin-api access scope.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
	Actors []ActorSeed `yaml:"actors"`
}

type ActorSeed struct {
	ExternalId  string            `yaml:"external_id"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

type settings struct {
	adminApiUrl         string
	adminUsername       string
	adminPrivateKeyPath string
	configPath          string
}

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

	for _, a := range cfg.Actors {
		created, err := upsertActor(client, s.adminApiUrl, a)
		if err != nil {
			return err
		}
		if created {
			logger.Info("actor created", "external_id", a.ExternalId, "namespace", a.Namespace)
		} else {
			logger.Info("actor already present", "external_id", a.ExternalId, "namespace", a.Namespace)
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
