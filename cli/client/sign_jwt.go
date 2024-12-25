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

func cmdSignJwt() *cobra.Command {
	var (
		admin          bool
		actorId        string
		privateKeyPath string
		secretKeyPath  string
		apis           string
	)

	cmd := &cobra.Command{
		Use:   "sign-jwt",
		Short: "Sign a JWT",
		RunE: func(cmd *cobra.Command, args []string) error {
			if admin && actorId == "" {
				user, err := user.Current()
				if err != nil {
					return errors.Wrap(err, "failed to retrieve current user to sign admin jwt")
				}

				actorId = user.Username
			}

			if actorId == "" {
				return fmt.Errorf("must specify user id to sign JWT")
			}

			if privateKeyPath == "" {
				return fmt.Errorf("must specify private key to sign JWT")
			}

			if apis == "" {
				return fmt.Errorf("must specify apis to sign JWT")
			}

			serviceStrings := strings.Split(apis, ",")
			serviceIds := make([]config.ServiceId, 0, len(serviceStrings))

			if len(serviceStrings) == 1 && serviceStrings[0] == "all" {
				serviceIds = config.AllServiceIds()
			} else {
				for _, serviceString := range serviceStrings {
					serviceId := config.ServiceId(serviceString)
					if !config.IsValidServiceId(serviceId) {
						return fmt.Errorf("invalid service id: %s", serviceString)
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

			token, err := b.Token()
			if err != nil {
				return err
			}

			fmt.Print(token)

			return nil
		},
	}

	cmd.Flags().BoolVar(&admin, "admin", false, "Sign the request as an admin")
	cmd.Flags().StringVar(&actorId, "actorId", "", "ActorID/username to sign the request as. For admin requests, defaults to current OS user")
	cmd.Flags().StringVar(&apis, "apis", "", fmt.Sprintf("Service identifiers to sign the token for. Comma separted list. Possibly values: %s or 'all' for all services", strings.Join(config.AllServiceIdStrings(), ", ")))

	cmd.Flags().StringVar(&privateKeyPath, "privateKeyPath", "", "Private key to use to sign request")
	cmd.Flags().StringVar(&secretKeyPath, "secretKeyPath", "", "Secret key to use to sign request")
	cmd.MarkFlagsMutuallyExclusive("privateKeyPath", "secretKeyPath")

	return cmd
}
