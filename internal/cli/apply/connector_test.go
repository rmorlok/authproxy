package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// Models the REST distinction between selected generations and the editable
// draft. Metadata is shared by all generations, as in the persistence layer.
type connectorServer struct {
	t           *testing.T
	id          string
	generations map[uint64]*connectors.Connector
	patches     []connectors.ConnectorPatch
	paths       []string
}

func newConnectorServer(t *testing.T) (*connectorServer, *Client) {
	s := &connectorServer{t: t, id: apid.New(apid.PrefixConnector).String(), generations: map[uint64]*connectors.Connector{}}
	return s, testClient(t, s.handle, false)
}

func (s *connectorServer) add(g uint64, state connectors.ConnectorReleaseState, display string) *connectors.Connector {
	c := connectors.NewConnector()
	c.Metadata = meta.ObjectMeta{
		ID:         s.id,
		Name:       common.ResourceName("example"),
		Namespace:  "root",
		Generation: g,
	}
	require.NoError(s.t, json.Unmarshal([]byte(fmt.Sprintf(`{"displayName":%q,"auth":{"type":"no-auth"}}`, display)), &c.Spec.Definition))

	c.Spec.Release.DesiredState = connectors.DesiredReleaseStateForObserved(state)
	c.Status = &connectors.ConnectorStatus{
		Release: connectors.ConnectorReleaseStatus{
			State: state,
		},
	}
	s.generations[g] = c

	return c
}

func (s *connectorServer) selected() *connectors.Connector {
	var selected *connectors.Connector
	ranks := map[connectors.ConnectorReleaseState]int{"primary": 0, "draft": 1, "active": 2, "archived": 3}
	for _, c := range s.generations {
		if selected == nil ||
			ranks[c.Status.Release.State] < ranks[selected.Status.Release.State] ||
			(c.Status.Release.State == selected.Status.Release.State &&
				c.Metadata.Generation > selected.Metadata.Generation) {
			selected = c
		}
	}
	return selected
}

func (s *connectorServer) handle(w http.ResponseWriter, r *http.Request) {
	t := s.t
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	if path == "namespaces/root" {
		fmt.Fprint(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"Namespace","metadata":{"id":"root","name":"root"},"spec":{}}`)
		return
	}
	selected := s.selected()
	base := "connectors/" + s.id
	encode := func(v any) { require.NoError(t, json.NewEncoder(w).Encode(v)) }
	if r.Method == "GET" {
		switch path {
		case "connectors":
			var items []string
			if selected != nil {
				b, _ := json.Marshal(selected)
				items = append(items, string(b))
			}
			fmt.Fprint(w, listJSON("Connector", items, ""))
		case base:
			if selected == nil {
				w.WriteHeader(404)
			} else {
				encode(selected)
			}
		case base + "/generations":
			// Two pages deliberately place the draft after the primary.
			keys := make([]int, 0, len(s.generations))
			for g := range s.generations {
				keys = append(keys, int(g))
			}
			sort.Ints(keys)
			var items []string
			cursor := ""
			for i, g := range keys {
				if len(keys) > 1 && ((r.URL.Query().Get("cursor") == "" && i > 0) || (r.URL.Query().Get("cursor") != "" && i == 0)) {
					continue
				}
				b, _ := json.Marshal(s.generations[uint64(g)])
				items = append(items, string(b))
			}
			if len(keys) > 1 && r.URL.Query().Get("cursor") == "" {
				cursor = "next"
			}
			fmt.Fprint(w, listJSON("Connector", items, cursor))
		default:
			g, _ := strconv.ParseUint(strings.TrimPrefix(path, base+"/generations/"), 10, 64)
			if c := s.generations[g]; c != nil {
				encode(c)
			} else {
				w.WriteHeader(404)
			}
		}
		return
	}
	require.Equal(t, "PATCH", r.Method)
	require.NotNil(t, selected)

	var patch connectors.ConnectorPatch
	require.NoError(t, json.NewDecoder(r.Body).Decode(&patch))
	s.patches = append(s.patches, patch)
	s.paths = append(s.paths, path)

	// Match the logical endpoint's release-only primary shortcut.
	if path == base && !patch.Spec.HasDefinition() && patch.Spec.HasRelease() && patch.Spec.Release.DesiredState != nil && *patch.Spec.Release.DesiredState == connectors.ConnectorReleaseStatePrimary && selected.Status.Release.State == connectors.ConnectorReleaseStatePrimary {
		encode(selected)
		return
	}

	current := selected

	if path != base {
		g, err := strconv.ParseUint(strings.TrimPrefix(path, base+"/generations/"), 10, 64)
		require.NoError(t, err)

		current = s.generations[g]
		require.NotNil(t, current)
		require.Equal(t, connectors.ConnectorReleaseStateDraft, current.Status.Release.State)
	} else if patch.Spec.HasDefinition() || patch.Spec.HasRelease() {
		var draft *connectors.Connector
		var newest *connectors.Connector
		for _, c := range s.generations {
			if c.Status.Release.State == connectors.ConnectorReleaseStateDraft {
				draft = c
			}

			if newest == nil || c.Metadata.Generation > newest.Metadata.Generation {
				newest = c
			}
		}

		if draft == nil {
			draft = newest.Clone()
			draft.Metadata.Generation++
			draft.Status.Release.State = connectors.ConnectorReleaseStateDraft
			draft.Spec.Release.DesiredState = connectors.ConnectorReleaseStateDraft
			s.generations[draft.Metadata.Generation] = draft
		}

		current = draft
	}

	updated, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)

	updated.Status.Release.State = updated.Spec.Release.DesiredState

	if updated.Status.Release.State == connectors.ConnectorReleaseStatePrimary {
		for _, c := range s.generations {
			if c.Status.Release.State == connectors.ConnectorReleaseStatePrimary {
				c.Status.Release.State = connectors.ConnectorReleaseStateActive
			}
		}
	}

	s.generations[updated.Metadata.Generation] = updated

	for _, c := range s.generations {
		c.Metadata.Annotations = updated.Metadata.Annotations
		c.Metadata.Labels = updated.Metadata.Labels
	}

	encode(updated)
}

func TestConnectorApplyLifecycle(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, state string
		draft       bool
	}{
		{"changed published definition becomes draft", "", false},
		{"changed published definition is explicitly published", "primary", false},
		{"existing draft is reused", "draft", true},
		{"existing draft is published", "primary", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := newConnectorServer(t)
			s.add(1, "primary", "Old")
			if tc.draft {
				d := s.add(2, "draft", "Work in progress")
				d.Spec.Definition.Description = "unmanaged draft detail"
			}
			spec := `{"definition":{"displayName":"New","auth":{"type":"no-auth"}}}`
			if tc.state != "" {
				spec = fmt.Sprintf(`{"definition":{"displayName":"New","auth":{"type":"no-auth"}},"release":{"desiredState":%q}}`, tc.state)
			}
			doc := batchDoc(t, "Connector", "example", "root", spec)
			for i := 0; i < 3; i++ {
				b, err := c.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
				require.NoError(t, err)
				results, err := b.Execute(ctx)
				require.NoError(t, err)
				if i == 2 {
					require.Equal(t, "unchanged", results[0].Status)
				}
			}
			require.Len(t, s.generations, 2)
			final := s.generations[2]
			require.Equal(t, "New", final.Spec.Definition.DisplayName)
			want := connectors.ConnectorReleaseState(tc.state)
			if want == "" {
				want = "draft"
			}
			require.Equal(t, want, final.Status.Release.State)
			if tc.draft {
				require.Equal(t, "unmanaged draft detail", final.Spec.Definition.Description)
			}
			require.True(t, s.patches[0].Spec.HasDefinition())
			if tc.state == "primary" {
				require.Equal(t, connectors.ConnectorReleaseStatePrimary, *s.patches[0].Spec.Release.DesiredState)
			}
		})
	}
	t.Run("a satisfied primary declaration leaves an unrelated draft alone", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "primary", "Same")
		s.add(2, "draft", "Unrelated")
		doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"Same","auth":{"type":"no-auth"}},"release":{"desiredState":"primary"}}`)
		for i := 0; i < 2; i++ {
			b, err := c.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
			require.NoError(t, err)
			_, err = b.Execute(ctx)
			require.NoError(t, err)
		}
		require.Len(t, s.patches, 1)
		require.False(t, s.patches[0].Spec.HasDefinition())
		require.False(t, s.patches[0].Spec.HasRelease())
		require.Equal(t, connectors.ConnectorReleaseStateDraft, s.generations[2].Status.Release.State)
		require.Equal(t, "Unrelated", s.generations[2].Spec.Definition.DisplayName)
	})
	t.Run("new drafts merge against the newest generation", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "primary", "Old")
		latest := s.add(2, "archived", "Latest")
		latest.Spec.Definition.Description = "newest detail"
		doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"New","auth":{"type":"no-auth"}}}`)
		b, err := c.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		_, err = b.Execute(ctx)
		require.NoError(t, err)
		require.Equal(t, "newest detail", s.generations[3].Spec.Definition.Description)
	})
	t.Run("explicit draft addressing stays on that generation", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "primary", "Old")
		s.add(2, "draft", "Draft")
		doc := clientDoc(t, "Connector", "  id: "+s.id+"\n  generation: 2", `{"definition":{"displayName":"New"}}`)
		b, err := c.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		_, err = b.Execute(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"connectors/" + s.id + "/generations/2"}, s.paths)
	})
	t.Run("explicit published generations cannot be mutated", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "primary", "Old")
		doc := clientDoc(t, "Connector", "  id: "+s.id+"\n  generation: 1", `{"definition":{"displayName":"New"}}`)
		_, err := c.Prepare(ctx, []Document{doc}, ReconcileOptions{Overwrite: true})
		require.ErrorContains(t, err, "not a draft")
		require.Empty(t, s.patches)
	})
}

func TestConnectorNewestEqualDefinitionStillCreatesDraft(t *testing.T) {
	s, c := newConnectorServer(t)
	s.add(1, "primary", "Old")
	s.add(2, "archived", "New")
	doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"New","auth":{"type":"no-auth"}}}`)
	b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
	require.NoError(t, err)
	_, err = b.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, s.generations, 3)
	require.Equal(t, connectors.ConnectorReleaseStateDraft, s.generations[3].Status.Release.State)
	require.Equal(t, "New", s.generations[3].Spec.Definition.DisplayName)
}

func TestConnectorReleaseAndRefresh(t *testing.T) {
	t.Run("logical primary intent publishes from an archived source", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "archived", "Same")
		doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"Same","auth":{"type":"no-auth"}},"release":{"desiredState":"primary"}}`)
		b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		_, err = b.Execute(context.Background())
		require.NoError(t, err)
		require.Len(t, s.generations, 2)
		require.Equal(t, connectors.ConnectorReleaseStatePrimary, s.generations[2].Status.Release.State)
	})
	t.Run("refresh preserves unrelated draft edits and metadata", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "primary", "Old")
		draft := s.add(2, "draft", "Draft")
		doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"New","auth":{"type":"no-auth"}}}`)
		b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		draft.Spec.Definition.Description = "concurrent description"
		for _, g := range s.generations {
			g.Metadata.Labels = map[string]string{"external": "preserved"}
			g.Metadata.Annotations = map[string]string{"example.com/external": "preserved"}
		}
		_, err = b.Execute(context.Background())
		require.NoError(t, err)
		require.Equal(t, "concurrent description", s.generations[2].Spec.Definition.Description)
		require.Equal(t, "preserved", s.generations[2].Metadata.Labels["external"])
		require.Equal(t, "preserved", s.generations[2].Metadata.Annotations["example.com/external"])
	})
	t.Run("overwrite false compares managed fields against draft not primary", func(t *testing.T) {
		s, c := newConnectorServer(t)
		s.add(1, "primary", "Old")
		s.add(2, "draft", "Desired")
		doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"Desired","auth":{"type":"no-auth"}},"release":{"desiredState":"draft"}}`)
		b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: false})
		require.NoError(t, err)
		_, err = b.Execute(context.Background())
		require.NoError(t, err)
		b, err = c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: false})
		require.NoError(t, err)
		s.generations[2].Spec.Definition.DisplayName = "Changed externally"
		before := len(s.patches)
		results, err := b.Execute(context.Background())
		require.ErrorIs(t, err, ErrBatchFailed)
		require.Contains(t, results[0].Error, "conflict")
		require.Len(t, s.patches, before)
	})
	t.Run("published explicit generation may be unchanged but cannot adopt history", func(t *testing.T) {
		s, c := newConnectorServer(t)
		current := s.add(1, "primary", "Same")
		doc := clientDoc(t, "Connector", "  id: "+s.id+"\n  generation: 1", `{"definition":{"displayName":"Same","auth":{"type":"no-auth"}}}`)
		h, err := newHistory(doc)
		require.NoError(t, err)
		h.Desired["metadata"].(map[string]any)["namespace"] = "root"
		encoded, err := json.Marshal(h)
		require.NoError(t, err)
		current.Metadata.Annotations = map[string]string{LastAppliedAnnotation: string(encoded)}
		b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
		require.NoError(t, err)
		results, err := b.Execute(context.Background())
		require.NoError(t, err)
		require.Equal(t, "unchanged", results[0].Status)
		require.Empty(t, s.patches)
		current.Metadata.Annotations = nil
		_, err = c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
		require.ErrorContains(t, err, "not a draft")
	})
}

func TestConnectorGenerationDiscoveryFailures(t *testing.T) {
	for _, tc := range []string{
		"forbidden",
		"missing selected",
		"duplicate generation",
		"wrong identity",
		"multiple drafts",
		"missing status",
		"invalid list",
		"incomplete list",
		"repeated cursor",
	} {
		t.Run(tc, func(t *testing.T) {
			s, _ := newConnectorServer(t)
			selected := s.add(1, "primary", "Old")
			bytes, err := json.Marshal(selected)
			require.NoError(t, err)
			writes := 0
			pages := 0

			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
					w.WriteHeader(500)
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/generations") {
					s.handle(w, r)
					return
				}
				pages++
				bad := selected.Clone()
				switch tc {
				case "forbidden":
					w.WriteHeader(403)
				case "missing selected":
					fmt.Fprint(w, listJSON("Connector", nil, ""))
				case "duplicate generation":
					fmt.Fprint(w, listJSON("Connector", []string{string(bytes), string(bytes)}, ""))
				case "wrong identity":
					bad.Metadata.ID = apid.New(apid.PrefixConnector).String()
					b, _ := json.Marshal(bad)
					fmt.Fprint(w, listJSON("Connector", []string{string(b)}, ""))
				case "multiple drafts":
					bad.Status.Release.State = "draft"
					bad.Metadata.Generation = 2
					b, _ := json.Marshal(bad)
					bad.Metadata.Generation = 3
					d, _ := json.Marshal(bad)
					fmt.Fprint(w, listJSON("Connector", []string{string(bytes), string(b), string(d)}, ""))
				case "missing status":
					bad.Status = nil
					b, _ := json.Marshal(bad)
					fmt.Fprint(w, listJSON("Connector", []string{string(b)}, ""))
				case "invalid list":
					fmt.Fprint(w, `{"items":[]}`)
				case "incomplete list":
					fmt.Fprintf(w, `{"apiVersion":"authproxy.net/v1alpha1","kind":"ConnectorList","metadata":{"remainingItemCount":1},"items":[%s]}`, bytes)
				case "repeated cursor":
					items := []string{}
					if pages == 1 {
						items = append(items, string(bytes))
					}
					fmt.Fprint(w, listJSON("Connector", items, "again"))
				}
			}, false)

			doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"New"}}`)

			_, err = c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
			require.Error(t, err)
			require.Zero(t, writes)
			require.LessOrEqual(t, pages, 2)
		})
	}
}

func TestConnectorPublishesMatchingDraftDefinition(t *testing.T) {
	for _, state := range []connectors.ConnectorReleaseState{"draft", "archived"} {
		t.Run(string(state), func(t *testing.T) {
			s, c := newConnectorServer(t)
			s.add(1, "primary", "Old")
			s.add(2, state, "New")
			doc := batchDoc(t, "Connector", "example", "root", `{"definition":{"displayName":"New","auth":{"type":"no-auth"}},"release":{"desiredState":"primary"}}`)
			b, err := c.Prepare(context.Background(), []Document{doc}, ReconcileOptions{Overwrite: true})
			require.NoError(t, err)
			_, err = b.Execute(context.Background())
			require.NoError(t, err)
			require.Equal(t, "New", s.selected().Spec.Definition.DisplayName)
			require.True(t, s.patches[0].Spec.HasDefinition())
			require.Equal(t, connectors.ConnectorReleaseStatePrimary, s.selected().Status.Release.State)
		})
	}
}

func TestConnectorPublicationCannotReplayMaskedSecrets(t *testing.T) {
	spec := `{"definition":{"displayName":"Example","auth":{"type":"OAuth2","clientId":{"value":"client"},"clientSecret":{"value":"confidential-material"},"authorization":{"endpoint":"https://example.com/auth"},"token":{"endpoint":"https://example.com/token"}}}}`
	old := batchDoc(t, "Connector", "example", "root", spec)
	live := withHistory(t, reconcileLive(t, "Connector", apid.New(apid.PrefixConnector).String(), strings.ReplaceAll(spec, "confidential-material", "********"), nil), old)
	selected := live.Resource.(*connectors.Connector).Clone()
	selected.Status = &connectors.ConnectorStatus{Release: connectors.ConnectorReleaseStatus{State: connectors.ConnectorReleaseStatePrimary}}
	intent := selected.Clone()
	intent.Spec.Release.DesiredState = connectors.ConnectorReleaseStatePrimary
	descriptor, err := resourceType("Connector")
	require.NoError(t, err)
	selection, err := descriptor.Generations.Select(intent, selected, true, true)
	require.NoError(t, err)
	live.generationContext = selection.Context
	desired := batchDoc(t, "Connector", "example", "root", strings.Replace(spec, `"clientSecret":{"value":"confidential-material"},`, "", 1))
	_, err = Reconcile(Target{Document: desired, Current: live}, ReconcileOptions{Overwrite: true})
	require.ErrorContains(t, err, "redacted placeholders")
	require.NotContains(t, err.Error(), "confidential-material")
}
