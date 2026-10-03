package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type startupApply struct {
	createSystemActor bool
	endpoint          string
	serviceID         sconfig.ServiceId
	signingKey        []byte
	actor             string
	actorNamespace    string
	documents         []apply.Document
	timeout           time.Duration
}

// Prepare local input before migrations or listeners are started. The endpoint
// comes from the selected listener, never a public base URL or client config.
func prepareStartupApply(
	ctx context.Context,
	options serveOptions,
) (*startupApply, error) {
	if len(options.apply.filenames) == 0 {
		return nil, nil
	}

	selected := &cfg.GetRoot().Api.ServiceHttp
	if options.applyService == sconfig.ServiceIdAdminApi {
		selected = &cfg.GetRoot().AdminApi.ServiceHttp
	}

	for _, filename := range options.apply.filenames {
		if filename == "-" {
			return nil, fmt.Errorf("--apply requires local files or directories; stdin is not supported")
		}
		if _, err := os.Stat(filename); err != nil {
			return nil, fmt.Errorf("--apply input: %w", err)
		}
	}

	documents, err := apply.Load(
		ctx,
		apply.Options{
			Filenames:  options.apply.filenames,
			Validation: apply.ValidationStrict,
		},
	)
	if err != nil {
		return nil, err
	}

	port, err := selected.PortVal.GetUint64Value(ctx)
	if err != nil {
		return nil, fmt.Errorf("apply listener port: %w", err)
	}

	if port == 0 || port > 65535 {
		return nil, fmt.Errorf("--apply requires a fixed listener port between 1 and 65535")
	}

	scheme := "http"
	if selected.TlsVal != nil {
		scheme = "https"
	}

	endpoint := fmt.Sprintf("%s://localhost:%d", scheme, port)
	if cfg.GetRoot().SystemAuth.GlobalAESKey == nil {
		return nil, fmt.Errorf("startup apply requires systemAuth.globalAesKey")
	}

	key, err := cfg.GetRoot().SystemAuth.GlobalAESKey.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("startup apply signing key: %w", err)
	}

	return &startupApply{
		endpoint:          endpoint,
		serviceID:         options.applyService,
		createSystemActor: options.apply.createSystemActor,
		signingKey:        key.Data,
		actor:             options.apply.actor,
		actorNamespace:    options.apply.actorNamespace,
		documents:         documents,
		timeout:           options.apply.timeout,
	}, nil
}

// Preparation is read-only and can be retried while the listener and configured
// actor synchronization come online. Never retry writes after an ambiguous result.
func (a *startupApply) run(ctx context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	// Mint only after migration and service launch so migration time cannot
	// consume the token's lifetime. Stored actor permissions authorize requests.
	signer, err := jwt.NewJwtTokenBuilder().WithSystemSigned().WithSecretKey(a.signingKey).
		WithServiceId(a.serviceID).WithActorExternalId(a.actor).WithNamespace(a.actorNamespace).
		WithExpiresIn(a.timeout + time.Minute).SignerCtx(ctx)
	if err != nil {
		return err
	}
	client, err := apply.NewClient(apply.ClientOptions{APIURL: a.endpoint, Signer: signer, Timeout: apply.DefaultRequestTimeout})
	if err != nil {
		return err
	}
	var batch *apply.Batch
	for {
		var err error
		batch, err = client.Prepare(ctx, a.documents, apply.ReconcileOptions{Overwrite: true})
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return fmt.Errorf("startup apply readiness: %w", ctx.Err())
		}
		if !startupApplyRetryable(err) {
			return fmt.Errorf("startup apply preparation: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("startup apply readiness: %w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	for _, warning := range batch.Warnings() {
		if _, err := fmt.Fprintln(out, "Warning: "+warning); err != nil {
			return err
		}
	}
	results, err := batch.Execute(ctx)
	for _, result := range results {
		if result.Error != "" {
			if _, outputErr := fmt.Fprintf(out, "%s %s: %s\n", result.Kind, result.Identity, result.Error); outputErr != nil {
				return errors.Join(err, outputErr)
			}
		}
		if _, outputErr := fmt.Fprintf(out, "%s %s: %s %s\n", result.Kind, result.Identity, result.Operation, result.Status); outputErr != nil {
			return errors.Join(err, outputErr)
		}
	}
	return err
}

func startupApplyRetryable(err error) bool {
	var apiErr *apply.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 ||
			apiErr.StatusCode == 429 ||
			apiErr.StatusCode == 503
	}
	return errors.Is(err, apply.ErrRequestFailed) ||
		errors.Is(err, context.DeadlineExceeded)
}

func serveWithApply(
	ctx context.Context,
	options serveOptions,
	a *startupApply,
	out io.Writer,
) error {
	if a == nil {
		return startServices(options)
	}
	if a.createSystemActor {
		if err := prepareSystemApplyActor(ctx); err != nil {
			return fmt.Errorf("startup apply actor: %w", err)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	start := startServices
	go func() { done <- start(options) }()

	applied := make(chan error, 1)
	go func() { applied <- a.run(ctx, out) }()

	select {
	case err := <-done:
		return err
	case err := <-applied:
		if err != nil {
			return fmt.Errorf("startup apply failed: %w", err)
		}
		return <-done
	}
}
