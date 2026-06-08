package apblob

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rmorlok/authproxy/internal/schema/config"
)

type FilesystemClient struct {
	root string
}

func NewFilesystemClient(cfg *config.BlobStorageFilesystem) (Client, error) {
	if cfg == nil {
		return nil, errors.New("filesystem blob storage config is required")
	}
	if strings.TrimSpace(cfg.Path) == "" {
		return nil, errors.New("filesystem blob storage path is required")
	}

	root, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve filesystem blob storage path: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create filesystem blob storage root: %w", err)
	}

	return &FilesystemClient{root: root}, nil
}

func (c *FilesystemClient) Put(ctx context.Context, input PutInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p, err := c.pathFor(input.Key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("create filesystem blob storage directory: %w", err)
	}
	if err := os.WriteFile(p, input.Data, 0o600); err != nil {
		return fmt.Errorf("write filesystem blob: %w", err)
	}
	return nil
}

func (c *FilesystemClient) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p, err := c.pathFor(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrBlobNotFound
		}
		return nil, fmt.Errorf("read filesystem blob: %w", err)
	}
	return data, nil
}

func (c *FilesystemClient) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p, err := c.pathFor(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete filesystem blob: %w", err)
	}
	return nil
}

func (c *FilesystemClient) pathFor(key string) (string, error) {
	if key == "" {
		return "", errors.New("blob key is required")
	}
	if strings.ContainsRune(key, '\x00') {
		return "", fmt.Errorf("invalid blob key %q: null byte is not allowed", key)
	}
	if strings.Contains(key, "\\") {
		return "", fmt.Errorf("invalid blob key %q: backslashes are not allowed", key)
	}
	if path.IsAbs(key) {
		return "", fmt.Errorf("invalid blob key %q: absolute paths are not allowed", key)
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid blob key %q: empty, dot, and dot-dot path segments are not allowed", key)
		}
	}

	cleaned := path.Clean(key)
	p := filepath.Join(c.root, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(c.root, p)
	if err != nil {
		return "", fmt.Errorf("resolve filesystem blob path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid blob key %q: path escapes blob storage root", key)
	}
	return p, nil
}

var _ Client = (*FilesystemClient)(nil)
