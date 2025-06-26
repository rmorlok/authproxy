package main

import (
	"fmt"
	"github.com/rmorlok/authproxy/cli/client/config"
	"github.com/spf13/cobra"
	"net/url"
	"time"
)

func cmdSignMarketplaceLoginUrl() *cobra.Command {
	var (
		resolver *config.Resolver
	)

	cmd := &cobra.Command{
		Use:   "sign-marketplace-login-url",
		Short: "Sign a Marketplace login URL",
		Long: `
Signs a marketplace login URL. This generates a URL with a signed JWT auth parameter that will allow the specified user
to access the marketplace SPA. This marketplace URL will be used by the SPA to establish a session for the user.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			marketplaceUrl, err := resolver.ResolveMarketplaceUrl()
			if err != nil {
				return err
			}

			if marketplaceUrl == "" {
				return fmt.Errorf("marketplace url not specified")
			}

			parsedUrl, err := url.Parse(marketplaceUrl)
			if err != nil {
				return fmt.Errorf("failed to parse marketplace URL: %v", err)
			}

			b, err := resolver.ResolveBuilder()
			if err != nil {
				return err
			}

			tok, err := b.
				WithNonce().
				WithExpiresIn(1 * time.Hour).
				Token()

			if err != nil {
				return err
			}

			query := parsedUrl.Query()
			query.Set("auth_token", tok)
			parsedUrl.RawQuery = query.Encode()

			fmt.Print(parsedUrl.String())

			return nil
		},
	}

	resolver = config.WithConfigParams(cmd)

	return cmd
}
