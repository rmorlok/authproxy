package apply

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// HistoryTarget pins the object and history observed before an editor or batch.
type HistoryTarget struct {
	Document Document
	Current  *LiveResource
	raw      string
}

func (c *Client) HistoryTargets(ctx context.Context, docs []Document) ([]HistoryTarget, error) {
	targets := make([]HistoryTarget, 0, len(docs))
	seen := map[string]bool{}
	for _, doc := range docs {
		live, err := c.Resolve(ctx, doc)
		if err != nil {
			return nil, err
		}
		if live == nil {
			return nil, fmt.Errorf("%s: history commands require an existing resource", doc.Source)
		}
		id := string(doc.Kind) + "/" + live.Metadata.ID
		if seen[id] {
			return nil, fmt.Errorf("duplicate history target")
		}
		seen[id] = true
		targets = append(targets, HistoryTarget{doc, live, live.Metadata.Annotations[LastAppliedAnnotation]})
	}
	return targets, nil
}

func (t HistoryTarget) LastApplied() (map[string]any, error) {
	if t.raw == "" {
		return nil, fmt.Errorf("%s: no last-applied history", t.Document.Source)
	}
	h, err := readHistory(t.raw, t.Document.Kind)
	if err != nil {
		return nil, err
	}
	if err = historyMatches(h, t.Current); err != nil {
		return nil, err
	}
	return h.Desired, nil
}

func historyMatches(h *History, live *LiveResource) error {
	m := asMap(h.Desired["metadata"])
	if id, _ := m["id"].(string); id != "" && id != live.Metadata.ID {
		return fmt.Errorf("last-applied history belongs to another resource")
	}
	if name, _ := m["name"].(string); name != "" && name != string(live.Metadata.Name) {
		return fmt.Errorf("last-applied history name mismatch")
	}
	if ns, _ := m["namespace"].(string); ns != "" && ns != live.Metadata.Namespace {
		return fmt.Errorf("last-applied history namespace mismatch")
	}
	return nil
}

// SetLastApplied changes only the history annotation. All inputs are validated
// before the first write. Concurrent history changes abort, without retries.
func (c *Client) SetLastApplied(ctx context.Context, targets []HistoryTarget, docs []Document, create bool) ([]Result, error) {
	if len(targets) != len(docs) {
		return nil, fmt.Errorf("history editing must retain every selected resource")
	}
	histories := make([]*History, len(docs))
	for i, doc := range docs {
		target := targets[i]
		if doc.Kind != target.Document.Kind || doc.Metadata.Generation != target.Document.Metadata.Generation {
			return nil, fmt.Errorf("history editing cannot change kind or generation")
		}
		h, err := newHistory(doc)
		if err != nil {
			return nil, err
		}
		if err = historyMatches(h, target.Current); err != nil {
			return nil, err
		}
		if target.raw == "" && !create {
			return nil, fmt.Errorf("missing history; use --create-annotation to initialize it")
		}
		if old, err := readHistory(target.raw, doc.Kind); err == nil {
			h.Secrets = uniqueSorted(append(h.Secrets, old.Secrets...))
		}
		h.Desired["metadata"].(map[string]any)["id"] = target.Current.Metadata.ID
		annotations := map[string]any{}
		for key, value := range target.Current.Metadata.Annotations {
			annotations[key] = value
		}
		holder := map[string]any{"apiVersion": meta.APIVersionV1Alpha1, "kind": doc.Kind, "metadata": map[string]any{"annotations": annotations}, "spec": map[string]any{}}
		if err = attachHistory(holder, h); err != nil {
			return nil, err
		}
		data, err := json.Marshal(holder)
		if err != nil {
			return nil, err
		}
		if _, err = decodePatch(doc.Kind, data, target.Current.Resource); err != nil {
			return nil, err
		}
		histories[i] = h
	}
	results := make([]Result, 0, len(docs))
	for i, target := range targets {
		pinned := target.Document
		pinned.Metadata.ID = target.Current.Metadata.ID
		live, err := c.Resolve(ctx, pinned)
		if err != nil {
			return results, err
		}
		if live.Metadata.Annotations[LastAppliedAnnotation] != target.raw {
			return results, fmt.Errorf("history changed since it was read; reload before retrying")
		}
		annotations := map[string]any{}
		for key, value := range live.Metadata.Annotations {
			annotations[key] = value
		}
		patch := map[string]any{"apiVersion": meta.APIVersionV1Alpha1, "kind": live.Kind, "metadata": map[string]any{"annotations": annotations}, "spec": map[string]any{}}
		if err = attachHistory(patch, histories[i]); err != nil {
			return results, err
		}
		status := "unchanged"
		if annotations[LastAppliedAnnotation] != target.raw {
			data, _ := json.Marshal(patch)
			if _, err = c.Update(ctx, Target{pinned, live}, data); err != nil {
				return results, err
			}
			status = "configured"
		}
		results = append(results, Result{Source: target.Document.Source, Kind: live.Kind, Identity: live.Metadata.ID, Operation: OperationUpdate, Status: status})
	}
	return results, nil
}
