package main

import (
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/cmd/cli/config"
	"github.com/rmorlok/authproxy/internal/api_common"
	routes2 "github.com/rmorlok/authproxy/internal/routes"
	"github.com/spf13/cobra"
)

func cmdListConnectors() *cobra.Command {
	var (
		resolver *config.Resolver
		out      Output[routes2.ConnectorJson]

		state string
		typ   string
		order string
	)

	cmd := &cobra.Command{
		Use:   "connectors",
		Short: "List connectors ",
		RunE: func(cmd *cobra.Command, args []string) error {
			signer, err := resolver.ResolveSigner()
			if err != nil {
				return err
			}

			apiUrl, err := resolver.ResolveApiUrl()
			if err != nil {
				return err
			}

			if apiUrl == "" {
				return errors.New("api url not specified")
			}

			connectionsUrl := fmt.Sprintf("%s/api/v1/connectors", apiUrl)

			client := resty.New()

			var response routes2.ListConnectorsResponseJson
			var apiErr api_common.ErrorResponse
			var resp *resty.Response

			req := signer.SignRestyRequest(client.R()).
				SetResult(&response).
				SetError(&apiErr)

			if typ != "" {
				req.SetQueryParam("type", typ)
			}
			if state != "" {
				req.SetQueryParam("state", state)
			}
			if order != "" {
				req.SetQueryParam("order_by", order)
			}

			resp, err = req.Get(connectionsUrl)

			if err != nil {
				return err
			} else if resp.IsError() {
				return out.ErrorResponse(resp)
			}

			defer out.Done()
			out.EmitAll(response.Items)

			for response.Cursor != "" && !out.ShouldStop() {
				resp, err = signer.SignRestyRequest(client.R()).
					SetResult(&response).
					SetError(&apiErr).
					SetQueryParam("cursor", response.Cursor).
					Get(connectionsUrl)
				if err != nil {
					return err
				} else if resp.IsError() {
					return errors.New(apiErr.Error)
				}
			}

			return nil
		},
	}

	resolver = config.WithConfigParams(cmd)
	out = OutputMultiple[routes2.ConnectorJson](cmd)

	cmd.Flags().StringVar(&state, "state", "", "Only show connectors in the specified state")
	cmd.Flags().StringVar(&typ, "type", "", "Only show connectors of the specified type")
	cmd.Flags().StringVar(&order, "order", "", "Order records by the specified field. Should be of the form \"field DESC|ASC\".")

	return cmd
}
