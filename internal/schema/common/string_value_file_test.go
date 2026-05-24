package common

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	return path
}

func TestStringValueFile_HasValue(t *testing.T) {
	ctx := context.Background()

	t.Run("existing file", func(t *testing.T) {
		path := writeTempFile(t, "hello")
		v := &StringValueFile{Path: path}
		assert.True(t, v.HasValue(ctx))
	})

	t.Run("missing file", func(t *testing.T) {
		v := &StringValueFile{Path: "/nonexistent/path/should/not/exist.txt"}
		assert.False(t, v.HasValue(ctx))
	})
}

func TestStringValueFile_GetValue(t *testing.T) {
	ctx := context.Background()

	t.Run("reads file contents", func(t *testing.T) {
		path := writeTempFile(t, "contents of the file")
		v := &StringValueFile{Path: path}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "contents of the file", got)
	})

	t.Run("empty file returns empty string", func(t *testing.T) {
		path := writeTempFile(t, "")
		v := &StringValueFile{Path: path}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		v := &StringValueFile{Path: "/nonexistent/path/should/not/exist.txt"}
		_, err := v.GetValue(ctx)
		require.Error(t, err)
	})

	t.Run("expands home directory", func(t *testing.T) {
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		tmp, err := os.CreateTemp(home, "string_value_file_*.txt")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.Remove(tmp.Name()) })
		_, err = tmp.WriteString("from home")
		require.NoError(t, err)
		require.NoError(t, tmp.Close())

		rel := filepath.Join("~", filepath.Base(tmp.Name()))
		v := &StringValueFile{Path: rel}
		got, err := v.GetValue(ctx)
		require.NoError(t, err)
		assert.Equal(t, "from home", got)
	})
}

func TestStringValueFile_Clone(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var v *StringValueFile
		assert.Nil(t, v.Clone())
	})

	t.Run("clone is independent copy", func(t *testing.T) {
		orig := &StringValueFile{Path: "/some/path"}
		clone := orig.Clone().(*StringValueFile)
		assert.Equal(t, orig, clone)
		clone.Path = "/different"
		assert.NotEqual(t, orig.Path, clone.Path)
	})
}
