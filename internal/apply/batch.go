package apply

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// Batch is a prepared snapshot. Plans are private because mutating one after
// dependency validation would invalidate execution ordering. It is single-use:
// retrying after an ambiguous mutation could create duplicate resources.
type Batch struct {
	client       *Client
	plans        []*Plan
	dependencies [][]int
	order        []int
	executed     atomic.Bool
}

// Result is safe for human or structured output. Resource is sanitized before
// it is exposed here; errors never echo server bodies or submitted spec values.
type Result struct {
	Source    string    `json:"source" yaml:"source"`
	Kind      meta.Kind `json:"kind" yaml:"kind"`
	Identity  string    `json:"identity" yaml:"identity"`
	Operation Operation `json:"operation" yaml:"operation"`
	Status    string    `json:"status" yaml:"status"`
	Resource  any       `json:"resource,omitempty" yaml:"resource,omitempty"`
	Error     string    `json:"error,omitempty" yaml:"error,omitempty"`
}

var ErrBatchFailed = errors.New("apply batch did not complete successfully; successful writes were not rolled back")

// Prepare resolves and reconciles the complete input, verifies external
// prerequisites, and checks dependency cycles before any mutation is possible.
// A failed read, invalid plan or missing prerequisite rejects the whole batch.
func (c *Client) Prepare(
	ctx context.Context,
	documents []Document,
	options ReconcileOptions,
) (*Batch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	targets, err := c.ResolveBatch(ctx, documents)
	if err != nil {
		return nil, err
	}

	batch := &Batch{client: c, dependencies: make([][]int, len(targets))}
	index := map[string]int{}

	for i, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		plan, err := Reconcile(target, options)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target.Document.Source, err)
		}

		batch.plans = append(batch.plans, plan)
		for _, key := range targetKeys(target) {
			if previous, ok := index[key]; ok && previous != i {
				return nil, fmt.Errorf("%s: duplicate batch target", target.Document.Source)
			}
			index[key] = i
		}
	}

	verified := map[string]bool{}
	for i, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		m := target.Document.Metadata
		if target.Current != nil {
			m = target.Current.Metadata
		}

		// Namespace membership depends on namespace creation, not on updating
		// an already-existing namespace. This permits installing a key in an
		// existing namespace before updating that namespace's encryptionKeyRef.
		if m.Namespace != "" {
			ref := meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Namespace",
				ID:         m.Namespace,
			}
			if err := batch.dependency(
				ctx,
				i,
				ref,
				true,
				index,
				verified,
			); err != nil {
				return nil, err
			}
		}

		references, err := registry.References(target.Document.Resource)
		if err != nil {
			return nil, err
		}
		for _, ref := range references {
			if err := batch.dependency(ctx, i, ref, false, index, verified); err != nil {
				return nil, err
			}
		}
	}
	order, err := dependencyOrder(batch.dependencies)
	if err != nil {
		return nil, err
	}
	batch.order = order
	return batch, nil
}

func targetKeys(target Target) []string {
	m := target.Document.Metadata
	if target.Current != nil {
		m = target.Current.Metadata
	}
	kind := target.Document.Kind
	keys := []string{}
	if m.ID != "" {
		keys = append(keys, string(kind)+"/id/"+m.ID)
	}
	if m.Name != "" {
		keys = append(keys, string(kind)+"/name/"+m.Namespace+"/"+string(m.Name))
	}
	if kind == "Namespace" {
		if path, err := namespace.PathFromMetadata(m); err == nil {
			keys = append(keys, "Namespace/id/"+path)
		}
	}
	return keys
}

// dependency validates one prerequisite during batch preparation. If the
// prerequisite is in the batch, it checks identity consistency and records a
// deduplicated ordering edge in b.dependencies[dependent]. Otherwise, it reads
// the prerequisite from the cluster and requires it to exist. Explicit reference
// generations must already exist even when the target is in the batch. This
// method performs no cluster writes; dependencyOrder checks cycles afterward.
//
// Parameters:
//   - ctx controls cancellation and deadlines for prerequisite reads.
//   - dependent is the input-order index in b.plans of the resource requiring ref.
//   - ref identifies the prerequisite by kind and ID or namespaced name, with
//     optional constraints on other identity fields and generation.
//   - namespaceMembership is true for a resource's namespace or namespace parent.
//     These require an ordering edge only when that namespace is being created;
//     an existing namespace need not finish updating first. False denotes an
//     explicit resource reference, which adds an edge even for an existing target.
//   - index maps the identity aliases produced by targetKeys to b.plans indices.
//     It contains all batch targets and is read-only here.
//   - verified is a non-nil cache shared by dependency calls within one batch
//     preparation. Successful external lookups are recorded by full reference
//     identity and generation to avoid repeated reads without bypassing ID/name
//     consistency checks.
//
// Invalid references, missing prerequisites and failed reads return an error
// identifying the dependent's source and prerequisite kind.
func (b *Batch) dependency(
	ctx context.Context,
	dependent int,
	ref meta.ObjectReference,
	namespaceMembership bool,
	index map[string]int,
	verified map[string]bool,
) error {
	source := b.plans[dependent].Document.Source
	fail := func(err error) error {
		return fmt.Errorf("%s: %s prerequisite: %w", source, ref.Kind, err)
	}

	if ref.APIVersion != meta.APIVersionV1Alpha1 {
		return fail(fmt.Errorf("unsupported reference apiVersion"))
	}

	m := meta.ObjectMeta{ID: ref.ID, Name: ref.Name, Namespace: ref.Namespace, Generation: ref.Generation}
	if err := normalizeIdentity(string(ref.Kind), &m, ""); err != nil {
		return fail(err)
	}

	key := string(ref.Kind) + "/name/" + m.Namespace + "/" + string(m.Name)
	if m.ID != "" {
		key = string(ref.Kind) + "/id/" + m.ID
	}

	if prerequisite, ok := index[key]; ok {
		plan := b.plans[prerequisite]
		actual := plan.Document.Metadata

		if plan.Target.Current != nil {
			actual = plan.Target.Current.Metadata
		}

		if (ref.ID != "" && ref.Kind != "Namespace" && ref.ID != actual.ID) ||
			(m.Name != "" && m.Name != actual.Name) ||
			(m.Namespace != "" && m.Namespace != actual.Namespace) {
			return fail(fmt.Errorf("reference does not match batch target identity"))
		}

		if ref.Generation != 0 {
			if plan.Target.Current == nil {
				return fail(fmt.Errorf("explicit generation must already exist"))
			}
			// Check the exact generation, not just the generation selected for apply.
			if _, err := b.client.Resolve(ctx, Document{Kind: ref.Kind, Metadata: m}); err != nil {
				return fail(err)
			}
		}

		if !namespaceMembership || plan.Operation == OperationCreate {
			if slices.Contains(b.dependencies[dependent], prerequisite) {
				return nil
			}

			b.dependencies[dependent] = append(b.dependencies[dependent], prerequisite)
		}
		return nil
	}
	
	// Cache full reference identities; ID and name supplied together must still
	// be checked for consistency even if an ID-only reference was already read.
	cacheKey := fmt.Sprintf("%s/%s/%s/%d", key, m.Namespace, m.Name, m.Generation)
	if verified[cacheKey] {
		return nil
	}
	live, err := b.client.Resolve(ctx, Document{Kind: ref.Kind, Metadata: m})
	if err != nil {
		return fail(err)
	}
	if live == nil {
		return fail(fmt.Errorf("referenced resource was not found"))
	}
	verified[cacheKey] = true
	return nil
}

// Select the first ready input on every iteration. Thus independent resources
// retain input order whenever their prerequisites permit it.
func dependencyOrder(dependencies [][]int) ([]int, error) {
	order := make([]int, 0, len(dependencies))
	done := make([]bool, len(dependencies))
	for len(order) < len(dependencies) {
		ready := -1
		for i, prerequisites := range dependencies {
			if done[i] {
				continue
			}
			available := true
			for _, p := range prerequisites {
				if !done[p] {
					available = false
					break
				}
			}
			if available {
				ready = i
				break
			}
		}
		if ready < 0 {
			return nil, fmt.Errorf("apply dependency cycle; no resources were written")
		}
		done[ready] = true
		order = append(order, ready)
	}
	return order, nil
}

// Warnings returns preparation warnings in input order, for display before
// writes. Errors writing warnings can therefore abort without changing cluster state.
func (b *Batch) Warnings() []string {
	var warnings []string
	for _, plan := range b.plans {
		for _, warning := range plan.Warnings {
			warnings = append(warnings, plan.Document.Source+": "+warning)
		}
	}
	return warnings
}

// Execute attempts each ready resource once. Failed dependencies are skipped;
// independent resources continue. Cancellation skips all remaining resources.
// Results are returned in execution order, including failures and skips.
func (b *Batch) Execute(ctx context.Context) ([]Result, error) {
	if !b.executed.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("apply batch has already been executed; prepare a new batch before retrying")
	}
	results := make([]Result, 0, len(b.plans))
	succeeded := make([]bool, len(b.plans))
	failed := false
	for _, i := range b.order {
		plan := b.plans[i]
		result := Result{Source: plan.Document.Source, Kind: plan.Document.Kind, Identity: documentIdentity(plan.Document), Operation: plan.Operation}
		if err := ctx.Err(); err != nil {
			result.Status = "skipped"
			result.Error = err.Error()
		} else {
			for _, p := range b.dependencies[i] {
				if !succeeded[p] {
					result.Status = "skipped"
					result.Error = "prerequisite did not complete successfully"
					break
				}
			}
		}
		if result.Status == "" {
			var live *LiveResource
			var err error
			switch plan.Operation {
			case OperationCreate:
				live, err = b.client.Create(ctx, plan.Document)
				result.Status = "created"
			case OperationUpdate:
				live, err = b.client.Update(ctx, plan.Target, plan.Patch)
				result.Status = "configured"
			case OperationUnchanged:
				live = plan.Target.Current
				result.Status = "unchanged"
			}
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error() + "; mutation outcome may be uncertain"
			} else {
				result.Identity = live.Metadata.ID
				result.Resource, _, err = apserde.SanitizeJSONForAPI(context.Background(), live.Resource)
				if err != nil {
					result.Status = "failed"
					result.Error = "cannot sanitize resource output; any successful write remains applied"
				}
			}
		}
		if result.Status == "failed" || result.Status == "skipped" {
			failed = true
		} else {
			succeeded[i] = true
		}
		results = append(results, result)
	}
	if failed {
		return results, errors.Join(ErrBatchFailed, ctx.Err())
	}
	return results, nil
}

func documentIdentity(doc Document) string {
	if doc.Metadata.ID != "" {
		return doc.Metadata.ID
	}
	if doc.Metadata.Namespace != "" {
		return doc.Metadata.Namespace + "/" + string(doc.Metadata.Name)
	}
	return string(doc.Metadata.Name)
}

// ResultName is the compact resource identifier used in human and name output.
func ResultName(result Result) string {
	return strings.ToLower(string(result.Kind)) + "/" + result.Identity
}
