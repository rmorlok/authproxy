// Package apply loads declarative resource input without contacting an AuthProxy
// cluster. Documents retain field presence for subsequent reconciliation.
package apply

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/registry"
)

const maxSourceBytes = 16 << 20

// Options controls input discovery. HTTP is used only for manifest sources;
// never supply an authenticated cluster client here.
type Options struct {
	Filenames []string
	Recursive bool
	Namespace string
	Selector  string
	Stdin     io.Reader
}

// Load validates the entire input before returning any documents. Files within
// directories are visited lexically; explicit inputs retain command-line order.
func Load(ctx context.Context, options Options) ([]Document, error) {
	if len(options.Filenames) == 0 {
		return nil, fmt.Errorf("at least one --filename is required")
	}

	selector, err := database.ParseLabelSelector(options.Selector)
	if err != nil {
		return nil, err
	}

	if options.Namespace != "" {
		if err := validateNamespace(options.Namespace); err != nil {
			return nil, fmt.Errorf("--namespace: %w", err)
		}
	}
	loader := inputLoader{
		ctx:      ctx,
		options:  options,
		selector: selector,
		seen:     map[string]string{},
		scheme:   registry.NewResourceScheme(),
	}

	for _, filename := range options.Filenames {
		if err := loader.load(filename); err != nil {
			return nil, err
		}
	}

	if loader.count == 0 {
		return nil, fmt.Errorf("no resource documents found")
	}

	return loader.documents, nil
}

type inputLoader struct {
	ctx       context.Context
	options   Options
	selector  database.LabelSelector
	seen      map[string]string
	stdinUsed bool
	count     int
	documents []Document
	scheme    decoder
}

func (l *inputLoader) load(source string) error {
	if err := l.ctx.Err(); err != nil {
		return err
	}

	if source == "-" {
		if l.stdinUsed {
			return fmt.Errorf("stdin may only be specified once")
		}

		l.stdinUsed = true
		if l.options.Stdin == nil {
			return fmt.Errorf("stdin is not available")
		}

		return l.read("stdin", l.options.Stdin)
	}

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		u, err := url.Parse(source)
		if err != nil || u.Host == "" || u.User != nil {
			return fmt.Errorf("manifest URL must have a host and must not contain credentials")
		}

		// Do not echo query strings, which can contain signed URL credentials.
		label := u.Scheme + "://" + u.Host + u.EscapedPath()
		req, err := http.NewRequestWithContext(
			l.ctx,
			http.MethodGet,
			source,
			nil, // body
		)
		if err != nil {
			return fmt.Errorf("%s: invalid manifest URL", label)
		}

		client := &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}

				if req.URL.User != nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
					return fmt.Errorf("invalid manifest redirect")
				}

				if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
					return fmt.Errorf("insecure manifest redirect")
				}

				return nil
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%s: manifest download failed", label)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: HTTP %d", label, resp.StatusCode)
		}

		return l.read(label, resp.Body)
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}

	if !info.IsDir() {
		return l.file(source)
	}

	return filepath.WalkDir(
		source,
		func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if err := l.ctx.Err(); err != nil {
				return err
			}

			if entry.IsDir() {
				if path != source && !l.options.Recursive {
					return filepath.SkipDir
				}
				return nil
			}

			// Do not follow symlinks discovered within directories.
			if !entry.Type().IsRegular() {
				return nil
			}

			switch strings.ToLower(filepath.Ext(path)) {
			case ".yaml", ".yml", ".json":
				return l.file(path)
			}

			return nil
		},
	)
}

func (l *inputLoader) file(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: expected a regular file", path)
	}

	return l.read(path, f)
}

func (l *inputLoader) read(source string, r io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(r, maxSourceBytes+1))
	if err != nil {
		return fmt.Errorf("%s: failed to read manifest", source)
	}

	if len(data) > maxSourceBytes {
		return fmt.Errorf("%s: exceeds 16 MiB manifest limit", source)
	}

	return l.decode(source, data)
}
