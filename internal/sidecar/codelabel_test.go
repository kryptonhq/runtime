/*
Copyright 2026 Krypton Authors.
*/

package sidecar

import "testing"

// codeLabel buckets status codes so the sidecar's request metrics stay at
// five label values instead of one per code.
func TestCodeLabel(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{100, "1xx"},
		{101, "1xx"},
		{200, "2xx"},
		{204, "2xx"},
		{299, "2xx"},
		{301, "3xx"},
		{304, "3xx"},
		{400, "4xx"},
		{404, "4xx"},
		{429, "4xx"},
		{499, "4xx"},
		{500, "5xx"},
		{502, "5xx"},
		{503, "5xx"},
		{599, "5xx"},
		// Anything below 100 isn't a real HTTP status; it falls into the
		// 1xx bucket rather than producing an empty label.
		{0, "1xx"},
		{-1, "1xx"},
	}
	for _, tc := range tests {
		if got := codeLabel(tc.code); got != tc.want {
			t.Errorf("codeLabel(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}
