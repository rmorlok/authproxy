package main

import (
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/rmorlok/authproxy/cmd/cli/config"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/spf13/cobra"
)

func cmdListConnectors() *cobra.Command {
	var (
		resolver *config.Resolver
		out      Output[cschema.Connector]

		name  string
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

			var response schemaapi.ListConnectorsResponseJson
			var apiErr httperr.ErrorResponse
			var resp *resty.Response

			req := signer.SignRestyRequest(client.R()).
				SetResult(&response).
				SetError(&apiErr)

			setConnectorListQuery(req, name, state, typ, order, "")

			resp, err = req.Get(connectionsUrl)

			if err != nil {
				return err
			} else if resp.IsError() {
				return out.ErrorResponse(resp)
			}

			defer out.Done()
			out.EmitAll(response.Items)

			for response.Metadata.Continue != "" && !out.ShouldStop() {
				cursor := response.Metadata.Continue
				response = schemaapi.ListConnectorsResponseJson{}
				req = signer.SignRestyRequest(client.R()).
					SetResult(&response).
					SetError(&apiErr)
				setConnectorListQuery(req, name, state, typ, order, cursor)
				resp, err = req.Get(connectionsUrl)
				if err != nil {
					return err
				} else if resp.IsError() {
					return errors.New(apiErr.Error)
				}
				out.EmitAll(response.Items)
			}

			return nil
		},
	}

	resolver = config.WithConfigParams(cmd)
	out = OutputMultiple[cschema.Connector](cmd)

	cmd.Flags().StringVar(&name, "name", "", "Only show connectors with this exact name")
	cmd.Flags().StringVar(&state, "state", "", "Only show connectors in the specified state")
	cmd.Flags().StringVar(&typ, "type", "", "Only show connectors of the specified type")
	cmd.Flags().StringVar(&order, "order", "", "Order records by the specified field. Should be of the form \"field DESC|ASC\".")

	return cmd
}

func setConnectorListQuery(req *resty.Request, name, state, typ, order, cursor string) {
	if name != "" {
		req.SetQueryParam("name", name)
	}
	if typ != "" {
		req.SetQueryParam("type", typ)
	}
	if state != "" {
		req.SetQueryParam("state", state)
	}
	if order != "" {
		req.SetQueryParam("orderBy", order)
	}
	if cursor != "" {
		req.SetQueryParam("cursor", cursor)
	}
}
