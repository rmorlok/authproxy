package core

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
)

func TestActor(t *testing.T) {
	t.Parallel()
	t.Run("GetExternalId", func(t *testing.T) {
		u := Actor{}
		assert.Equal(t, "", u.GetExternalId())

		u.ExternalId = "test-user"
		assert.Equal(t, "test-user", u.GetExternalId())
	})
	t.Run("GetNamespace", func(t *testing.T) {
		u := Actor{}
		assert.Equal(t, "", u.GetNamespace())

		u.Namespace = "test-namespace"
		assert.Equal(t, "test-namespace", u.GetNamespace())
	})
	t.Run("GetName", func(t *testing.T) {
		u := Actor{Id: apid.MustParse("act_test1234567890ab")}
		assert.Equal(t, scommon.ResourceName("act_test1234567890ab"), u.GetName())

		u.Name = "billing-user"
		assert.Equal(t, scommon.ResourceName("billing-user"), u.GetName())
	})
	t.Run("GetLabels", func(t *testing.T) {
		u := Actor{}
		assert.Nil(t, u.GetLabels())

		u.Labels = map[string]string{"key": "value"}
		assert.Equal(t, map[string]string{"key": "value"}, u.GetLabels())
	})
}
