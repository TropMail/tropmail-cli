// Package cache stores fetched email bodies on disk.
//
// Email bodies are immutable once delivered, so a hit never needs revalidation
// and re-reading a message costs no network round trip. Only body content is
// cached; mutable fields such as state and action status always come from the API.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultTTL bounds how long a cached body is served, mostly to keep the
// directory from growing without limit.
const DefaultTTL = 30 * 24 * time.Hour

// Cache is a content-addressed store under ~/.cache/tropmail.
type Cache struct {
	dir      string
	ttl      time.Duration
	disabled bool
}

type entry struct {
	StoredAt time.Time       `json:"stored_at"`
	Payload  json.RawMessage `json:"payload"`
}

// Dir returns the cache directory, honouring TROPMAIL_CACHE_DIR.
func Dir() (string, error) {
	if custom := os.Getenv("TROPMAIL_CACHE_DIR"); custom != "" {
		return custom, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate cache directory: %w", err)
	}
	return filepath.Join(base, "tropmail"), nil
}

// New opens the cache. A cache that cannot be located is disabled rather than
// fatal: caching is an optimisation, never a requirement.
func New(enabled bool) *Cache {
	if !enabled {
		return &Cache{disabled: true}
	}
	dir, err := Dir()
	if err != nil {
		return &Cache{disabled: true}
	}
	return &Cache{dir: filepath.Join(dir, "bodies"), ttl: DefaultTTL}
}

func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	// Shard by the first byte so large mailboxes do not create one huge directory.
	return filepath.Join(c.dir, name[:2], name+".json")
}

// Get decodes a cached value into out and reports whether it was a hit.
func (c *Cache) Get(key string, out any) bool {
	if c.disabled {
		return false
	}
	raw, err := os.ReadFile(c.path(key))
	if err != nil {
		return false
	}
	var stored entry
	if err := json.Unmarshal(raw, &stored); err != nil {
		return false
	}
	if c.ttl > 0 && time.Since(stored.StoredAt) > c.ttl {
		_ = os.Remove(c.path(key))
		return false
	}
	return json.Unmarshal(stored.Payload, out) == nil
}

// Put stores a value. Failures are silent: a cache miss is always acceptable.
func (c *Cache) Put(key string, value any) {
	if c.disabled {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	encoded, err := json.Marshal(entry{StoredAt: time.Now(), Payload: payload})
	if err != nil {
		return
	}

	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "entry-*.json")
	if err != nil {
		return
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(encoded); err != nil {
		temp.Close()
		return
	}
	if err := temp.Close(); err != nil {
		return
	}
	_ = os.Chmod(temp.Name(), 0o600)
	_ = os.Rename(temp.Name(), path)
}

// Remove deletes one entry, for example after an action changes an email.
func (c *Cache) Remove(key string) {
	if c.disabled {
		return
	}
	_ = os.Remove(c.path(key))
}

// Clear removes every cached body and reports how many entries were deleted.
func (c *Cache) Clear() (int, error) {
	if c.disabled || c.dir == "" {
		return 0, nil
	}
	removed := 0
	err := filepath.WalkDir(c.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			if os.Remove(path) == nil {
				removed++
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return removed, err
	}
	return removed, os.RemoveAll(c.dir)
}

// BodyKey is the cache key for one rendering of one email.
func BodyKey(emailID, view string) string {
	return emailID + "|" + view
}
