package headroom

import (
	"testing"
)

func TestRouter_Classify(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	cases := []struct {
		name string
		in   string
		want ContentType
	}{
		{"json object", `{"k":"v"}`, ContentTypeJSON},
		{"json array", `[1,2,3]`, ContentTypeJSON},
		{"go source", "package foo\n\nfunc f() {}\n", ContentTypeGo},
		{
			"diff",
			"diff --git a/f b/f\nindex 0..1\n--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-a\n+b\n",
			ContentTypeDiff,
		},
		{"log", "2026-06-29T12:00:00Z INFO hello\n", ContentTypeLog},
		{"text prose", "just some prose without structure", ContentTypeText},
		{"empty", "", ContentTypeText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := r.Classify([]byte(tc.in))
			if got != tc.want {
				t.Errorf("Classify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
