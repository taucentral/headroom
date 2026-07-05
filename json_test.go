package headroom

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestJSONCompressor_RoundTrip(t *testing.T) {
	t.Parallel()
	c := NewJSONCompressor()
	in := []byte(`{"a":1,"b":2}`)
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	// Must be valid JSON with the same key set.
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v (output=%q)", err, out)
	}
	if len(got) != 2 || got["a"] == nil || got["b"] == nil {
		t.Fatalf("output key set changed: %v (output=%q)", got, out)
	}
}

func TestJSONCompressor_NestedPrune(t *testing.T) {
	t.Parallel()
	c := NewJSONCompressor("embedding", "token")
	in := []byte(`{"id":"x","embedding":[0.1,0.2],"nested":{"token":"secret","keep":true}}`)
	out, err := c.Compress(context.Background(), in)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if _, exists := got["embedding"]; exists {
		t.Errorf("top-level embedding not pruned: %s", out)
	}
	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested not a map: %v", got["nested"])
	}
	if _, exists := nested["token"]; exists {
		t.Errorf("nested token not pruned: %s", out)
	}
	if _, exists := nested["keep"]; !exists {
		t.Errorf("nested keep pruned: %s", out)
	}
}

func TestJSONCompressor_RejectsNonJSON(t *testing.T) {
	t.Parallel()
	c := NewJSONCompressor()
	cases := [][]byte{
		[]byte(`not json`),
		[]byte(``),
		[]byte(`{unquoted: 1}`),
		[]byte(`{"a":}`),
	}
	for _, in := range cases {
		_, err := c.Compress(context.Background(), in)
		if !errors.Is(err, ErrNotJSON) {
			t.Errorf("Compress(%q): got err=%v, want ErrNotJSON", in, err)
		}
	}
}
