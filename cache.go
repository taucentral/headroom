package headroom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	tau "github.com/coevin/tau/pkg/tau"
)

// OriginalCache writes the verbatim original of every compressed block
// to an embedder-supplied tau.Store, keyed by the SHA-256 of the bytes.
// The same cache is read by the Retrieve tool when the model asks for
// an original back.
//
// OriginalCache does NOT own the Store. The embedder constructs the
// Store and passes it to NewOriginalCache; the embedder (or the test
// harness) closes the Store when finished.
type OriginalCache struct {
	store tau.Store

	// seenMu guards seen. The cache is content-addressed: a second Put
	// for the same hash is a no-op for stores that do not tolerate
	// duplicate IDs. We track what we have already Put in-process so
	// we don't even call Store.Put for a duplicate within this session.
	seenMu sync.Mutex
	seen   map[string]struct{}
}

// NewOriginalCache wraps the given Store. The cache does not take
// ownership of the Store's lifecycle.
func NewOriginalCache(store tau.Store) *OriginalCache {
	return &OriginalCache{store: store, seen: make(map[string]struct{})}
}

// Put writes original to the Store under its SHA-256 hash and returns
// the hex-encoded hash. A second Put for byte-identical content is a
// no-op (the existing hash is returned without a second Store.Put).
//
// Tags are populated as ["ccr", string(contentType)] and Source is
// "ccr" so an embedder inspecting the Store can filter cache entries
// by tag or source.
func (c *OriginalCache) Put(ctx context.Context, original []byte, contentType ContentType) (string, error) {
	sum := sha256.Sum256(original)
	hash := hex.EncodeToString(sum[:])

	c.seenMu.Lock()
	if _, ok := c.seen[hash]; ok {
		c.seenMu.Unlock()
		return hash, nil
	}
	c.seen[hash] = struct{}{}
	c.seenMu.Unlock()

	entry := tau.Entry{
		ID:        hash,
		Text:      string(original),
		Tags:      []string{"ccr", string(contentType)},
		Source:    "ccr",
		Timestamp: time.Now().UTC(),
	}
	if err := c.store.Put(ctx, entry); err != nil {
		// Roll back the in-process dedupe so a retry after a transient
		// store error actually re-attempts the Put.
		c.seenMu.Lock()
		delete(c.seen, hash)
		c.seenMu.Unlock()
		return "", err
	}
	return hash, nil
}

// Get returns the original bytes for hash by querying the Store. Store
// implementations are free to index however they like; the cache
// queries by keyword (the hash) and filters results whose Entry.ID
// matches exactly.
func (c *OriginalCache) Get(ctx context.Context, hash string) ([]byte, error) {
	q := tau.Query{KeywordQuery: hash, Limit: 16}
	entries, err := c.store.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.ID == hash {
			return []byte(e.Text), nil
		}
	}
	return nil, ErrOriginalNotFound
}

// ErrOriginalNotFound is returned by OriginalCache.Get when no Entry in
// the Store has the requested ID.
var ErrOriginalNotFound = errors.New("headroom: original not found in cache")

// writeCorrection writes a correction string to the Store under the
// given symptom hash. Used by the LearnObserver to persist corrections
// keyed by the failed response's first-block SHA-256. The ID is
// namespaced as "correction:<symptom>" so it does not collide with
// original-bytes IDs (which are bare hex hashes).
//
// writeCorrection is best-effort; the caller (LearnObserver) ignores
// the returned error.
func (c *OriginalCache) writeCorrection(ctx context.Context, symptomHash, correction string) error {
	entry := tau.Entry{
		ID:        "correction:" + symptomHash,
		Text:      correction,
		Tags:      []string{"ccr", "learn", "correction"},
		Source:    "ccr-learn",
		Timestamp: time.Now().UTC(),
	}
	return c.store.Put(ctx, entry)
}
