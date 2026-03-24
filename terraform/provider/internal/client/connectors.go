package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Connector struct {
	Id          string            `json:"id"`
	Version     uint64            `json:"version"`
	Namespace   string            `json:"namespace"`
	State       string            `json:"state"`
	DisplayName string            `json:"display_name"`
	Highlight   string            `json:"highlight,omitempty"`
	Description string            `json:"description"`
	Logo        string            `json:"logo"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Versions    int64             `json:"versions,omitempty"`
}

type ConnectorVersion struct {
	Id         string            `json:"id"`
	Version    uint64            `json:"version"`
	Namespace  string            `json:"namespace"`
	State      string            `json:"state"`
	Definition json.RawMessage   `json:"definition"`
	Labels     map[string]string `json:"labels,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type CreateConnectorRequest struct {
	Namespace  string            `json:"namespace"`
	Definition json.RawMessage   `json:"definition"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type UpdateConnectorRequest struct {
	Definition *json.RawMessage   `json:"definition,omitempty"`
	Labels     *map[string]string `json:"labels,omitempty"`
}

type CreateConnectorVersionRequest struct {
	Definition *json.RawMessage   `json:"definition,omitempty"`
	Labels     *map[string]string `json:"labels,omitempty"`
}

type ForceStateRequest struct {
	State string `json:"state"`
}

type ListConnectorVersionsResponse struct {
	Items  []ConnectorVersion `json:"items"`
	Cursor string             `json:"cursor,omitempty"`
}

func (c *Client) CreateConnector(ctx context.Context, req CreateConnectorRequest) (*ConnectorVersion, error) {
	var cv ConnectorVersion
	err := c.post(ctx, "/api/v1/connectors", req, &cv)
	return &cv, err
}

func (c *Client) GetConnector(ctx context.Context, id string) (*Connector, error) {
	var conn Connector
	err := c.get(ctx, fmt.Sprintf("/api/v1/connectors/%s", id), &conn)
	return &conn, err
}

func (c *Client) GetConnectorVersion(ctx context.Context, id string, version uint64) (*ConnectorVersion, error) {
	var cv ConnectorVersion
	err := c.get(ctx, fmt.Sprintf("/api/v1/connectors/%s/versions/%d", id, version), &cv)
	return &cv, err
}

func (c *Client) UpdateConnector(ctx context.Context, id string, req UpdateConnectorRequest) (*ConnectorVersion, error) {
	var cv ConnectorVersion
	err := c.patch(ctx, fmt.Sprintf("/api/v1/connectors/%s", id), req, &cv)
	return &cv, err
}

func (c *Client) UpdateConnectorVersion(ctx context.Context, id string, version uint64, req UpdateConnectorRequest) (*ConnectorVersion, error) {
	var cv ConnectorVersion
	err := c.patch(ctx, fmt.Sprintf("/api/v1/connectors/%s/versions/%d", id, version), req, &cv)
	return &cv, err
}

func (c *Client) CreateConnectorVersion(ctx context.Context, id string, req CreateConnectorVersionRequest) (*ConnectorVersion, error) {
	var cv ConnectorVersion
	err := c.post(ctx, fmt.Sprintf("/api/v1/connectors/%s/versions", id), req, &cv)
	return &cv, err
}

func (c *Client) ForceConnectorVersionState(ctx context.Context, id string, version uint64, state string) error {
	return c.put(ctx, fmt.Sprintf("/api/v1/connectors/%s/versions/%d/_force_state", id, version), ForceStateRequest{State: state}, nil)
}

func (c *Client) ListConnectorVersions(ctx context.Context, id string) (*ListConnectorVersionsResponse, error) {
	var resp ListConnectorVersionsResponse
	err := c.get(ctx, fmt.Sprintf("/api/v1/connectors/%s/versions", id), &resp)
	return &resp, err
}
