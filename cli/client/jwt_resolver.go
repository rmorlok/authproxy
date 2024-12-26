package main

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/spf13/cobra"
	"os/user"
	"strings"
)

type jwtResolver struct {
	admin          bool
	actorId        string
	privateKeyPath string
	secretKeyPath  string
	apis           string
}

func withJwtParams(cmd *cobra.Command) *jwtResolver {
	r := jwtResolver{}

	cmd.Flags().BoolVar(&r.admin, "admin", false, "Sign the request as an admin")
	cmd.Flags().StringVar(&r.actorId, "actorId", "", "ActorID/username to sign the request as. For admin requests, defaults to current OS user")
	cmd.Flags().StringVar(&r.apis, "apis", "all", fmt.Sprintf("Service identifiers to sign the token for. Comma separted list. Possibly values: %s or 'all' for all services", strings.Join(config.AllServiceIdStrings(), ", ")))

	cmd.Flags().StringVar(&r.privateKeyPath, "privateKeyPath", "", "Private key to use to sign request")
	cmd.Flags().StringVar(&r.secretKeyPath, "secretKeyPath", "", "Secret key to use to sign request")
	cmd.MarkFlagsMutuallyExclusive("privateKeyPath", "secretKeyPath")

	return &r
}

func (j *jwtResolver) ResolveBuilder() (jwt.TokenBuilder, error) {
	if j.admin && j.actorId == "" {
		user, err := user.Current()
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve current user to sign admin jwt")
		}

		j.actorId = user.Username
	}

	if j.actorId == "" {
		return nil, fmt.Errorf("must specify user id to sign JWT")
	}

	if j.privateKeyPath == "" {
		return nil, fmt.Errorf("must specify private key to sign JWT")
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
		WithActorId(j.actorId).
		WithServiceIds(serviceIds)

	if j.privateKeyPath != "" {
		b = b.WithPrivateKeyPath(j.privateKeyPath)
	} else {
		b = b.WithSecretKeyPath(j.secretKeyPath)
	}

	if j.admin {
		b = b.WithAdmin()
	}

	return b, nil
}

func (j *jwtResolver) ResolveToken() (string, error) {
	b, err := j.ResolveBuilder()
	if err != nil {
		return "", err
	}

	return b.Token()
}

func (j *jwtResolver) ResolveSigner() (jwt.Signer, error) {
	b, err := j.ResolveBuilder()
	if err != nil {
		return nil, err
	}

	return b.Signer()
}
