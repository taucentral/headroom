package headroom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	tau "github.com/taucentral/tau/pkg/tau"
)

func TestOriginalCache_PutThenGet(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)

	original := []byte(`{"big":"payload","tokens":99999}`)
	hash, err := cache.Put(context.Background(), original, ContentTypeJSON)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Hash matches the expected SHA-256.
	want := sha256.Sum256(original)
	if hash != hex.EncodeToString(want[:]) {
		t.Errorf("Put returned hash %q, want %q", hash, hex.EncodeToString(want[:]))
	}

	got, err := cache.Get(context.Background(), hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("Get returned %q, want %q", got, original)
	}

	// Verify the Store entry was populated per the spec.
	q := tau.Query{KeywordQuery: hash, Limit: 16}
	entries, err := store.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("store.Query: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.ID == hash {
			found = true
			if e.Source != "ccr" {
				t.Errorf("entry.Source = %q, want \"ccr\"", e.Source)
			}
			if !contains(e.Tags, "ccr") || !contains(e.Tags, "json") {
				t.Errorf("entry.Tags = %v, want [ccr json]", e.Tags)
			}
		}
	}
	if !found {
		t.Errorf("no Store entry with ID %q", hash)
	}
}

func TestOriginalCache_DuplicatePutIsNoOp(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)

	original := []byte("duplicate me")
	h1, err := cache.Put(context.Background(), original, ContentTypeText)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	h2, err := cache.Put(context.Background(), original, ContentTypeText)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}
	// Exactly one Store entry.
	q := tau.Query{KeywordQuery: h1, Limit: 32}
	entries, err := store.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.ID == h1 {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate Put wrote %d entries, want 1", count)
	}
}

func TestOriginalCache_ConcurrentSameHashIsNoOp(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)

	original := []byte("race-me")
	const N = 16
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		hash string
		errs []error
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := cache.Put(context.Background(), original, ContentTypeText)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if hash == "" {
				hash = h
			} else if hash != h {
				errs = append(errs, errors.New("hash mismatch across goroutines"))
			}
		}()
	}
	wg.Wait()
	for _, e := range errs {
		t.Errorf("concurrent Put error: %v", e)
	}
	if hash == "" {
		t.Fatal("no hash returned")
	}
	q := tau.Query{KeywordQuery: hash, Limit: 32}
	entries, err := store.Query(context.Background(), q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.ID == hash {
			count++
		}
	}
	if count != 1 {
		t.Errorf("concurrent Put of same hash wrote %d entries, want 1", count)
	}
}

func TestOriginalCache_GetMissingHash(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	cache := NewOriginalCache(store)
	_, err := cache.Get(context.Background(), "deadbeef")
	if !errors.Is(err, ErrOriginalNotFound) {
		t.Errorf("Get(missing): got err=%v, want ErrOriginalNotFound", err)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
