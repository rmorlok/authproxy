package helpers

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// marketplaceDistRel is the path of the marketplace UI's vite build output,
// relative to the repository root.
const marketplaceDistRel = "ui/marketplace/dist"

// marketplaceBuildOnce ensures we only run the build once per `go test`
// process even if multiple tests call EnsureMarketplaceBuilt.
var (
	marketplaceBuildOnce sync.Once
	marketplaceBuildErr  error
)

// EnsureMarketplaceBuilt makes sure ui/marketplace/dist/index.html exists,
// running `yarn workspace @authproxy/marketplace build` once per test process
// if the dist is missing. Tests that drive the marketplace UI through a real
// browser depend on this; the public service's static config points at the
// resulting dist directory.
//
// VITE_PUBLIC_BASE_URL is set to the empty string for the build so the SPA's
// API calls are same-origin — the test's public service serves both the SPA
// and the JSON API on the same listener.
func EnsureMarketplaceBuilt(t *testing.T) {
	t.Helper()

	root := repoRoot()
	indexPath := filepath.Join(root, marketplaceDistRel, "index.html")

	marketplaceBuildOnce.Do(func() {
		if _, err := os.Stat(indexPath); err == nil {
			return
		}

		cmd := exec.Command("yarn", "workspace", "@authproxy/marketplace", "build")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "VITE_PUBLIC_BASE_URL=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			marketplaceBuildErr = err
			t.Logf("marketplace build failed:\n%s", string(out))
		}
	})

	require.NoErrorf(t, marketplaceBuildErr,
		"marketplace UI build failed; run `yarn workspace @authproxy/marketplace build` locally to debug")
	_, err := os.Stat(indexPath)
	require.NoErrorf(t, err, "expected marketplace dist at %s after build", indexPath)
}

// MarketplaceDistPath returns the absolute path to the built marketplace UI.
func MarketplaceDistPath() string {
	return filepath.Join(repoRoot(), marketplaceDistRel)
}
