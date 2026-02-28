package config

import (
	"context"
	"sync"
	"time"
)

// keyVersionCacheEntry holds a cached KeyVersionInfo with its fetch time.
type keyVersionCacheEntry struct {
	info      KeyVersionInfo
	fetchedAt time.Time
}

// keyDataCache provides TTL-based caching at the KeyVersionInfo level.
// External providers (Vault, AWS, GCP) embed this and set fetch callbacks
// so that their public interface methods delegate to the cache.
type keyDataCache struct {
	mu  sync.Mutex
	ttl time.Duration

	currentVersion *keyVersionCacheEntry

	versionCache map[string]*keyVersionCacheEntry

	listVersions *struct {
		infos     []KeyVersionInfo
		fetchedAt time.Time
	}

	fetchCurrent func(ctx context.Context) (KeyVersionInfo, error)
	fetchVersion func(ctx context.Context, version string) (KeyVersionInfo, error)
	fetchList    func(ctx context.Context) ([]KeyVersionInfo, error)
}

func (c *keyDataCache) valid(fetchedAt time.Time) bool {
	return c.ttl <= 0 || time.Since(fetchedAt) < c.ttl
}

// GetCurrentVersion returns the cached current version or fetches it.
func (c *keyDataCache) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentVersion != nil && c.valid(c.currentVersion.fetchedAt) {
		return c.currentVersion.info, nil
	}

	info, err := c.fetchCurrent(ctx)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	c.currentVersion = &keyVersionCacheEntry{
		info:      info,
		fetchedAt: time.Now(),
	}
	return info, nil
}

// GetVersion returns a cached specific version or fetches it.
func (c *keyDataCache) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.versionCache != nil {
		if entry, ok := c.versionCache[version]; ok && c.valid(entry.fetchedAt) {
			return entry.info, nil
		}
	}

	info, err := c.fetchVersion(ctx, version)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	if c.versionCache == nil {
		c.versionCache = make(map[string]*keyVersionCacheEntry)
	}
	c.versionCache[version] = &keyVersionCacheEntry{
		info:      info,
		fetchedAt: time.Now(),
	}
	return info, nil
}

// ListVersions returns cached version list or fetches it.
func (c *keyDataCache) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.listVersions != nil && c.valid(c.listVersions.fetchedAt) {
		return c.listVersions.infos, nil
	}

	infos, err := c.fetchList(ctx)
	if err != nil {
		return nil, err
	}

	c.listVersions = &struct {
		infos     []KeyVersionInfo
		fetchedAt time.Time
	}{
		infos:     infos,
		fetchedAt: time.Now(),
	}
	return infos, nil
}
