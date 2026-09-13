package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

type ReconcileOptions struct {
	// Overwrite permits restoring managed fields changed outside apply. CLI
	// callers default this to true; false reports conflicts without values.
	Overwrite bool
}

type Operation string

const (
	OperationCreate    Operation = "create"
	OperationUpdate    Operation = "update"
	OperationUnchanged Operation = "unchanged"
)

// Plan is a validated single-resource write, not an executed operation. Create
// uses Document; Update uses Target and Patch. Both carry history in the same
// write as the resource. Payloads may contain secrets and must never be logged.
type Plan struct {
	Operation Operation
	Document  Document
	Target    Target
	Patch     []byte
	Warnings  []string
}

// Reconcile performs no I/O and never mutates its inputs. The executor must
// submit the returned payload unchanged and must not save history separately.
func Reconcile(target Target, options ReconcileOptions) (*Plan, error) {
	doc, err := documentWithObject(target.Document, target.Document.Object)
	if err != nil {
		return nil, err
	}

	if err := apserde.ValidateNoRedactedPlaceholders(doc.Resource); err != nil {
		return nil, fmt.Errorf("redacted placeholders cannot be applied")
	}

	h, err := newHistory(doc)
	if err != nil {
		return nil, err
	}

	if _, ok := doc.Metadata.Annotations[LastAppliedAnnotation]; ok {
		return nil, fmt.Errorf("last-applied annotation is managed by apply; omit it from manifests")
	}

	plan := &Plan{Target: target, Document: doc}
	validator := &Client{scheme: registry.NewResourceScheme()}

	if target.Current == nil {
		object, err := plainObject(doc.Object)
		if err != nil {
			return nil, err
		}

		if err = attachHistory(object, h); err != nil {
			return nil, err
		}

		plan.Document, err = documentWithObject(doc, object)
		if err != nil {
			return nil, err
		}

		if _, err = validator.createBody(plan.Document); err != nil {
			return nil, err
		}

		plan.Operation = OperationCreate
		return plan, nil
	}

	current := target.Current
	actualMeta, actualKind, err := resourceMetadata(current.Resource)

	if err != nil || actualKind != doc.Kind || actualMeta.ID != current.Metadata.ID {
		return nil, fmt.Errorf("invalid reconciliation target")
	}

	if (doc.Metadata.ID != "" && doc.Metadata.ID != current.Metadata.ID) ||
		(doc.Metadata.Name != "" && doc.Metadata.Name != current.Metadata.Name) ||
		(doc.Metadata.Namespace != "" && doc.Metadata.Namespace != current.Metadata.Namespace) ||
		(doc.Metadata.Generation != 0 && doc.Metadata.Generation != current.Metadata.Generation) {
		return nil, fmt.Errorf("reconciliation target does not match supplied identity")
	}

	if current.Kind != doc.Kind {
		return nil, fmt.Errorf("reconciliation target kind mismatch")
	}

	live, err := plainObject(current.Resource)
	if err != nil {
		return nil, err
	}

	previous := map[string]any{}
	if raw, ok := current.Metadata.Annotations[LastAppliedAnnotation]; ok {
		old, err := readHistory(raw, doc.Kind)
		if err != nil {
			return nil, err
		}

		previous = old.Desired
		pm, _ := previous["metadata"].(map[string]any)

		if id, _ := pm["id"].(string); id != "" && id != current.Metadata.ID {
			return nil, fmt.Errorf("last-applied history belongs to a different resource")
		}

		if id, _ := pm["id"].(string); id == "" {
			if name, _ := pm["name"].(string); name != string(current.Metadata.Name) {
				return nil, fmt.Errorf("last-applied history belongs to a different resource")
			}
		}

		if ns, _ := pm["namespace"].(string); ns != "" && ns != current.Metadata.Namespace {
			return nil, fmt.Errorf("last-applied history belongs to a different namespace")
		}

		// Remember historical secret presence even when omitted now. It confers no
		// right to clear a secret and contains no value or comparison fingerprint.
		h.Secrets = uniqueSorted(append(h.Secrets, old.Secrets...))
	} else {
		plan.Warnings = append(plan.Warnings, "adopting resource without last-applied history; unspecified fields are preserved")
	}

	desired, err := plainObject(doc.Object)
	if err != nil {
		return nil, err
	}

	forces := map[string]bool{}
	for _, path := range apserde.SensitivePaths(doc.Resource) {
		if _, ok := at(desired, path); ok {
			forces[pointer(path)] = true
		}
	}

	merger := threeWay{overwrite: options.Overwrite, force: forces}
	patch := map[string]any{
		"apiVersion": string(meta.APIVersionV1Alpha1),
		"kind":       string(doc.Kind),
		"metadata":   map[string]any{},
		"spec":       map[string]any{},
	}
	patchMeta := patch["metadata"].(map[string]any)

	// REST replaces whole metadata maps. Compute their contents with the same
	// field ownership rules, then send the complete merged map.
	for _, field := range []string{"labels", "annotations"} {
		path := []string{"metadata", field}
		old, _ := at(previous, path)
		now, _ := at(live, path)
		next, _ := at(desired, path)
		a := asMap(old)
		b := asMap(now)
		d := asMap(next)

		if field == "annotations" {
			delete(a, LastAppliedAnnotation)
			delete(b, LastAppliedAnnotation)
			delete(d, LastAppliedAnnotation)
		}

		merged, _, err := merger.merge(a, true, b, true, d, true, path)
		if err != nil {
			return nil, err
		}

		if !equalJSON(b, merged) {
			patchMeta[field] = merged
		}
	}

	oldSpec := asMap(previous["spec"])
	liveSpec := asMap(live["spec"])
	desiredSpec := asMap(desired["spec"])
	patchSpec := patch["spec"].(map[string]any)

	for _, field := range keys(oldSpec, desiredSpec) {
		old, op := oldSpec[field]
		now, np := liveSpec[field]
		next, dp := desiredSpec[field]

		value, present, err := merger.merge(old, op, now, np, next, dp, []string{"spec", field})
		if err != nil {
			return nil, err
		}

		forced := merger.forced([]string{"spec", field})

		if present == np && equalJSON(value, now) && !forced {
			continue
		}

		if !present {
			value = clearSpecField(doc.Kind, field)
		}

		patchSpec[field] = value
	}

	// Definition and other object patches replace entire fields. Do not replay
	// masked values while preserving unowned fields. decodePatch below rejects
	// any masked secret left in the replacement; a caller must provide it.
	if len(forces) > 0 {
		plan.Warnings = append(plan.Warnings, "explicit secrets are submitted without comparison; omitted secrets are preserved")
	}

	historyMeta := h.Desired["metadata"].(map[string]any)
	historyMeta["id"] = current.Metadata.ID
	if current.Metadata.Namespace != "" {
		historyMeta["namespace"] = current.Metadata.Namespace
	}

	// Merge history into live/user annotations, even if no user annotation changed.
	annotations := asMap(liveMeta(live)["annotations"])
	if v, ok := patchMeta["annotations"]; ok {
		annotations = asMap(v)
	}

	holder := map[string]any{"metadata": map[string]any{"annotations": annotations}}

	if err := attachHistory(holder, h); err != nil {
		return nil, err
	}

	if !equalJSON(liveMeta(live)["annotations"], annotations) {
		patchMeta["annotations"] = annotations
	}

	// Compare effective typed values as well as JSON presence. For example an
	// empty permissions list serializes as omitted on GET; do not patch it on
	// every apply. Explicit secrets are intentionally never compared this way.
	if len(patchSpec) > 0 {
		candidate, err := json.Marshal(patch)
		if err != nil {
			return nil, fmt.Errorf("cannot encode reconciled patch")
		}

		typedPatch, err := decodePatch(doc.Kind, candidate, current.Resource)
		if err != nil {
			return nil, err
		}

		descriptor, _ := resourceType(doc.Kind)
		updated, err := descriptor.ApplyPatch(current.Resource, typedPatch)
		if err != nil {
			return nil, fmt.Errorf("invalid reconciled resource")
		}

		effective, err := plainObject(updated)
		if err != nil {
			return nil, err
		}

		effectiveSpec := asMap(effective["spec"])

		for field := range patchSpec {
			if !merger.forced([]string{"spec", field}) && equalJSON(liveSpec[field], effectiveSpec[field]) {
				delete(patchSpec, field)
			}
		}
	}

	if len(patchMeta) == 0 && len(patchSpec) == 0 {
		plan.Operation = OperationUnchanged
		return plan, nil
	}

	data, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("cannot encode reconciled patch")
	}

	if _, err = decodePatch(doc.Kind, data, current.Resource); err != nil {
		return nil, err
	}

	plan.Operation = OperationUpdate
	plan.Patch = data

	return plan, nil
}

func documentWithObject(doc Document, object map[string]any) (Document, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return Document{}, fmt.Errorf("cannot encode planned resource")
	}
	resource, err := registry.NewResourceScheme().DecodeJSON(data)
	if err != nil {
		return Document{}, fmt.Errorf("invalid planned resource")
	}
	m, kind, err := resourceMetadata(resource)
	if err != nil {
		return Document{}, err
	}
	if kind != doc.Kind {
		return Document{}, fmt.Errorf("planned resource kind mismatch")
	}
	if _, ok := object["metadata"].(map[string]any); !ok {
		return Document{}, fmt.Errorf("planned metadata must be an object")
	}
	if spec, present := object["spec"]; present {
		if _, ok := spec.(map[string]any); !ok {
			return Document{}, fmt.Errorf("planned spec must be an object")
		}
	}
	if _, present := object["status"]; present {
		return Document{}, fmt.Errorf("status is server-owned")
	}
	if err := normalizeIdentity(string(kind), &m, ""); err != nil {
		return Document{}, err
	}
	doc.Object = object
	doc.Resource = resource
	doc.Metadata = m
	return doc, nil
}
func attachHistory(object map[string]any, h *History) error {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("cannot encode last-applied history")
	}
	m := liveMeta(object)
	annotations, ok := m["annotations"].(map[string]any)
	if !ok {
		annotations = map[string]any{}
		m["annotations"] = annotations
	}
	annotations[LastAppliedAnnotation] = string(data)
	typed := map[string]string{}
	for key, value := range annotations {
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("invalid annotations")
		}
		typed[key] = s
	}
	if err := meta.ValidateAnnotations(typed); err != nil {
		return fmt.Errorf("annotations including last-applied history are invalid or exceed 256 KiB")
	}
	return nil
}

func liveMeta(object map[string]any) map[string]any {
	m, _ := object["metadata"].(map[string]any)
	return m
}

func asMap(value any) map[string]any {
	result := map[string]any{}
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func equalJSON(a, b any) bool {
	x, e1 := json.Marshal(a)
	y, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && bytes.Equal(x, y)
}

func keys(maps ...map[string]any) []string {
	set := map[string]bool{}
	for _, m := range maps {
		for k := range m {
			set[k] = true
		}
	}
	result := []string{}
	for k := range set {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func clearSpecField(kind meta.Kind, field string) any {
	if kind == "Actor" && field == "permissions" {
		return []any{}
	}
	// Nullable fields clear with null; canonical patch validation rejects
	// removals that the resource contract does not support.
	return nil
}

type threeWay struct {
	overwrite bool
	force     map[string]bool
}

func (m threeWay) forced(path []string) bool {
	p := pointer(path)
	for f := range m.force {
		if f == p || strings.HasPrefix(f, p+"/") {
			return true
		}
	}
	return false
}

// merge reconciles one field or subtree across three versions:
//   - old is the previously applied desired value from sanitized history;
//     op reports whether that history contains the field.
//   - live is the current server value; lp reports whether the field is present
//     in the live resource.
//   - desired is the new manifest value; dp reports whether the manifest
//     explicitly supplies the field.
//   - path contains unescaped JSON property names (for example, {"spec",
//     "definition", "displayName"}) used for conflict locations and matching
//     forced secret writes. pointer converts these segments to a JSON pointer.
//
// Presence is independent of value: (nil, true) means explicit JSON null,
// whereas (nil, false) means omitted. The same distinction applies to the
// returned value and presence flag: false tells the caller to remove the field,
// while true retains the returned value, including null. An error reports a
// conflicting change when overwriting managed-field drift is disabled.
func (m threeWay) merge(
	old any,
	op bool,
	live any,
	lp bool,
	desired any,
	dp bool,
	path []string,
) (any, bool, error) {
	if !op && !dp {
		return live, lp, nil
	}

	if d, ok := desired.(map[string]any); dp && ok {
		o := asMap(old)
		l := asMap(live)
		result := asMap(live)

		for _, key := range keys(o, d) {
			ov, ob := o[key]
			lv, lb := l[key]
			dv, db := d[key]

			v, p, err := m.merge(ov, ob, lv, lb, dv, db, append(append([]string(nil), path...), key))
			if err != nil {
				return nil, false, err
			}

			if p {
				result[key] = v
			} else {
				delete(result, key)
			}
		}

		return result, true, nil
	}

	if !dp {
		// Omitting a managed object deletes only its managed children.
		if _, ok := old.(map[string]any); ok {
			v, _, err := m.merge(old, op, live, lp, map[string]any{}, true, path)
			if len(asMap(v)) > 0 {
				return v, true, err
			}
			return nil, false, err
		}
	}

	changed := dp != lp || !equalJSON(desired, live)
	if changed && op && !m.overwrite && (op != lp || !equalJSON(old, live)) && !m.forced(path) {
		return nil, false, fmt.Errorf("apply conflict at %s; live field differs from last-applied configuration", pointer(path))
	}

	return desired, dp, nil
}
