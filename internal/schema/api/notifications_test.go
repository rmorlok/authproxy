package api

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestNotificationListAndViewActions(t *testing.T) {
	id := apid.New(apid.PrefixNotification)
	target := NewNotificationReference(id)

	list := NewListNotificationsResponseJson(nil, "next")
	require.Equal(t, meta.NewTypeMeta("NotificationList"), list.TypeMeta)
	require.Empty(t, list.Items)
	require.Equal(t, "next", list.Metadata.Continue)

	request := NewNotificationViewRequest(target)
	require.NoError(t, request.ValidateRequest(NotificationViewActionKind))

	response := NewNotificationViewResponse(target)
	require.NoError(t, response.ValidateResponse(NotificationViewActionKind))
	require.True(t, response.Status.Viewed)
}

func TestNotificationViewActionRejectsInvalidTarget(t *testing.T) {
	request := NewNotificationViewRequest(meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       NotificationKind,
		Namespace:  "root",
		Name:       "notice",
	})
	require.ErrorContains(t, request.ValidateRequest(NotificationViewActionKind), "support id only")
}

func TestNotificationBatchViewActionValidation(t *testing.T) {
	first := NewNotificationReference(apid.New(apid.PrefixNotification))
	second := NewNotificationReference(apid.New(apid.PrefixNotification))
	request := NotificationBatchViewAction{
		TypeMeta: meta.NewTypeMeta(NotificationBatchViewActionKind),
		Metadata: &NotificationBatchViewMetadata{Targets: []meta.ObjectReference{first, second}},
		Spec:     &NotificationBatchViewSpec{},
	}
	require.NoError(t, request.ValidateRequest(NotificationBatchViewActionKind))

	response := NewNotificationBatchViewResponse(request.Metadata.Targets)
	require.NoError(t, response.ValidateResponse(NotificationBatchViewActionKind))

	request.Metadata.Targets = append(request.Metadata.Targets, first)
	require.ErrorContains(t, request.ValidateRequest(NotificationBatchViewActionKind), "duplicate id")
}

func TestNotificationBatchViewActionStrictJSON(t *testing.T) {
	var action NotificationBatchViewAction
	err := util.DecodeJSONStrict([]byte(`{
		"apiVersion":"authproxy.net/v1alpha1",
		"kind":"NotificationBatchView",
		"metadata":{"targets":[]},
		"spec":{}
	}`), &action)
	require.NoError(t, err)
	require.ErrorContains(t, action.ValidateRequest(NotificationBatchViewActionKind), "must not be empty")
}
