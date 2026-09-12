package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apserde"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

const DefaultRequestTimeout = 30 * time.Second

// ClientOptions selects a single cluster service. Timeout zero disables the
// deadline; CLI callers should use DefaultRequestTimeout unless overridden.
type ClientOptions struct {
	APIURL      string
	AdminAPIURL string
	Admin       bool
	Signer      jwt.Signer
	Timeout     time.Duration
}

// Client performs ordinary REST operations. It does not reconcile, retry
// mutations, or imply transactional batch semantics.
type Client struct {
	baseURL string
	admin   bool
	signer  jwt.Signer
	http    *http.Client
	scheme  *registry.ResourceScheme
}

func NewClient(options ClientOptions) (*Client, error) {
	endpoint := options.APIURL
	if options.Admin {
		endpoint = options.AdminAPIURL
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("a valid %s service URL without credentials, query or fragment is required", map[bool]string{false: "API", true: "admin API"}[options.Admin])
	}
	if options.Signer == nil {
		return nil, fmt.Errorf("request signer is required")
	}
	if options.Timeout < 0 {
		return nil, fmt.Errorf("request timeout cannot be negative")
	}
	return &Client{baseURL: strings.TrimRight(endpoint, "/") + "/api/v1", admin: options.Admin, signer: options.Signer, scheme: registry.NewResourceScheme(), http: &http.Client{Timeout: options.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// APIError exposes the status without echoing response bodies that may contain
// sensitive submitted values. It remains inspectable with errors.As.
type APIError struct{ StatusCode int }

func (e *APIError) Error() string {
	return fmt.Sprintf("AuthProxy API returned HTTP %d (%s)", e.StatusCode, http.StatusText(e.StatusCode))
}

// LiveResource contains a typed resource and transport redaction information.
// Resource may contain secrets and must not be logged. Redacted means some
// readable values were masked; WriteOnlyFields cannot be compared from GET.
type LiveResource struct {
	Resource        any
	Metadata        meta.ObjectMeta
	Kind            meta.Kind
	Redacted        bool
	WriteOnlyFields []string
}

// Target is a resolved document. Current nil means a namespaced-name lookup
// found no resource; explicit IDs and generation targets never become creates.
type Target struct {
	Document Document
	Current  *LiveResource
}

func (c *Client) checkKind(kind meta.Kind) error {
	if _, err := resourceType(kind); err != nil {
		return err
	}
	if kind == "Key" && !c.admin {
		return fmt.Errorf("Key resources require admin mode and an admin API endpoint")
	}
	return nil
}

// ResolveBatch resolves and validates the complete batch without mutations.
// Check every kind before any requests, then detect aliases resolving to one ID.
func (c *Client) ResolveBatch(ctx context.Context, documents []Document) ([]Target, error) {
	for _, doc := range documents {
		if err := c.checkKind(doc.Kind); err != nil {
			return nil, fmt.Errorf("%s: %w", doc.Source, err)
		}
	}
	var targets []Target
	seen := map[string]string{}
	for _, doc := range documents {
		live, err := c.Resolve(ctx, doc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc.Source, err)
		}
		target := Target{Document: doc, Current: live}
		if live == nil {
			if _, err := c.createBody(doc); err != nil {
				return nil, fmt.Errorf("%s: %w", doc.Source, err)
			}
		} else {
			identity := string(live.Kind) + "/" + live.Metadata.ID
			if previous, ok := seen[identity]; ok {
				return nil, fmt.Errorf("%s: duplicate live target; also defined at %s", doc.Source, previous)
			}
			seen[identity] = doc.Source
			data, err := patchDocument(doc)
			if err != nil {
				return nil, err
			}
			if _, err := decodePatch(doc.Kind, data, live.Resource); err != nil {
				return nil, fmt.Errorf("%s: %w", doc.Source, err)
			}
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// Resolve looks up an ID or exact namespaced name. List filtering is repeated
// locally because server namespace filters may include descendant namespaces.
func (c *Client) Resolve(ctx context.Context, doc Document) (*LiveResource, error) {
	if err := c.checkKind(doc.Kind); err != nil {
		return nil, err
	}
	adapter, _ := resourceType(doc.Kind)
	m := doc.Metadata
	if err := normalizeIdentity(string(doc.Kind), &m, ""); err != nil {
		return nil, err
	}
	id := m.ID
	if doc.Kind == "Namespace" && id == "" {
		id, _ = namespace.PathFromMetadata(m)
	}
	var live *LiveResource
	var err error
	if id != "" {
		live, err = c.get(ctx, doc.Kind, adapter.Collection+"/"+url.PathEscape(id))
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && doc.Metadata.ID == "" && doc.Kind == "Namespace" {
			return nil, nil
		}
	} else {
		live, err = c.findByName(ctx, doc.Kind, adapter.Collection, m)
	}
	if err != nil {
		return nil, err
	}
	if live == nil {
		if doc.Kind == "Connection" || m.Generation != 0 {
			return nil, fmt.Errorf("existing %s target was not found", doc.Kind)
		}
		return nil, nil
	}
	if (m.ID != "" && m.ID != live.Metadata.ID) || (m.Name != "" && m.Name != live.Metadata.Name) || (m.Namespace != "" && m.Namespace != live.Metadata.Namespace) {
		return nil, fmt.Errorf("resolved resource does not match supplied identity")
	}
	if m.Generation != 0 {
		resolvedID := live.Metadata.ID
		live, err = c.get(ctx, doc.Kind, adapter.Collection+"/"+url.PathEscape(live.Metadata.ID)+"/generations/"+strconv.FormatUint(m.Generation, 10))
		if err != nil {
			return nil, err
		}
		if live.Metadata.ID != resolvedID || live.Metadata.Generation != m.Generation || (m.ID != "" && m.ID != live.Metadata.ID) || (m.Name != "" && m.Name != live.Metadata.Name) || (m.Namespace != "" && m.Namespace != live.Metadata.Namespace) {
			return nil, fmt.Errorf("resolved generation does not match supplied identity")
		}
	}
	return live, nil
}

func (c *Client) findByName(ctx context.Context, kind meta.Kind, collection string, m meta.ObjectMeta) (*LiveResource, error) {
	query := url.Values{"namespace": {m.Namespace}, "name": {string(m.Name)}}
	seen := map[string]bool{}
	var found *LiveResource
	for {
		data, redacted, err := c.request(ctx, http.MethodGet, collection, query, nil)
		if err != nil {
			return nil, err
		}
		var page apiv1alpha1.ResourceList[json.RawMessage]
		if err := util.DecodeJSONStrict(data, &page); err != nil {
			return nil, fmt.Errorf("invalid resource list response")
		}
		if page.APIVersion != meta.APIVersionV1Alpha1 || page.Kind != apiv1alpha1.ListKind(kind) || page.Items == nil {
			return nil, fmt.Errorf("unexpected resource list response")
		}
		for _, item := range page.Items {
			live, err := c.decodeLive(kind, item, redacted)
			if err != nil {
				return nil, err
			}
			if live.Metadata.Namespace != m.Namespace || live.Metadata.Name != m.Name {
				continue
			}
			if found != nil && found.Metadata.ID != live.Metadata.ID {
				return nil, fmt.Errorf("ambiguous namespaced resource name")
			}
			found = live
		}
		cursor := page.Metadata.Continue
		if cursor == "" {
			if page.Metadata.RemainingItemCount != nil && *page.Metadata.RemainingItemCount > 0 {
				return nil, fmt.Errorf("incomplete resource list response")
			}
			return found, nil
		}
		if seen[cursor] {
			return nil, fmt.Errorf("resource list repeated a pagination cursor")
		}
		seen[cursor] = true
		query = url.Values{"cursor": {cursor}}
	}
}

func (c *Client) get(ctx context.Context, kind meta.Kind, path string) (*LiveResource, error) {
	data, redacted, err := c.request(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	return c.decodeLive(kind, data, redacted)
}
func (c *Client) decodeLive(kind meta.Kind, data []byte, redacted bool) (*LiveResource, error) {
	resource, err := c.scheme.DecodeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("invalid %s response fields or types", kind)
	}
	m, actual, err := resourceMetadata(resource)
	if err != nil || actual != kind || m.ID == "" {
		return nil, fmt.Errorf("unexpected resource response identity")
	}
	if m.Name == "" || (kind != "Namespace" && m.Namespace == "") {
		return nil, fmt.Errorf("incomplete response identity")
	}
	normalized := m
	if err := normalizeIdentity(string(kind), &normalized, ""); err != nil {
		return nil, fmt.Errorf("invalid response identity")
	}
	if normalized.Name != m.Name || normalized.Namespace != m.Namespace {
		return nil, fmt.Errorf("incomplete response identity")
	}
	if err := apserde.ValidateNoRedactedPlaceholders(resource); err != nil {
		redacted = true
	}
	live := &LiveResource{Resource: resource, Metadata: m, Kind: kind, Redacted: redacted}
	switch kind {
	case "Actor":
		live.WriteOnlyFields = []string{"spec.signingKey"}
	case "Key":
		live.WriteOnlyFields = []string{"spec.keyData"}
	}
	return live, nil
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body []byte) ([]byte, bool, error) {
	endpoint := c.baseURL + "/" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("cannot construct API request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.signer.SignAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, false, fmt.Errorf("API request timed out: %w", context.DeadlineExceeded)
		}
		return nil, false, fmt.Errorf("API request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, &APIError{StatusCode: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, false, fmt.Errorf("API response timed out: %w", context.DeadlineExceeded)
		}
		return nil, false, fmt.Errorf("cannot read API response")
	}
	if len(data) > maxSourceBytes {
		return nil, false, fmt.Errorf("API response exceeds 16 MiB limit")
	}
	return data, resp.Header.Get(apserde.RedactedHeader) == "true", nil
}

func (c *Client) createBody(doc Document) ([]byte, error) {
	if err := c.checkKind(doc.Kind); err != nil {
		return nil, err
	}
	if doc.Kind == "Connection" {
		return nil, fmt.Errorf("Connection creation requires setup actions")
	}
	if doc.Metadata.ID != "" || doc.Metadata.Generation != 0 {
		return nil, fmt.Errorf("explicit ID or generation targets cannot be created")
	}
	data, err := json.Marshal(doc.Object)
	if err != nil {
		return nil, fmt.Errorf("cannot encode resource")
	}
	resource, err := c.scheme.DecodeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("invalid create resource")
	}
	m, kind, err := resourceMetadata(resource)
	if err != nil || kind != doc.Kind {
		return nil, fmt.Errorf("create kind mismatch")
	}
	if m.Name == "" {
		return nil, fmt.Errorf("create requires a resource name")
	}
	if err := apserde.ValidateNoRedactedPlaceholders(resource); err != nil {
		return nil, fmt.Errorf("redacted placeholders cannot be submitted")
	}
	descriptor, _ := resourceType(kind)
	if err := descriptor.ValidateResource(resource, meta.ValidationModeCreate); err != nil {
		return nil, fmt.Errorf("%s create failed semantic validation", kind)
	}
	return data, nil
}

// Create sends one validated resource. Batch execution and create-race handling
// belong to the executor; this method never retries a POST.
func (c *Client) Create(ctx context.Context, doc Document) (*LiveResource, error) {
	body, err := c.createBody(doc)
	if err != nil {
		return nil, err
	}
	adapter, _ := resourceType(doc.Kind)
	data, redacted, err := c.request(ctx, http.MethodPost, adapter.Collection, nil, body)
	if err != nil {
		return nil, err
	}
	return c.decodeLive(doc.Kind, data, redacted)
}

// Update submits a calculated canonical patch against a previously resolved
// resource. It never derives a patch from a redacted server response.
func (c *Client) Update(ctx context.Context, target Target, patch []byte) (*LiveResource, error) {
	if target.Current == nil {
		return nil, fmt.Errorf("update requires a resolved resource")
	}
	kind := target.Document.Kind
	if err := c.checkKind(kind); err != nil {
		return nil, err
	}
	if _, err := decodePatch(kind, patch, target.Current.Resource); err != nil {
		return nil, err
	}
	adapter, _ := resourceType(kind)
	path := adapter.Collection + "/" + url.PathEscape(target.Current.Metadata.ID)
	if target.Document.Metadata.Generation != 0 {
		path += "/generations/" + strconv.FormatUint(target.Document.Metadata.Generation, 10)
	}
	data, redacted, err := c.request(ctx, http.MethodPatch, path, nil, patch)
	if err != nil {
		return nil, err
	}
	return c.decodeLive(kind, data, redacted)
}

func patchDocument(doc Document) ([]byte, error) {
	object := make(map[string]any, len(doc.Object)+1)
	for k, v := range doc.Object {
		object[k] = v
	}
	if _, exists := object["spec"]; !exists {
		object["spec"] = map[string]any{}
	}
	data, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("cannot encode patch document")
	}
	return data, nil
}
