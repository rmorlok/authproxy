package iface

import (
	"testing"

	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/require"
)

// TestHTTPClientActorValidate accepts only the two explicit choices and rejects
// the zero value and every degenerate ActingAs argument.
func TestHTTPClientActorValidate(t *testing.T) {
	var typedNil *authcore.Actor
	actor := &authcore.Actor{Id: apid.New(apid.PrefixActor)}

	// Explicit choices are valid; ActingAs(nil) is never the opt-out.
	require.NoError(t, ActingAs(actor).Validate())
	require.NoError(t, WithoutActor().Validate())
	require.True(t, WithoutActor().IsWithoutActor())
	require.False(t, ActingAs(actor).IsWithoutActor())
	require.Nil(t, WithoutActor().Actor())
	require.Same(t, actor, ActingAs(actor).Actor())

	// Missing, typed-nil, and id-less actors all fail the same way.
	for _, choice := range []HTTPClientActor{
		{},
		ActingAs(nil),
		ActingAs(typedNil),
		ActingAs(&authcore.Actor{}),
	} {
		require.ErrorIs(t, choice.Validate(), ErrHTTPClientActorRequired)
		require.False(t, choice.IsWithoutActor())
	}
}

// normalizedActor runs choice through Normalize, as client construction does,
// so the actor id is captured for Agrees.
func normalizedActor(t *testing.T, choice HTTPClientActor) HTTPClientActor {
	t.Helper()
	options, err := HTTPClientOptions{Actor: choice}.Normalize()
	require.NoError(t, err)
	return options.Actor
}

// TestHTTPClientActorAgrees compares context actors by identity and treats an
// actor-free context as consistent with either choice.
func TestHTTPClientActorAgrees(t *testing.T) {
	id := apid.New(apid.PrefixActor)
	configured := normalizedActor(t, ActingAs(&authcore.Actor{Id: id}))
	withoutActor := normalizedActor(t, WithoutActor())

	// An absent context actor never conflicts.
	require.True(t, configured.Agrees(nil))
	require.True(t, withoutActor.Agrees(nil))

	// Identity, not object identity, decides agreement for ActingAs.
	require.True(t, configured.Agrees(&authcore.Actor{Id: id}))
	require.False(t, configured.Agrees(&authcore.Actor{Id: apid.New(apid.PrefixActor)}))

	// The opt-out conflicts with any context actor.
	require.False(t, withoutActor.Agrees(&authcore.Actor{Id: id}))

	// An un-normalized ActingAs has no captured id and refuses context actors.
	require.False(t, ActingAs(&authcore.Actor{Id: id}).Agrees(&authcore.Actor{Id: id}))
}

// TestHTTPClientActorAgreesUsesCapturedID pins agreement to the id captured at
// normalization, so mutating the actor object afterwards changes nothing.
func TestHTTPClientActorAgreesUsesCapturedID(t *testing.T) {
	originalID := apid.New(apid.PrefixActor)
	actor := &authcore.Actor{Id: originalID}
	configured := normalizedActor(t, ActingAs(actor))

	// Rewrite the shared actor's id after the choice was normalized.
	actor.Id = apid.New(apid.PrefixActor)

	// The new id is rejected and the captured original id still agrees.
	require.False(t, configured.Agrees(&authcore.Actor{Id: actor.Id}))
	require.True(t, configured.Agrees(&authcore.Actor{Id: originalID}))
}

// TestHTTPClientOptionsNormalize requires an actor, applies defaults, and
// detaches labels from the caller's map.
func TestHTTPClientOptionsNormalize(t *testing.T) {
	_, err := HTTPClientOptions{}.Normalize()
	require.ErrorIs(t, err, ErrHTTPClientActorRequired)

	// Defaults fill an empty request type; an explicit one is preserved.
	labels := map[string]string{"team": "captured"}
	normalized, err := HTTPClientOptions{Labels: labels, Actor: WithoutActor()}.Normalize()
	require.NoError(t, err)
	require.Equal(t, httpf.RequestTypeProxy, normalized.RequestType)
	explicit, err := HTTPClientOptions{RequestType: httpf.RequestTypeProbe, Actor: WithoutActor()}.Normalize()
	require.NoError(t, err)
	require.Equal(t, httpf.RequestTypeProbe, explicit.RequestType)

	// Later caller edits cannot reach the normalized copy.
	labels["team"] = "changed"
	require.Equal(t, map[string]string{"team": "captured"}, normalized.Labels)
}
