package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const ConnectorKind = "Connector"

type ConnectorReleaseSpec struct {
	DesiredState string `json:"desiredState,omitempty"`
}

type ConnectorSpec struct {
	Release    ConnectorReleaseSpec `json:"release,omitempty"`
	Definition json.RawMessage      `json:"definition"`
}

type ConnectorReleaseStatus struct {
	State string `json:"state"`
}

type ConnectorStatus struct {
	Release ConnectorReleaseStatus `json:"release"`
}

type Connector struct {
	TypeMeta
	Metadata ObjectMetadata   `json:"metadata"`
	Spec     ConnectorSpec    `json:"spec"`
	Status   *ConnectorStatus `json:"status,omitempty"`

	// DataRedacted is transport metadata derived from the response header. It
	// is not part of the resource JSON contract.
	DataRedacted bool `json:"-"`
}

type CreateConnectorRequest struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     ConnectorSpec  `json:"spec"`
}

type ConnectorReleaseSpecPatch struct {
	DesiredState *string `json:"desiredState,omitempty"`
}

type ConnectorSpecPatch struct {
	Release    *ConnectorReleaseSpecPatch `json:"release,omitempty"`
	Definition *json.RawMessage           `json:"definition,omitempty"`
}

type UpdateConnectorRequest struct {
	TypeMeta
	Metadata *ObjectMetadataPatch `json:"metadata"`
	Spec     *ConnectorSpecPatch  `json:"spec"`
}

type CreateConnectorVersionRequest struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     ConnectorSpec  `json:"spec"`
}

type ForceStateRequest struct {
	State string `json:"state"`
}

type ListConnectorVersionsResponse struct {
	ResourceList[Connector]
}

// ConnectorDefinitionSummary contains the definition fields surfaced by the
// Terraform data source in addition to its opaque definition document.
type ConnectorDefinitionSummary struct {
	DisplayName string
	Description string
	Logo        string
}

// DecodeConnectorDefinitionSummary extracts display fields without treating
// resource metadata as part of spec.definition. Logo supports both public URL
// and base64 image representations accepted by the connector schema.
func DecodeConnectorDefinitionSummary(definition json.RawMessage) (ConnectorDefinitionSummary, error) {
	var value struct {
		DisplayName string          `json:"displayName"`
		Description string          `json:"description"`
		Logo        json.RawMessage `json:"logo"`
	}
	if err := json.Unmarshal(definition, &value); err != nil {
		return ConnectorDefinitionSummary{}, err
	}

	result := ConnectorDefinitionSummary{
		DisplayName: value.DisplayName,
		Description: value.Description,
	}
	if len(value.Logo) == 0 || string(value.Logo) == "null" {
		return result, nil
	}
	if value.Logo[0] == '"' {
		if err := json.Unmarshal(value.Logo, &result.Logo); err != nil {
			return ConnectorDefinitionSummary{}, err
		}
		return result, nil
	}
	var image struct {
		PublicURL string `json:"publicUrl"`
		MimeType  string `json:"mimeType"`
		Base64    string `json:"base64"`
	}
	if err := json.Unmarshal(value.Logo, &image); err != nil {
		return ConnectorDefinitionSummary{}, err
	}
	if image.PublicURL != "" {
		result.Logo = image.PublicURL
	} else if image.Base64 != "" {
		if _, err := base64.StdEncoding.DecodeString(image.Base64); err != nil {
			return ConnectorDefinitionSummary{}, fmt.Errorf("decode connector logo: %w", err)
		}
		result.Logo = fmt.Sprintf("data:%s;base64,%s", image.MimeType, image.Base64)
	}
	return result, nil
}

func (c *Client) CreateConnector(ctx context.Context, req CreateConnectorRequest) (*Connector, error) {
	var connector Connector
	err := c.writeConnector(ctx, "POST", "/api/v1/connectors", req, &connector)
	return &connector, err
}

func (c *Client) GetConnector(ctx context.Context, id string) (*Connector, error) {
	var connector Connector
	err := c.readConnector(ctx, fmt.Sprintf("/api/v1/connectors/%s", id), &connector)
	return &connector, err
}

func (c *Client) GetConnectorVersion(ctx context.Context, id string, version uint64) (*Connector, error) {
	var connector Connector
	err := c.readConnector(ctx, fmt.Sprintf("/api/v1/connectors/%s/generations/%d", id, version), &connector)
	return &connector, err
}

func (c *Client) UpdateConnector(ctx context.Context, id string, req UpdateConnectorRequest) (*Connector, error) {
	var connector Connector
	err := c.writeConnector(ctx, "PATCH", fmt.Sprintf("/api/v1/connectors/%s", id), req, &connector)
	return &connector, err
}

func (c *Client) UpdateConnectorVersion(ctx context.Context, id string, version uint64, req UpdateConnectorRequest) (*Connector, error) {
	var connector Connector
	err := c.writeConnector(ctx, "PATCH", fmt.Sprintf("/api/v1/connectors/%s/generations/%d", id, version), req, &connector)
	return &connector, err
}

func (c *Client) CreateConnectorVersion(ctx context.Context, id string, req CreateConnectorVersionRequest) (*Connector, error) {
	var connector Connector
	err := c.writeConnector(ctx, "POST", fmt.Sprintf("/api/v1/connectors/%s/generations", id), req, &connector)
	return &connector, err
}

func (c *Client) ForceConnectorVersionState(ctx context.Context, id string, version uint64, state string) error {
	return c.put(ctx, fmt.Sprintf("/api/v1/connectors/%s/generations/%d/_forceState", id, version), ForceStateRequest{State: state}, nil)
}

func (c *Client) ListConnectorVersions(ctx context.Context, id string) (*ListConnectorVersionsResponse, error) {
	var response ListConnectorVersionsResponse
	resp, err := c.http.R().SetContext(ctx).SetResult(&response).
		Get(fmt.Sprintf("/api/v1/connectors/%s/generations", id))
	if err != nil {
		return &response, err
	}
	redacted := resp.Header().Get("X-AuthProxy-Data-Redacted") == "true"
	for i := range response.Items {
		response.Items[i].DataRedacted = redacted
	}
	err = checkResponse(resp)
	return &response, err
}

func (c *Client) readConnector(ctx context.Context, path string, result *Connector) error {
	resp, err := c.http.R().SetContext(ctx).SetResult(result).Get(path)
	if err != nil {
		return err
	}
	result.DataRedacted = resp.Header().Get("X-AuthProxy-Data-Redacted") == "true"
	return checkResponse(resp)
}

func (c *Client) writeConnector(ctx context.Context, method, path string, body any, result *Connector) error {
	resp, err := c.http.R().SetContext(ctx).SetBody(body).SetResult(result).Execute(method, path)
	if err != nil {
		return err
	}
	result.DataRedacted = resp.Header().Get("X-AuthProxy-Data-Redacted") == "true"
	return checkResponse(resp)
}
