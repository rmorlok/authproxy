package main

import (
	"fmt"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

func cmdVerifyJwt() *cobra.Command {
	var (
		publicKeyPath string
		secretKeyPath string
	)

	cmd := &cobra.Command{
		Use:   "verify-jwt",
		Short: "Verify a JWT",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token := args[0]

			if publicKeyPath == "" {
				return fmt.Errorf("must specify public key to verify JWT")
			}

			pb := jwt.NewJwtTokenParserBuilder()

			if publicKeyPath != "" {
				pb = pb.WithPublicKeyPath(publicKeyPath)
			} else if secretKeyPath != "" {
				pb = pb.WithSharedKeyPath(secretKeyPath)
			}

			result, err := pb.Parse(token)
			if err != nil {
				if strings.Contains(err.Error(), "signature is invalid") {
					fmt.Fprintf(os.Stderr, "%v\n", err)
					os.Exit(1)
				}

				return err
			}

			util.MustPrettyPrintJSON(result)

			return nil
		},
	}

	cmd.Flags().StringVar(&publicKeyPath, "publicKeyPath", "", "Public key to use to verify JWT")
	cmd.Flags().StringVar(&secretKeyPath, "secretKeyPath", "", "Secret key to use to sign request")
	cmd.MarkFlagsMutuallyExclusive("publicKeyPath", "secretKeyPath")

	return cmd
}
