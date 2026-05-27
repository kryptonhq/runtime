/*
Copyright 2026 Krypton Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controlplane

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// Vite builds into ../../ui/dist. The Makefile builds the UI before the
// Go binary so this embed is non-empty. If the directory doesn't exist
// (someone running `go build` without `make ui` first), the embed will
// still succeed but UI() returns a placeholder handler.
//
//go:embed all:embed/dist
var uiAssets embed.FS

// UI returns a handler that serves the built React app at /ui/*. Any
// non-asset path inside /ui/ falls back to index.html so client-side
// routing works.
func UI() http.Handler {
	dist, err := fs.Sub(uiAssets, "embed/dist")
	if err != nil {
		return placeholderUI()
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return placeholderUI()
	}

	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/ui")
		path = strings.TrimPrefix(path, "/")

		// Try to serve a real asset first.
		if path != "" {
			if f, err := dist.Open(path); err == nil {
				_ = f.Close()
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + path
				fileServer.ServeHTTP(w, r2)
				return
			}
		}
		// SPA fallback. Write index.html directly so we don't trigger
		// the file server's directory-vs-file redirect logic.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func placeholderUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholderHTML))
	})
}

const placeholderHTML = `<!doctype html>
<html><head><title>Krypton UI not built</title>
<style>body{font-family:system-ui;background:#0f172a;color:#e2e8f0;padding:3rem;line-height:1.5}
code{background:#1e293b;padding:.2em .4em;border-radius:4px;font-family:ui-monospace,monospace}
</style></head><body>
<h1>Krypton UI not built</h1>
<p>The control plane binary was compiled without the UI assets. Build the UI first:</p>
<pre><code>make ui
make build</code></pre>
<p>Or in dev: <code>cd ui && pnpm dev</code></p>
</body></html>`
