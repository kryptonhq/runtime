//go:build envtest

/*
Copyright 2026 Krypton Authors.
*/

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Every manifest under config/samples/ is applied to a real API server.
// These files are referenced verbatim from the README and the docs site
// (`kubectl apply -f .../config/samples/llm/qwen2.5-0.5b.yaml`), so a sample
// that drifts from the CRD schema is a broken quickstart — the first thing a
// new user hits.
//
// This replaces schema-validating the samples with kubeconform, which would
// need the CRDs converted to JSON Schema and still wouldn't catch defaulting
// or enum violations the way the API server does.
func TestSamplesAreAccepted(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join("..", "..", "config", "samples")

	var manifests []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext == ".yaml" || ext == ".yml" {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(manifests) == 0 {
		t.Fatalf("no sample manifests found under %s — did the directory move?", root)
	}

	ns := newNamespace(t)

	for _, path := range manifests {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			// Samples may hold multiple documents.
			for i, doc := range splitYAMLDocs(string(raw)) {
				if strings.TrimSpace(doc) == "" {
					continue
				}
				obj := &unstructured.Unstructured{}
				if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
					t.Fatalf("doc %d: parse YAML: %v", i, err)
				}
				if obj.GetKind() == "" {
					continue // e.g. a lone comment block
				}
				// Namespace-scoped samples ship with their own namespace
				// (often one that doesn't exist here); redirect into the
				// test namespace so we're testing the schema, not RBAC.
				if obj.GetKind() != "Namespace" {
					obj.SetNamespace(ns)
				}

				if err := k8sClient.Create(ctx, obj); err != nil {
					t.Errorf("doc %d (%s/%s) was rejected by the API server: %v\n"+
						"this sample is referenced from the README/docs, so it must apply cleanly",
						i, obj.GetKind(), obj.GetName(), err)
				}
			}
		})
	}
}

// splitYAMLDocs splits on document separators at the start of a line. Good
// enough for hand-written sample manifests.
func splitYAMLDocs(s string) []string {
	parts := strings.Split(s, "\n---")
	for i := range parts {
		parts[i] = strings.TrimPrefix(parts[i], "---")
	}
	return parts
}
