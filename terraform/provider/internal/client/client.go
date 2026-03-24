package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client is the AuthProxy admin API HTTP client.
type Client struct {
	http    *resty.Client
	baseURL string
}

// Config holds the configuration for creating a new Client.
type Config struct {
	Endpoint       string
	BearerToken    string
	PrivateKeyPath string
	Username       string
}

// New creates a new AuthProxy API client.
func New(cfg Config) (*Client, error) {
	token := cfg.BearerToken

	if token == "" && cfg.PrivateKeyPath != "" {
		var err error
		token, err = signJWT(cfg.PrivateKeyPath, cfg.Username)
		if err != nil {
			return nil, fmt.Errorf("failed to sign JWT: %w", err)
		}
	}

	if token == "" {
		return nil, fmt.Errorf("either bearer_token or private_key_path must be set")
	}

	httpClient := resty.New().
		SetBaseURL(cfg.Endpoint).
		SetAuthToken(token).
		SetHeader("Content-Type", "application/json").
		SetTimeout(30 * time.Second)

	return &Client{
		http:    httpClient,
		baseURL: cfg.Endpoint,
	}, nil
}

// apiError is the JSON error response from the AuthProxy API.
type apiError struct {
	Error string `json:"error"`
}

// checkResponse checks the HTTP response for errors and returns an APIError if appropriate.
func checkResponse(resp *resty.Response) error {
	if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
		return nil
	}

	msg := resp.String()
	var errResp apiError
	if err := json.Unmarshal(resp.Body(), &errResp); err == nil && errResp.Error != "" {
		msg = errResp.Error
	}

	return &APIError{
		StatusCode: resp.StatusCode(),
		Message:    msg,
	}
}

// get performs a GET request and unmarshals the response.
func (c *Client) get(_ context.Context, path string, result interface{}) error {
	resp, err := c.http.R().SetResult(result).Get(path)
	if err != nil {
		return err
	}
	return checkResponse(resp)
}

// post performs a POST request and unmarshals the response.
func (c *Client) post(_ context.Context, path string, body, result interface{}) error {
	resp, err := c.http.R().SetBody(body).SetResult(result).Post(path)
	if err != nil {
		return err
	}
	return checkResponse(resp)
}

// patch performs a PATCH request and unmarshals the response.
func (c *Client) patch(_ context.Context, path string, body, result interface{}) error {
	resp, err := c.http.R().SetBody(body).SetResult(result).Patch(path)
	if err != nil {
		return err
	}
	return checkResponse(resp)
}

// put performs a PUT request and unmarshals the response.
func (c *Client) put(_ context.Context, path string, body, result interface{}) error {
	resp, err := c.http.R().SetBody(body).SetResult(result).Put(path)
	if err != nil {
		return err
	}
	return checkResponse(resp)
}

// delete performs a DELETE request.
func (c *Client) delete(_ context.Context, path string) error {
	resp, err := c.http.R().Delete(path)
	if err != nil {
		return err
	}
	return checkResponse(resp)
}
