package headroom

import (
	"context"
	"sync"

	tau "github.com/coevin/tau/pkg/tau"
)

// memStore is a minimal in-memory tau.Store for tests. It supports Put,
// Query (by keyword against Entry.Text and Entry.ID), and Close. It is
// safe for concurrent use. Queries do not support embedding similarity
// (EmbeddingQuery is ignored); TagsQuery and SinceQuery are honoured.
type memStore struct {
	mu      sync.RWMutex
	entries []tau.Entry
	closed  bool
}

func newMemStore() *memStore { return &memStore{} }

func (m *memStore) Put(_ context.Context, e tau.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return tau.ErrStoreClosed
	}
	// Content-addressed: duplicate IDs are a no-op.
	for _, existing := range m.entries {
		if existing.ID == e.ID {
			return nil
		}
	}
	m.entries = append(m.entries, e)
	return nil
}

func (m *memStore) Query(_ context.Context, q tau.Query) ([]tau.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, tau.ErrStoreClosed
	}
	var out []tau.Entry
	for _, e := range m.entries {
		if q.KeywordQuery != "" {
			if !containsFold(e.Text, q.KeywordQuery) && e.ID != q.KeywordQuery {
				continue
			}
		}
		if len(q.TagsQuery) > 0 {
			if !hasAllTags(e.Tags, q.TagsQuery) {
				continue
			}
		}
		if !q.SinceQuery.IsZero() && e.Timestamp.Before(q.SinceQuery) {
			continue
		}
		out = append(out, e)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (m *memStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func containsFold(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func hasAllTags(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
