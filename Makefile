# Krypton Runtime — developer Makefile
# Run `make help` for a list of targets.

SHELL := /usr/bin/env bash

# ---- Versions ---------------------------------------------------------------
CONTROLLER_TOOLS_VERSION ?= v0.16.4
GOLANGCI_LINT_VERSION    ?= v2.5.0
KIND_VERSION             ?= v0.24.0

# ---- Paths ------------------------------------------------------------------
LOCALBIN := $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_GEN := $(LOCALBIN)/controller-gen
GOLANGCI_LINT  := $(LOCALBIN)/golangci-lint

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

# ---- Test & lint ------------------------------------------------------------

.PHONY: test
test: ## Run unit tests.
	go test -race -count=1 ./...

.PHONY: vet
vet: ## go vet ./...
	go vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint.
	$(GOLANGCI_LINT) run

$(GOLANGCI_LINT): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

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
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
