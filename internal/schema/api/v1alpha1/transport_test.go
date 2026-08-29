package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type testItem struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          struct {
		Enabled bool `json:"enabled" yaml:"enabled"`
	} `json:"spec" yaml:"spec"`
}

func TestResourceListConstructorAndSerialization(t *testing.T) {
	remaining := int64(4)
	list := NewResourceList[testItem]("Widget", nil, ListMeta{
		Continue:           "next-page",
		RemainingItemCount: &remaining,
	})
	require.Equal(t, meta.APIVersionV1Alpha1, list.APIVersion)
	require.Equal(t, meta.Kind("WidgetList"), list.Kind)
	require.NotNil(t, list.Items)

	data, err := json.Marshal(list)
	require.NoError(t, err)
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"WidgetList",
      "metadata":{"continue":"next-page","remainingItemCount":4},
      "items":[]
    }`, string(data))

	yamlData, err := yaml.Marshal(list)
	require.NoError(t, err)
	require.Contains(t, string(yamlData), "apiVersion: authproxy.net/v1alpha1\nkind: WidgetList\nmetadata:")
	require.Contains(t, string(yamlData), "items: []")
	require.NoError(t, list.Validate("Widget"))
}

func TestActionConstructorsPutTargetInMetadata(t *testing.T) {
	target := meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       "Connection",
		ID:         "cxn_123",
	}
	type disconnectSpec struct {
		TimeoutSeconds int `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	}
	type disconnectStatus struct {
		TaskID string `json:"taskId" yaml:"taskId"`
	}

	request := NewActionRequest("ConnectionDisconnect", target, disconnectSpec{TimeoutSeconds: 30})
	require.Nil(t, request.Status)
	requestJSON, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, `{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"ConnectionDisconnect",
      "metadata":{"target":{"apiVersion":"authproxy.net/v1alpha1","kind":"Connection","id":"cxn_123"}},
      "spec":{"timeoutSeconds":30}
    }`, string(requestJSON))

	response := NewActionResponse("ConnectionDisconnect", target, disconnectSpec{TimeoutSeconds: 30}, disconnectStatus{TaskID: "task-1"})
	require.Equal(t, "task-1", response.Status.TaskID)
	require.NoError(t, response.Validate("ConnectionDisconnect"))
	require.NoError(t, response.ValidateResponse("ConnectionDisconnect"))
	require.ErrorContains(t, response.ValidateRequest("ConnectionDisconnect"), "$.status: is server-owned")
}

func TestListAndActionValidationIsFieldAddressable(t *testing.T) {
	list := ResourceList[testItem]{TypeMeta: meta.TypeMeta{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       "GadgetList",
	}}
	err := list.Validate("Widget")
	require.ErrorContains(t, err, `$.kind: must be "WidgetList"`)

	action := Action[struct{}, struct{}]{TypeMeta: meta.NewTypeMeta("WidgetList")}
	err = action.Validate("WidgetList")
	require.ErrorContains(t, err, "must not be a list kind")

	negative := int64(-1)
	list = NewResourceList[testItem]("Widget", nil, ListMeta{RemainingItemCount: &negative})
	require.ErrorContains(t, list.Validate("Widget"), "$.metadata.remainingItemCount")

	request := NewActionRequest("WidgetAction", meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       "Widget",
		Name:       "example",
	}, struct{}{})
	require.NoError(t, request.Validate("WidgetAction"))

	request.Metadata.Target.Name = ""
	require.ErrorContains(t, request.Validate("WidgetAction"), "$.metadata.target: must contain id or name")
}
