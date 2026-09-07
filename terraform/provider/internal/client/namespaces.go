package client

import (
	"context"
	"fmt"
	"strings"
)

const NamespaceKind = "Namespace"

type NamespaceSpec struct {
	EncryptionKeyRef *ObjectReference `json:"encryptionKeyRef,omitempty"`
}

type NamespaceStatus struct {
	State string `json:"state"`
}

type Namespace struct {
	TypeMeta
	Metadata ObjectMetadata   `json:"metadata"`
	Spec     NamespaceSpec    `json:"spec"`
	Status   *NamespaceStatus `json:"status,omitempty"`
}

type CreateNamespaceRequest struct {
	TypeMeta
	Metadata ObjectMetadata `json:"metadata"`
	Spec     NamespaceSpec  `json:"spec"`
}

type NamespaceSpecPatch struct{}

type UpdateNamespaceRequest struct {
	TypeMeta
	Metadata *ObjectMetadataPatch `json:"metadata"`
	Spec     *NamespaceSpecPatch  `json:"spec"`
}

// NamespaceMetadataForPath maps Terraform's stable path attribute onto the
// API's Kubernetes-style name and parent namespace identity.
func NamespaceMetadataForPath(path string) ObjectMetadata {
	index := strings.LastIndex(path, ".")
	if index < 0 {
		return ObjectMetadata{Name: path}
	}
	return ObjectMetadata{Name: path[index+1:], Namespace: path[:index]}
}

func (c *Client) CreateNamespace(ctx context.Context, req CreateNamespaceRequest) (*Namespace, error) {
	var ns Namespace
	err := c.post(ctx, "/api/v1/namespaces", req, &ns)
	return &ns, err
}

func (c *Client) GetNamespace(ctx context.Context, path string) (*Namespace, error) {
	var ns Namespace
	err := c.get(ctx, fmt.Sprintf("/api/v1/namespaces/%s", path), &ns)
	return &ns, err
}

func (c *Client) UpdateNamespace(ctx context.Context, path string, req UpdateNamespaceRequest) (*Namespace, error) {
	var ns Namespace
	err := c.patch(ctx, fmt.Sprintf("/api/v1/namespaces/%s", path), req, &ns)
	return &ns, err
}

func (c *Client) DeleteNamespace(ctx context.Context, path string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/namespaces/%s", path))
}
