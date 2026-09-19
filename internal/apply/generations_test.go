package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// This synthetic lifecycle uses Actor's wire decoder as a stand-in for a future
// resource, a different collection, and metadata labels instead of release/spec
// fields. It is installed on a descriptor copy, never the global registry.
type buildLifecycle struct{}
type buildSelection struct{ publish bool }

func (buildLifecycle) State(resource any) (meta.GenerationState, error) {
	switch resource.(*actor.Actor).Metadata.Labels["phase"] {
	case "workbench":
		return meta.GenerationEditable, nil
	case "released":
		return meta.GenerationPublished, nil
	default:
		return meta.GenerationHistorical, nil
	}
}
func (buildLifecycle) ChangesGeneration(patch any) (bool, error) {
	return patch.(*actor.ActorPatch).Metadata.Labels != nil, nil
}
func (buildLifecycle) Select(desired, selected any, changed, hasEditable bool) (meta.GenerationSelection, error) {
	if !changed {
		return meta.GenerationSelection{Source: meta.GenerationSelected}, nil
	}
	source := meta.GenerationNewest
	if hasEditable {
		source = meta.GenerationEditableSource
	}
	return meta.GenerationSelection{Source: source, Context: buildSelection{publish: desired.(*actor.Actor).Metadata.Labels["publish"] == "yes"}}, nil
}
func (buildLifecycle) Finalize(desired, current, patch any, ctx any, explicit bool) (any, error) {
	p := patch.(*actor.ActorPatch)
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var result actor.ActorPatch
	if err = json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if explicit {
		return nil, fmt.Errorf("build generation is immutable")
	}
	if selection, ok := ctx.(buildSelection); ok && selection.publish {
		labels := map[string]string{}
		if result.Metadata.Labels != nil {
			for k, v := range *result.Metadata.Labels {
				labels[k] = v
			}
		}
		labels["phase"] = "released"
		result.Metadata.Labels = &labels
	}
	return &result, nil
}

func TestGenerationOrchestrationUsesRegisteredLifecycle(t *testing.T) {
	for _, hasEditable := range []bool{false, true} {
		t.Run(fmt.Sprintf("editable=%t", hasEditable), func(t *testing.T) {
			id := apid.New(apid.PrefixActor).String()
			resource := func(g int, phase string) string {
				return fmt.Sprintf(`{"apiVersion":"authproxy.net/v1alpha1","kind":"Actor","metadata":{"id":%q,"name":"example","namespace":"root","generation":%d,"labels":{"phase":%q}},"spec":{"externalId":"subject"}}`, id, g, phase)
			}
			selectedJSON := resource(1, "released")
			latestPhase := "retired"
			if hasEditable {
				latestPhase = "workbench"
			}
			latestJSON := resource(2, latestPhase)
			requests := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/api/v1/builds/"+id+"/generations", r.URL.Path)
				if r.URL.Query().Get("cursor") == "" {
					fmt.Fprint(w, listJSON("Actor", []string{selectedJSON}, "next"))
				} else {
					require.Equal(t, "next", r.URL.Query().Get("cursor"))
					fmt.Fprint(w, listJSON("Actor", []string{latestJSON}, ""))
				}
			}, false)
			descriptor, err := resourceType("Actor")
			require.NoError(t, err)
			require.Nil(t, descriptor.Generations)
			descriptor.Collection = "builds"
			descriptor.Generations = buildLifecycle{}
			selected, err := c.decodeLive("Actor", []byte(selectedJSON), false)
			require.NoError(t, err)
			doc := clientDoc(t, "Actor", "  name: example\n  namespace: root\n  labels: {publish: yes}", `{"externalId":"subject"}`)
			chosen, err := c.selectGenerationTarget(context.Background(), doc, selected, descriptor)
			require.NoError(t, err)
			require.Equal(t, uint64(2), chosen.Metadata.Generation)
			require.Equal(t, 2, requests)
			require.Nil(t, selected.generationContext)
			patch := map[string]any{"apiVersion": string(meta.APIVersionV1Alpha1), "kind": "Actor", "metadata": map[string]any{}, "spec": map[string]any{}}
			finalized, err := finalizeGenerationPatch(descriptor, doc, chosen, patch)
			require.NoError(t, err)
			require.Equal(t, "released", finalized["metadata"].(map[string]any)["labels"].(map[string]any)["phase"])
			require.Empty(t, patch["metadata"])
			doc.Metadata.Generation = 2
			_, err = finalizeGenerationPatch(descriptor, doc, chosen, patch)
			require.ErrorContains(t, err, "build generation is immutable")
			original, _ := resourceType("Actor")
			require.Nil(t, original.Generations)
		})
	}
}
