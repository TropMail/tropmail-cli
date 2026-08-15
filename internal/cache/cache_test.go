package cache

import (
	"testing"
	"time"
)

type payload struct {
	Subject string `json:"subject"`
}

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	t.Setenv("TROPMAIL_CACHE_DIR", t.TempDir())
	return New(true)
}

func TestPutGetRoundTrip(t *testing.T) {
	cache := newTestCache(t)
	cache.Put(BodyKey("id", "text"), payload{Subject: "hello"})

	var out payload
	if !cache.Get(BodyKey("id", "text"), &out) {
		t.Fatal("expected a cache hit")
	}
	if out.Subject != "hello" {
		t.Errorf("subject = %q", out.Subject)
	}
}

func TestViewsAreCachedSeparately(t *testing.T) {
	cache := newTestCache(t)
	cache.Put(BodyKey("id", "text"), payload{Subject: "plain"})

	var out payload
	if cache.Get(BodyKey("id", "markdown"), &out) {
		t.Error("the markdown view should not hit the text entry")
	}
}

func TestMissReturnsFalse(t *testing.T) {
	cache := newTestCache(t)

	var out payload
	if cache.Get(BodyKey("absent", "text"), &out) {
		t.Error("expected a miss")
	}
}

func TestRemove(t *testing.T) {
	cache := newTestCache(t)
	key := BodyKey("id", "text")
	cache.Put(key, payload{Subject: "hello"})
	cache.Remove(key)

	var out payload
	if cache.Get(key, &out) {
		t.Error("entry survived Remove")
	}
}

func TestExpiredEntryIsDropped(t *testing.T) {
	cache := newTestCache(t)
	cache.ttl = time.Nanosecond
	key := BodyKey("id", "text")
	cache.Put(key, payload{Subject: "hello"})

	time.Sleep(2 * time.Millisecond)

	var out payload
	if cache.Get(key, &out) {
		t.Error("expired entry was served")
	}
}

func TestClearRemovesEverything(t *testing.T) {
	cache := newTestCache(t)
	for _, id := range []string{"a", "b", "c"} {
		cache.Put(BodyKey(id, "text"), payload{Subject: id})
	}

	removed, err := cache.Clear()
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if removed != 3 {
		t.Errorf("removed = %d, want 3", removed)
	}

	var out payload
	if cache.Get(BodyKey("a", "text"), &out) {
		t.Error("entry survived Clear")
	}
}

func TestDisabledCacheIsInert(t *testing.T) {
	cache := New(false)
	cache.Put(BodyKey("id", "text"), payload{Subject: "hello"})

	var out payload
	if cache.Get(BodyKey("id", "text"), &out) {
		t.Error("a disabled cache must never hit")
	}
	if removed, err := cache.Clear(); err != nil || removed != 0 {
		t.Errorf("Clear = %d, %v", removed, err)
	}
}
