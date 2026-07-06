package headroom

import (
	"context"
	"errors"
	"strings"
	"testing"

	tau "github.com/taucentral/tau/pkg/tau"
)

// newRegistryWithDefaults registers the JSON, Go, log, and diff
// compressors so tests for the CCR mutator exercise real dispatch.
// The TextCompressor is intentionally NOT registered here so prose
// falls through as pass-through.
func newRegistryWithDefaults(t *testing.T) *Registry {
	t.Helper()
	reg := NewRegistry()
	for _, c := range []Compressor{
		NewJSONCompressor(),
		NewGoASTCompressor(),
		NewLogCompressor(),
		NewDiffCompressor(),
	} {
		if err := reg.Register(c); err != nil {
			t.Fatalf("Register(%T): %v", c, err)
		}
	}
	return reg
}

func TestCCRRequestMutator_HintAppended(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	reg := newRegistryWithDefaults(t)
	mut := NewCCRRequestMutator(reg, NewRouter(), NewOriginalCache(store))

	req := &tau.Request{}
	if err := mut.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	if len(req.System) != 1 {
		t.Fatalf("System = %d blocks, want 1", len(req.System))
	}
	tc, ok := req.System[0].(tau.TextContent)
	if !ok {
		t.Fatalf("System[0] = %T, want TextContent", req.System[0])
	}
	if !strings.Contains(tc.Text, "ccr_retrieve") {
		t.Errorf("hint missing: %q", tc.Text)
	}
}

func TestCCRRequestMutator_JSONBlockCompressedAndCached(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	reg := newRegistryWithDefaults(t)
	mut := NewCCRRequestMutator(reg, NewRouter(), NewOriginalCache(store))

	original := `{"name":"alex","age":42,"long":"` + strings.Repeat("x", 500) + `"}`
	req := &tau.Request{
		Messages: []tau.Message{
			{Role: tau.Role("user"), Content: []tau.ContentBlock{tau.TextContent{Text: original}}},
		},
	}
	if err := mut.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	tc, ok := req.Messages[0].Content[0].(tau.TextContent)
	if !ok {
		t.Fatalf("block type %T, want TextContent", req.Messages[0].Content[0])
	}
	if !strings.Contains(tc.Text, "compressed via ccr") {
		t.Errorf("block not compressed: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "retrieve original with hash") {
		t.Errorf("retrieval marker missing: %q", tc.Text)
	}
	hash := extractHash(t, tc.Text)
	got, err := NewOriginalCache(store).Get(context.Background(), hash)
	if err != nil {
		t.Fatalf("cache.Get: %v", err)
	}
	if string(got) != original {
		t.Errorf("cache returned %q, want %q", got, original)
	}
}

// alwaysFailCompressor always returns an error to exercise the
// compressor-failure pass-through path.
type alwaysFailCompressor struct{}

func (alwaysFailCompressor) ContentTypes() []ContentType { return []ContentType{ContentTypeJSON} }
func (alwaysFailCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	return nil, errors.New("synthetic compressor failure")
}

// panicCompressor always panics to exercise the panic-recovery path.
type panicCompressor struct{}

func (panicCompressor) ContentTypes() []ContentType { return []ContentType{ContentTypeJSON} }
func (panicCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	panic("synthetic compressor panic")
}

func TestCCRRequestMutator_PassThroughOnCompressorError(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	reg := NewRegistry()
	if err := reg.Register(alwaysFailCompressor{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mut := NewCCRRequestMutator(reg, NewRouter(), NewOriginalCache(store))
	original := `{"x":1}`
	req := &tau.Request{
		Messages: []tau.Message{
			{Role: tau.Role("user"), Content: []tau.ContentBlock{tau.TextContent{Text: original}}},
		},
	}
	if err := mut.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	got, ok := req.Messages[0].Content[0].(tau.TextContent)
	if !ok {
		t.Fatalf("block type %T", req.Messages[0].Content[0])
	}
	if got.Text != original {
		t.Errorf("compressor error altered text: got %q, want %q", got.Text, original)
	}
}

func TestCCRRequestMutator_PassThroughOnStoreError(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	// Close immediately so all subsequent Put calls fail with
	// ErrStoreClosed.
	_ = store.Close()
	reg := NewRegistry()
	if err := reg.Register(NewJSONCompressor()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mut := NewCCRRequestMutator(reg, NewRouter(), NewOriginalCache(store))
	original := `{"x":1}`
	req := &tau.Request{
		Messages: []tau.Message{
			{Role: tau.Role("user"), Content: []tau.ContentBlock{tau.TextContent{Text: original}}},
		},
	}
	if err := mut.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	got, ok := req.Messages[0].Content[0].(tau.TextContent)
	if !ok {
		t.Fatalf("block type %T", req.Messages[0].Content[0])
	}
	if got.Text != original {
		t.Errorf("store error altered text: got %q, want %q", got.Text, original)
	}
}

func TestCCRRequestMutator_PanicRecovery(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	reg := NewRegistry()
	if err := reg.Register(panicCompressor{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mut := NewCCRRequestMutator(reg, NewRouter(), NewOriginalCache(store))
	original := `{"x":1}`
	req := &tau.Request{
		Messages: []tau.Message{
			{Role: tau.Role("user"), Content: []tau.ContentBlock{tau.TextContent{Text: original}}},
		},
	}
	if err := mut.MutateRequest(context.Background(), req); err != nil {
		t.Fatalf("MutateRequest: %v", err)
	}
	got, ok := req.Messages[0].Content[0].(tau.TextContent)
	if !ok {
		t.Fatalf("block type %T", req.Messages[0].Content[0])
	}
	if got.Text != original {
		t.Errorf("panic did not fall through to pass-through: got %q", got.Text)
	}
}

// extractHash pulls the hex token after "hash " from a CCR marker.
func extractHash(t *testing.T, marker string) string {
	t.Helper()
	idx := strings.Index(marker, "hash ")
	if idx < 0 {
		t.Fatalf("no hash marker in %q", marker)
	}
	rest := marker[idx+len("hash "):]
	end := strings.IndexAny(rest, "\n>")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end])
}
