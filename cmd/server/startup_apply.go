package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type startupApplyOptions struct {
	filenames      []string
	actor          string
	actorNamespace string
	timeout        time.Duration
}

type startupApply struct {
	endpoint       string
	serviceID      sconfig.ServiceId
	signingKey     []byte
	actor          string
	actorNamespace string
	documents      []apply.Document
	timeout        time.Duration
}

// Prepare local input before migrations or listeners are started. The endpoint
// comes from the selected listener, never a public base URL or client config.
func prepareStartupApply(
	ctx context.Context,
	services string,
	options startupApplyOptions,
) (*startupApply, error) {
	if len(options.filenames) == 0 {
		return nil, nil
	}

	if options.actor == "" {
		return nil, fmt.Errorf("--apply requires --apply-actor (an existing actor's external ID)")
	}

	if err := namespace.ValidatePath(options.actorNamespace); err != nil {
		return nil, fmt.Errorf("--apply-actor-namespace: %w", err)
	}

	if options.timeout <= 0 {
		return nil, fmt.Errorf("--apply-timeout must be positive")
	}

	var selected *sconfig.ServiceHttp
	var serviceID sconfig.ServiceId
	for _, id := range strings.Split(services, ",") {
		switch id {
		case "all", "admin-api":
			selected = &cfg.GetRoot().AdminApi.ServiceHttp
			serviceID = sconfig.ServiceIdAdminApi
		case "api":
			if selected == nil {
				selected = &cfg.GetRoot().Api.ServiceHttp
				serviceID = sconfig.ServiceIdApi
			}
		}
	}

	if selected == nil {
		return nil, fmt.Errorf("--apply requires serving api or admin-api")
	}

	for _, filename := range options.filenames {
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
			Filenames:  options.filenames,
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
		endpoint:       endpoint,
		serviceID:      serviceID,
		signingKey:     key.Data,
		actor:          options.actor,
		actorNamespace: options.actorNamespace,
		documents:      documents,
		timeout:        options.timeout,
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
	noBanner bool,
	services string,
	a *startupApply,
	out io.Writer,
) error {
	if a == nil {
		return startServices(noBanner, services)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	start := startServices
	go func() { done <- start(noBanner, services) }()

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
