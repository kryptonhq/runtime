# Krypton Runtime — developer Makefile
# Run `make help` for a list of targets.

SHELL := /usr/bin/env bash

# ---- Versions ---------------------------------------------------------------
CONTROLLER_TOOLS_VERSION ?= v0.16.4
GOLANGCI_LINT_VERSION    ?= v2.5.0
KIND_VERSION             ?= v0.24.0
# v1.13.0 or newer: earlier releases pin golang.org/x/tools v0.19.0, which
# does not compile under Go 1.24+ (internal/tokeninternal negative array len).
GOTESTSUM_VERSION        ?= v1.13.0
KUBECONFORM_VERSION      ?= v0.6.7
CHAINSAW_VERSION         ?= v0.2.12
HELM_UNITTEST_VERSION    ?= 0.7.2

# envtest control-plane binaries (kube-apiserver + etcd). Keep the
# setup-envtest branch aligned with the controller-runtime minor in go.mod
# (v0.19.x -> release-0.19) or the asset index won't resolve.
ENVTEST_K8S_VERSION   ?= 1.31.0
SETUP_ENVTEST_VERSION ?= release-0.19

# ---- Paths ------------------------------------------------------------------
LOCALBIN := $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_GEN := $(LOCALBIN)/controller-gen
GOLANGCI_LINT  := $(LOCALBIN)/golangci-lint
SETUP_ENVTEST  := $(LOCALBIN)/setup-envtest
GOTESTSUM      := $(LOCALBIN)/gotestsum
KUBECONFORM    := $(LOCALBIN)/kubeconform
CHAINSAW       := $(LOCALBIN)/chainsaw

# Coverage + JUnit artifacts land here. One profile per tier so Codecov
# flags stay independent and carryforward works when a tier is skipped.
COVER_DIR := coverage
$(COVER_DIR):
	mkdir -p $(COVER_DIR)

# Only our own code counts toward coverage. cmd/** is process wiring with
# no meaningful assertions, examples/** ships as sample images, and
# zz_generated.deepcopy.go is controller-gen output — see hack/cover-filter.sh.
COVERPKG := ./api/...,./internal/...

# gotestsum emits JUnit XML (input for CI flaky-test tracking) and reads
# better locally. Plain `go test` is the fallback so a fresh clone works
# with no tools installed.
ifneq ($(wildcard $(LOCALBIN)/gotestsum),)
GO_TEST = $(GOTESTSUM) --format pkgname --junitfile $(COVER_DIR)/$(1).junit.xml --
else
GO_TEST = go test
endif

# ---- Build ------------------------------------------------------------------

.PHONY: build
build: ui ## Build all binaries (depends on a UI build).
	go build -o bin/manager ./cmd/manager
	go build -o bin/control-plane ./cmd/control-plane
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/krypton-proxy ./cmd/krypton-proxy

# ---- UI ---------------------------------------------------------------------

UI_DIR        := ui
UI_DIST       := $(UI_DIR)/dist
UI_EMBED_DIST := internal/controlplane/embed/dist

.PHONY: ui
ui: $(UI_EMBED_DIST)/index.html ## Build the React UI and stage it for go:embed.

# Build-time version surfaced in the UI sidebar footer. Falls back to
# the short git SHA so untagged dev builds still show something useful.
KRYPTON_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
KRYPTON_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

$(UI_EMBED_DIST)/index.html: $(shell find $(UI_DIR)/src $(UI_DIR)/public 2>/dev/null) $(UI_DIR)/package.json $(UI_DIR)/vite.config.ts
	cd $(UI_DIR) && pnpm install --frozen-lockfile && \
		VITE_KRYPTON_VERSION=$(KRYPTON_VERSION) \
		VITE_KRYPTON_COMMIT=$(KRYPTON_COMMIT) \
		pnpm build
	rm -rf $(UI_EMBED_DIST)
	mkdir -p $(UI_EMBED_DIST)
	cp -R $(UI_DIST)/. $(UI_EMBED_DIST)/
	touch $(UI_EMBED_DIST)/.gitkeep

.PHONY: ui-dev
ui-dev: ## Run the Vite dev server (proxies /v1 to localhost:8090).
	cd $(UI_DIR) && pnpm install && pnpm dev

.PHONY: ui-clean
ui-clean:
	rm -rf $(UI_DIST) $(UI_EMBED_DIST)
	mkdir -p $(UI_EMBED_DIST)
	touch $(UI_EMBED_DIST)/.gitkeep

# ---- Docs (Hugo + Docsy, deployed to Vercel) -------------------------------

WEBSITE_DIR := website

.PHONY: docs
docs: $(WEBSITE_DIR)/node_modules ## Build the documentation site to website/public/.
	cd $(WEBSITE_DIR) && hugo --minify

.PHONY: docs-serve
docs-serve: $(WEBSITE_DIR)/node_modules ## Serve docs locally with live reload (http://127.0.0.1:1313).
	cd $(WEBSITE_DIR) && hugo server

$(WEBSITE_DIR)/node_modules: $(WEBSITE_DIR)/package.json
	cd $(WEBSITE_DIR) && npm install
	touch $@

.PHONY: docs-clean
docs-clean:
	rm -rf $(WEBSITE_DIR)/public $(WEBSITE_DIR)/resources $(WEBSITE_DIR)/.hugo_build.lock

.PHONY: tidy
tidy: ## go mod tidy.
	go mod tidy

# ---- Test -------------------------------------------------------------------
#
# Four tiers, split by what infrastructure is real:
#
#   test-unit     pure functions + fake client. No binaries, no network.
#   test-envtest  real kube-apiserver + etcd via envtest. No kubelet, so
#                 pods never become Ready and owner refs never cascade.
#   test-store    real Postgres over a DSN.
#   test-e2e      real kind cluster, real images, real kubelet.
#
# `make test` stays the fast inner loop (unit only).

.PHONY: test
test: test-unit ## Run the fast unit tier (alias for test-unit).

.PHONY: test-unit
test-unit: $(COVER_DIR) ## Unit tests with race detector + coverage.
	$(call GO_TEST,unit) -race -count=1 -shuffle=on \
		-covermode=atomic -coverpkg=$(COVERPKG) \
		-coverprofile=$(COVER_DIR)/unit.out \
		./api/... ./internal/...
	@./hack/cover-filter.sh $(COVER_DIR)/unit.out
	@go tool cover -func=$(COVER_DIR)/unit.out | tail -1

.PHONY: test-envtest
test-envtest: $(COVER_DIR) envtest ## Integration tests against a real API server.
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		$(call GO_TEST,envtest) -tags envtest -race -count=1 \
			-covermode=atomic -coverpkg=$(COVERPKG) \
			-coverprofile=$(COVER_DIR)/envtest.out \
			-timeout 10m \
			./test/integration/...
	@./hack/cover-filter.sh $(COVER_DIR)/envtest.out
	@go tool cover -func=$(COVER_DIR)/envtest.out | tail -1

# Boots a throwaway Postgres in Docker, runs the store contract against it,
# then tears it down. Set KRYPTON_TEST_POSTGRES_DSN to point at your own.
.PHONY: test-store
test-store: $(COVER_DIR) ## Store integration tests against Postgres.
	@./hack/postgres-up.sh
	KRYPTON_TEST_POSTGRES_DSN="$${KRYPTON_TEST_POSTGRES_DSN:-postgres://krypton:krypton@127.0.0.1:5432/krypton?sslmode=disable}" \
		$(call GO_TEST,store) -tags integration -race -count=1 \
			-covermode=atomic -coverpkg=$(COVERPKG) \
			-coverprofile=$(COVER_DIR)/store.out \
			./internal/controlplane/store/...
	@./hack/cover-filter.sh $(COVER_DIR)/store.out
	@go tool cover -func=$(COVER_DIR)/store.out | tail -1

.PHONY: test-ui
test-ui: ## Frontend unit + component tests with coverage.
	cd $(UI_DIR) && pnpm install --frozen-lockfile && pnpm test:coverage

.PHONY: test-helm
test-helm: ## Lint, schema-validate, and unit-test the Helm chart.
	@./hack/helm-validate.sh

.PHONY: test-e2e
test-e2e: ## Full e2e against a kind cluster (builds + loads images first).
	@./hack/e2e.sh

.PHONY: test-all
test-all: test-unit test-envtest test-store test-helm test-ui ## Every tier except e2e.

.PHONY: cover
cover: ## Merge tier profiles and print the combined total.
	@./hack/cover-merge.sh

.PHONY: cover-html
cover-html: cover ## Merge profiles and open an HTML coverage report.
	go tool cover -html=$(COVER_DIR)/merged.out -o $(COVER_DIR)/index.html
	@echo "wrote $(COVER_DIR)/index.html"

.PHONY: test-clean
test-clean:
	rm -rf $(COVER_DIR)

# ---- Lint -------------------------------------------------------------------

.PHONY: vet
vet: ## go vet ./...
	go vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint.
	$(GOLANGCI_LINT) run

# Downloads the official release binary rather than `go install`ing from
# source. Two reasons: the v2 module path moved (…/v2/cmd/golangci-lint), and
# a source build inherits golangci-lint's own Go language version, which then
# refuses to analyse a newer target — `run.go: "1.25"` in .golangci.yml fails
# against a binary built with 1.24. The CI action uses the same prebuilt
# artifacts, so local and CI now agree.
$(GOLANGCI_LINT): | $(LOCALBIN)
	@set -euo pipefail; \
	version=$(patsubst v%,%,$(GOLANGCI_LINT_VERSION)); \
	os=$$(go env GOOS); arch=$$(go env GOARCH); \
	dir="golangci-lint-$${version}-$${os}-$${arch}"; \
	url="https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/$${dir}.tar.gz"; \
	echo ">> downloading $${url}"; \
	tmp=$$(mktemp -d); \
	curl -sSfL "$${url}" -o "$${tmp}/gcl.tar.gz"; \
	tar -xzf "$${tmp}/gcl.tar.gz" -C "$${tmp}"; \
	install -m 0755 "$${tmp}/$${dir}/golangci-lint" $(GOLANGCI_LINT); \
	rm -rf "$${tmp}"; \
	$(GOLANGCI_LINT) --version

# ---- Codegen ---------------------------------------------------------------

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Regenerate CRDs, RBAC, and webhook manifests.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook \
		paths="./api/..." paths="./internal/controller/..." \
		output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Regenerate deepcopy methods.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

$(CONTROLLER_GEN): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

# Fails when generated artifacts or the duplicated chart CRDs are stale.
# The chart ships its own copy of each CRD (Helm does not template
# crds/), so the two trees have to be kept byte-identical by hand.
.PHONY: verify-codegen
verify-codegen: ## Fail if generated files or chart CRD copies are stale.
	@./hack/verify-codegen.sh

# ---- Tools -----------------------------------------------------------------

.PHONY: envtest
envtest: $(SETUP_ENVTEST) ## Download envtest control-plane binaries.
	@$(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path >/dev/null

$(SETUP_ENVTEST): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

$(GOTESTSUM): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install gotest.tools/gotestsum@$(GOTESTSUM_VERSION)

$(KUBECONFORM): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/yannh/kubeconform/cmd/kubeconform@$(KUBECONFORM_VERSION)

$(CHAINSAW): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/kyverno/chainsaw@$(CHAINSAW_VERSION)

# helm-unittest is a Helm plugin, not a Go binary, so it installs into
# Helm's own plugin dir rather than bin/. `helm plugin install` errors when
# the plugin is already there, and --verify=false is mandatory on Helm 4
# because the upstream release publishes no provenance.
.PHONY: helm-unittest
helm-unittest: ## Install the helm-unittest plugin (no-op if present).
	@if helm plugin list 2>/dev/null | grep -qi unittest; then \
		echo ">> helm-unittest already installed"; \
	else \
		helm plugin install https://github.com/helm-unittest/helm-unittest --verify=false; \
	fi

.PHONY: tools
tools: $(GOLANGCI_LINT) $(CONTROLLER_GEN) $(SETUP_ENVTEST) $(GOTESTSUM) $(KUBECONFORM) $(CHAINSAW) helm-unittest ## Install every dev tool into bin/.
	@echo
	@echo "Tools installed into $(LOCALBIN). Run 'make test-all' to verify."

# ---- Run --------------------------------------------------------------------

.PHONY: run
run: manifests generate ## Run the manager locally against the current kubecontext.
	go run ./cmd/manager

# ---- kind dev cluster -------------------------------------------------------

.PHONY: kind-up
kind-up: ## Create a local kind cluster named "krypton-dev".
	kind create cluster --name krypton-dev --config hack/kind.yaml || true

.PHONY: kind-down
kind-down: ## Delete the local kind cluster.
	kind delete cluster --name krypton-dev

.PHONY: deploy-dev
deploy-dev: ## Full local loop: kind cluster, build, load, helm install, deploy echo agent.
	@./hack/local-up.sh

.PHONY: deploy-dev-llm
deploy-dev-llm: ## Full local loop plus the Qwen llama.cpp Model sample.
	@DEPLOY_LLM=true ./hack/local-up.sh

.PHONY: e2e-local
e2e-local: ## Run the smoke-test script against a port-forwarded gateway + control plane.
	@./hack/e2e-smoke.sh

.PHONY: docker-build
docker-build: ui ## Build all runtime images locally (TAG=dev by default).
	@for c in manager control-plane gateway krypton-proxy mcp-stdio-bridge; do \
		echo ">> building krypton/$$c"; \
		docker build --build-arg COMPONENT=$$c -t krypton/$$c:$${TAG:-dev} . ; \
	done
	@docker build -f examples/mcp/go/Dockerfile -t krypton/mcp-hello:$${TAG:-dev} .

# ---- Help -------------------------------------------------------------------

.PHONY: help
help: ## Show this help.
	@# The character class needs 0-9: without it `test-e2e` and `e2e-local`
	@# silently never appear in this listing.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
