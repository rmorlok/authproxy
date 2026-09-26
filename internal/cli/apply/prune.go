package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	apiv1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
)

type PruneOptions struct {
	Namespace, Selector string
	Allowlist           []string
	All, Wait           bool
	Timeout             time.Duration
}

func (o PruneOptions) kinds() ([]meta.Kind, error) {
	if o.Namespace == "" {
		return nil, fmt.Errorf("--prune requires an explicit --namespace")
	}

	if err := validateNamespace(o.Namespace); err != nil {
		return nil, err
	}

	if o.All == (strings.TrimSpace(o.Selector) != "") {
		return nil, fmt.Errorf("--prune requires exactly one of --selector or --all")
	}

	if o.Timeout < 0 {
		return nil, fmt.Errorf("--timeout cannot be negative")
	}

	if len(o.Allowlist) == 0 {
		return nil, fmt.Errorf("--prune requires an explicit --prune-allowlist")
	}

	if _, err := database.ParseLabelSelector(o.Selector); err != nil {
		return nil, err
	}

	var kinds []meta.Kind
	seen := map[meta.Kind]bool{}
	for _, value := range o.Allowlist {
		kind := meta.Kind(strings.TrimPrefix(value, "authproxy.net/v1alpha1/"))
		if kind != "Actor" &&
			kind != "Key" &&
			kind != "RateLimit" {
			return nil, fmt.Errorf("prune supports only Actor, Key and RateLimit")
		}

		if !seen[kind] {
			kinds = append(kinds, kind)
			seen[kind] = true
		}
	}

	return kinds, nil
}

func (o PruneOptions) Validate() error { _, err := o.kinds(); return err }

type PrunePlan struct {
	client     *Client
	options    PruneOptions
	candidates []*LiveResource
	executed   atomic.Bool
	batch      *Batch
}

// listAll exhausts and validates every page. Namespace filtering is repeated
// locally by callers because server filters may include descendants.
func (c *Client) listAll(
	ctx context.Context,
	kind meta.Kind,
	namespace string,
) ([]*LiveResource, error) {
	descriptor, err := resourceType(kind)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if namespace != "" {
		query.Set("namespace", namespace)
	}

	seen := map[string]bool{}
	ids := map[string]bool{}

	var result []*LiveResource
	for {
		data, redacted, err := c.request(
			ctx,
			http.MethodGet,
			descriptor.Collection,
			query,
			nil, // body
		)
		if err != nil {
			return nil, err
		}

		var page apiv1.ResourceList[json.RawMessage]
		if util.DecodeJSONStrict(data, &page) != nil ||
			page.APIVersion != meta.APIVersionV1Alpha1 ||
			page.Kind != apiv1.ListKind(kind) ||
			page.Items == nil {
			return nil, fmt.Errorf("invalid prune inventory response")
		}

		if page.Metadata.RemainingItemCount != nil && *page.Metadata.RemainingItemCount < 0 {
			return nil, fmt.Errorf("invalid prune inventory count")
		}

		for _, item := range page.Items {
			live, err := c.decodeLive(kind, item, redacted)
			if err != nil {
				return nil, err
			}

			if ids[live.Metadata.ID] {
				return nil, fmt.Errorf("duplicate identity in prune inventory")
			}

			ids[live.Metadata.ID] = true

			result = append(result, live)
		}

		cursor := page.Metadata.Continue

		if cursor == "" {
			if page.Metadata.RemainingItemCount != nil &&
				*page.Metadata.RemainingItemCount > 0 {
				return nil, fmt.Errorf("incomplete prune inventory")
			}

			return result, nil
		}

		if seen[cursor] {
			return nil, fmt.Errorf("repeated prune inventory cursor")
		}

		seen[cursor] = true
		query = url.Values{"cursor": {cursor}}
	}
}

// PreparePrune fixes the maximum deletion set before any apply writes. It never
// interprets a failed or incomplete read as an empty inventory.
func (b *Batch) PreparePrune(
	ctx context.Context,
	options PruneOptions,
) (*PrunePlan, error) {
	kinds, err := options.kinds()
	if err != nil {
		return nil, err
	}

	if len(b.plans) == 0 {
		return nil, fmt.Errorf("refusing prune with no selected manifests")
	}

	keep := map[string]bool{}
	for _, plan := range b.plans {
		ns := plan.Document.Metadata.Namespace
		if plan.Target.Current != nil {
			ns = plan.Target.Current.Metadata.Namespace
			keep[string(plan.Document.Kind)+"/"+plan.Target.Current.Metadata.ID] = true
		}

		if ns != options.Namespace {
			return nil, fmt.Errorf("all prune inputs must belong to the exact --namespace scope")
		}
	}

	selector, _ := database.ParseLabelSelector(options.Selector)

	p := &PrunePlan{
		client:  b.client,
		options: options,
		batch:   b,
	}
	for _, kind := range kinds {
		live, err := b.client.listAll(ctx, kind, options.Namespace)
		if err != nil {
			return nil, err
		}

		for _, resource := range live {
			if resource.Metadata.Namespace != options.Namespace ||
				!selector.Matches(resource.Metadata.Labels) ||
				keep[string(kind)+"/"+resource.Metadata.ID] {
				continue
			}

			raw := resource.Metadata.Annotations[LastAppliedAnnotation]
			if raw == "" {
				continue
			}

			h, err := readHistory(raw, kind)
			if err != nil {
				return nil, err
			}

			if err = historyMatches(h, resource); err != nil {
				return nil, err
			}

			if resource.Metadata.ID == "key_global" {
				return nil, fmt.Errorf("the global key cannot be pruned")
			}

			p.candidates = append(p.candidates, resource)
		}
	}

	return p, nil
}

// checkReferences is deliberately unfiltered: a reference in another namespace
// also prevents deletion. Authorization or incomplete-list failures abort prune.
func (p *PrunePlan) checkReferences(ctx context.Context) error {
	if len(p.candidates) == 0 {
		return nil
	}

	// Only Key is a possible reference target among the supported prune kinds.
	hasKey := false
	for _, candidate := range p.candidates {
		hasKey = hasKey || candidate.Kind == "Key"
	}

	if !hasKey {
		return nil
	}

	resources, err := p.client.listAll(ctx, "Namespace", "")
	if err != nil {
		return err
	}

	for _, resource := range resources {
		refs, err := registry.References(resource.Resource)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			for _, candidate := range p.candidates {
				if ref.Kind == candidate.Kind &&
					(ref.ID == candidate.Metadata.ID ||
						(ref.HasNamespacedName() &&
							ref.Namespace == candidate.Metadata.Namespace &&
							ref.Name == candidate.Metadata.Name)) {
					return fmt.Errorf("cannot prune referenced %s %s", candidate.Kind, candidate.Metadata.ID)
				}
			}
		}
	}

	return nil
}

func (p *PrunePlan) Execute(ctx context.Context) ([]Result, error) {
	if p.batch == nil ||
		!p.batch.completedSuccessfully.Load() {
		return nil, fmt.Errorf("prune requires the prepared apply batch to complete successfully")
	}

	if !p.executed.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("prune plan has already executed")
	}

	if err := p.checkReferences(ctx); err != nil {
		return nil, err
	}

	// Refresh the entire deletion set before the first DELETE. Any changed
	// identity, labels, history or resource version requires another invocation.
	for _, old := range p.candidates {
		if err := p.checkCandidate(ctx, old); err != nil {
			return nil, err
		}
	}

	var results []Result
	for _, resource := range p.candidates {
		descriptor, _ := resourceType(resource.Kind)
		path := pathToOptionalGeneration(
			descriptor.Collection,
			resource.Metadata.ID,
			0, // generation
		)

		result := Result{
			Source:    "prune",
			Kind:      resource.Kind,
			Identity:  resource.Metadata.ID,
			Operation: Operation("delete"),
			Status:    "pruned",
		}

		deletePath := path
		if resource.Kind == "Key" {
			deletePath += "/unused"
		}

		err := p.checkCandidate(ctx, resource)
		if err == nil {
			_, _, err = p.client.request(ctx, http.MethodDelete, deletePath, nil, nil)
		}

		if err == nil && p.options.Wait {
			err = p.waitDeleted(ctx, resource.Kind, path)
		}

		if err != nil {
			result.Status = "failed"
			result.Error = "prune failed; deletion outcome may be uncertain: " + err.Error()
			results = append(results, result)
			return results, err
		}

		results = append(results, result)
	}
	return results, nil
}

func (p *PrunePlan) waitDeleted(ctx context.Context, kind meta.Kind, path string) error {
	if p.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.options.Timeout)
		defer cancel()
	}
	for {
		_, err := p.client.get(ctx, kind, path)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (p *PrunePlan) checkCandidate(ctx context.Context, old *LiveResource) error {
	descriptor, _ := resourceType(old.Kind)
	live, err := p.client.get(ctx, old.Kind, pathToOptionalGeneration(descriptor.Collection, old.Metadata.ID, 0))
	if err != nil {
		return err
	}
	selector, _ := database.ParseLabelSelector(p.options.Selector)
	if live.Metadata.Namespace != p.options.Namespace || !selector.Matches(live.Metadata.Labels) || !equalJSON(live.Resource, old.Resource) {
		return fmt.Errorf("prune candidate changed; rerun apply")
	}
	return nil
}
