package headroom

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeCompressor is a minimal Compressor for registry tests. It claims
// the given types and returns its input unchanged.
type fakeCompressor struct{ types []ContentType }

func (f *fakeCompressor) ContentTypes() []ContentType                    { return f.types }
func (f *fakeCompressor) Compress(ctx context.Context, in []byte) ([]byte, error) {
	return in, nil
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	c := &fakeCompressor{types: []ContentType{ContentTypeJSON}}
	if err := reg.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := reg.Lookup(ContentTypeJSON)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != c {
		t.Fatalf("Lookup returned %p, want %p", got, c)
	}
}

func TestRegistry_Duplicate(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := reg.Register(&fakeCompressor{types: []ContentType{ContentTypeJSON}}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := reg.Register(&fakeCompressor{types: []ContentType{ContentTypeJSON}})
	if !errors.Is(err, ErrCompressorAlreadyRegistered) {
		t.Fatalf("got err=%v, want ErrCompressorAlreadyRegistered", err)
	}
	// First registration must still be the one returned.
	got, err := reg.Lookup(ContentTypeJSON)
	if err != nil {
		t.Fatalf("Lookup after duplicate: %v", err)
	}
	if _, ok := got.(*fakeCompressor); !ok {
		t.Fatalf("Lookup returned %T after duplicate registration", got)
	}
}

func TestRegistry_Unknown(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	_, err := reg.Lookup(ContentTypeLog)
	if !errors.Is(err, ErrUnknownCompressor) {
		t.Fatalf("got err=%v, want ErrUnknownCompressor", err)
	}
}

func TestRegistry_ConcurrentRegisterUnderRace(t *testing.T) {
	// Race detector: many goroutines registering compressors for
	// distinct types; all must succeed and all lookups must resolve.
	t.Parallel()
	reg := NewRegistry()
	allTypes := []ContentType{
		ContentTypeJSON, ContentTypeGo, ContentTypeLog,
		ContentTypeDiff, ContentTypeText,
	}
	var wg sync.WaitGroup
	for _, ty := range allTypes {
		wg.Add(1)
		go func(ty ContentType) {
			defer wg.Done()
			if err := reg.Register(&fakeCompressor{types: []ContentType{ty}}); err != nil {
				t.Errorf("concurrent Register(%s): %v", ty, err)
			}
		}(ty)
	}
	wg.Wait()
	for _, ty := range allTypes {
		if _, err := reg.Lookup(ty); err != nil {
			t.Errorf("Lookup(%s) after concurrent Register: %v", ty, err)
		}
	}
}
