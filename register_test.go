package headroom

import (
	"testing"

	tau "github.com/coevin/tau/pkg/tau"
)

func TestMiddleware_EmptyOptsReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	got := Middleware(RegisterOptions{}, store)
	if len(got) != 0 {
		t.Errorf("Middleware({}) = %v, want empty slice", got)
	}
}

func TestMiddleware_CCROnlyReturnsOneRequestMutator(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	got := Middleware(RegisterOptions{CCR: true}, store)
	if len(got) != 1 {
		t.Fatalf("Middleware(CCR:true) = %d elements, want 1", len(got))
	}
	if _, ok := got[0].(tau.RequestMutator); !ok {
		t.Errorf("element %T does not satisfy tau.RequestMutator", got[0])
	}
}

func TestMiddleware_AllOptsReturnsFourComponents(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	got := Middleware(RegisterOptions{
		CCR:             true,
		PrefixStabilizer: true,
		OutputObserver:  true,
		LearnObserver:   true,
	}, store)
	if len(got) != 4 {
		t.Fatalf("Middleware(all) = %d elements, want 4", len(got))
	}
	var (
		mutators  int
		observers int
	)
	for _, v := range got {
		if _, ok := v.(tau.RequestMutator); ok {
			mutators++
		}
		if _, ok := v.(tau.ResponseObserver); ok {
			observers++
		}
	}
	if mutators != 2 {
		t.Errorf("got %d RequestMutators, want 2 (CCR + PrefixStabilizer)", mutators)
	}
	if observers != 2 {
		t.Errorf("got %d ResponseObservers, want 2 (Output + Learn)", observers)
	}
}

func TestMiddleware_EachCallReturnsFreshInstances(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	a := Middleware(RegisterOptions{CCR: true}, store)
	b := Middleware(RegisterOptions{CCR: true}, store)
	if a[0] == b[0] {
		t.Errorf("Middleware returned the same pointer across calls")
	}
}

func TestTools_ReturnsRetrieveTool(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	t.Cleanup(func() { _ = store.Close() })
	tools := Tools(store)
	if len(tools) != 1 {
		t.Fatalf("Tools = %d, want 1", len(tools))
	}
	if tools[0].Name() != "ccr_retrieve" {
		t.Errorf("Tools[0].Name() = %q, want \"ccr_retrieve\"", tools[0].Name())
	}
}
