package config

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/spf13/cobra"
)

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
				return nil, errors.Wrapf(err, "invalid config file path '%s'", j.configFile)
			} else {
				// Was not explicitly configured, so ignore
				return nil, nil
			}
		}
	}

	_, err = os.Stat(configPath)
	if err != nil {
		if explicitConfig {
			return nil, errors.Wrapf(err, "config file '%s' does not exist", j.configFile)
		} else {
			// Ignore
			return nil, nil
		}
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file '%s'", j.configFile)
	}

	root, err := UnmarshallYamlRoot(content)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse yaml config file '%s'", j.configFile)
	}

	j.root = root
	return root, nil
}

func (j *Resolver) ResolveBuilder() (jwt.TokenBuilder, error) {
	admin := j.admin
	actorId := j.actorId

	if actorId == "" {
		root, err := j.resolveRoot()
		if err != nil {
			return nil, err
		}
		if root.AdminUsername() != "" {
			admin = true
			actorId = root.AdminUsername()
		}
	}

	if admin && actorId == "" {
		u, err := user.Current()
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve current user to sign admin jwt")
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
		WithActorId(actorId).
		WithServiceIds(serviceIds)

	if privateKeyPath != "" {
		b = b.WithPrivateKeyPath(privateKeyPath)
	} else {
		b = b.WithSecretKeyPath(secretKeyPath)
	}

	if admin {
		b = b.WithAdmin()
	}

	return b, nil
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
