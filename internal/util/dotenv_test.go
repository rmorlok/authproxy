package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDotEnvWalksParents(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("DOTENV_ROOT=root\nDOTENV_SHARED=root\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a", ".env"), []byte("DOTENV_MID=mid\nDOTENV_SHARED=mid\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nested, ".env"), []byte("DOTENV_NEAR=near\nDOTENV_SHARED=near\n"), 0o644))

	for _, k := range []string{"DOTENV_ROOT", "DOTENV_MID", "DOTENV_NEAR", "DOTENV_SHARED"} {
		require.NoError(t, os.Unsetenv(k))
	}
	t.Cleanup(func() {
		for _, k := range []string{"DOTENV_ROOT", "DOTENV_MID", "DOTENV_NEAR", "DOTENV_SHARED"} {
			_ = os.Unsetenv(k)
		}
	})

	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(orig) })
	require.NoError(t, os.Chdir(nested))

	LoadDotEnv()

	require.Equal(t, "root", os.Getenv("DOTENV_ROOT"))
	require.Equal(t, "mid", os.Getenv("DOTENV_MID"))
	require.Equal(t, "near", os.Getenv("DOTENV_NEAR"))
	require.Equal(t, "near", os.Getenv("DOTENV_SHARED"), "nearest .env should win for shared keys")
}
