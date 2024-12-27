package main

import (
	"fmt"
	"github.com/rmorlok/authproxy/cli/client/config"
	"github.com/spf13/cobra"
)

func cmdSignJwt() *cobra.Command {
	var (
		resolver *config.Resolver
	)

	cmd := &cobra.Command{
		Use:   "sign-jwt",
		Short: "Sign a JWT",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolver.ResolveToken()
			if err != nil {
				return err
			}

			fmt.Print(token)

			return nil
		},
	}

	resolver = config.WithConfigParams(cmd)

	return cmd
}
