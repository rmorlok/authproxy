package main

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/auth"
	"github.com/spf13/cobra"
	"os/user"
)

func cmdSignJwt() *cobra.Command {
	var (
		admin          bool
		userId         string
		privateKeyPath string
		secretKeyPath  string
	)

	cmd := &cobra.Command{
		Use:   "sign-jwt",
		Short: "Sign a JWT",
		RunE: func(cmd *cobra.Command, args []string) error {
			if admin && userId == "" {
				user, err := user.Current()
				if err != nil {
					return errors.Wrap(err, "failed to retrieve current user to sign admin jwt")
				}

				userId = user.Username
			}

			if userId == "" {
				return fmt.Errorf("must specify user id to sign JWT")
			}

			if privateKeyPath == "" {
				return fmt.Errorf("must specify private key to sign JWT")
			}

			b := auth.NewJwtTokenBuilder().
				WithActorId(userId)

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
	cmd.Flags().StringVar(&userId, "actorId", "", "ActorID/username to sign the request as. For admin requests, defaults to current OS user")

	cmd.Flags().StringVar(&privateKeyPath, "privateKeyPath", "", "Private key to use to sign request")
	cmd.Flags().StringVar(&secretKeyPath, "secretKeyPath", "", "Secret key to use to sign request")
	cmd.MarkFlagsMutuallyExclusive("privateKeyPath", "secretKeyPath")

	return cmd
}
