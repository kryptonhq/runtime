# Testing Krypton Runtime

Four tiers, split by **what infrastructure is real**. Pick the cheapest tier
that can actually catch the bug you care about.

| Tier | Command | What's real | Time |
| ---- | ------- | ----------- | ---- |
| Unit | `make test-unit` | Nothing. Pure functions + `client/fake`. | ~15s |
| Integration | `make test-envtest` | A real kube-apiserver and etcd (no kubelet). | ~30s |
| Store | `make test-store` | A real Postgres in Docker. | ~10s |
| Chart | `make test-helm` | Nothing; renders and schema-validates templates. | ~5s |
| Frontend | `make test-ui` | jsdom + MSW-intercepted HTTP. | ~5s |
| e2e | `make test-e2e` | A real kind cluster: kubelet, images, network. | ~10m |

`make test-all` runs everything except e2e. `make cover` merges the per-tier
profiles and prints the combined total.

---

## Which tier does my test belong in?

**Unit** — the default. Anything that's a pure function of its inputs:
scaling decisions, path parsing, sort comparators, status-equality checks.
The fake client is fine for "given these objects, does Reconcile produce
that object".

**Integration (envtest)** — anything that depends on **API-server
semantics**. The fake client does no schema validation, no defaulting, no
pruning, and no status-subresource enforcement, so it will happily stay green
while production breaks. Use this tier for:

- CRD schema round-trips (see below — this is the big one)
- defaulting and validation
- finalizers and deletion actually completing
- optimistic-concurrency conflicts
- `SetupWithManager` and watch wiring

**e2e** — anything that needs a kubelet or real network: pod readiness,
cold starts, latency, streaming, image pull behaviour, the Helm chart
actually installing.

---

## envtest: what it can and cannot do

envtest boots a real `kube-apiserver` and `etcd`, then installs the CRDs from
`config/crd/bases`. It does **not** run a kubelet or any built-in
controllers. That has hard consequences for what you may assert:

| Don't assert | Because | Assert instead |
| ------------ | ------- | -------------- |
| A Pod becomes Ready | No kubelet; pods are never scheduled | The Deployment was created with the right spec |
| A Deployment reports `readyReplicas` | No deployment controller | `spec.replicas` is what you expect |
| A child is deleted with its parent | No garbage collector | The `ownerReference` is *set* |
| `Phase == Ready` | Depends on ready pods | `Phase` is `Pending`, or just that it's non-empty |

Tests that violate these will hang until timeout and look flaky.

### The CRD round-trip test

`config/crd/bases/*.yaml` are **hand-written** — note the
`controller-gen.kubebuilder.io/version: hand-written-bootstrap` annotation.
Nothing regenerates them from the Go types, so a field added to `AgentSpec`
with no matching property in the CRD gets **silently pruned** by the API
server. No error, no warning: the data is just gone.

`TestAgentSpecSurvivesRoundTrip` and `TestModelSpecSurvivesRoundTrip` in
`test/integration/crd_schema_test.go` populate every field, write through a
real API server, and read back. **If you add a field to a spec type, add it
there too.** `hack/verify-codegen.sh` also compares the Go json tags against
the CRD's `spec.properties` as a cheap static backstop.

---

## Known API quirks the tests pin

Two of these are documented as tests rather than fixed, because fixing them
is an API change:

1. **`minReplicas: 0` is unreachable from a typed Go client.** The field is
   `int32` with `omitempty` *and* `+kubebuilder:default=1`. omitempty elides
   the zero, the API server sees an absent field, and applies the default.
   `kubectl apply` with an explicit `0` works fine.
   See `TestTypedClientCannotSetZeroMinReplicas`.

2. **Duration defaults don't apply from a typed Go client.**
   `scaleToZeroAfter`, `timeout` and `startupTimeout` are `metav1.Duration`,
   a struct — and `omitempty` does nothing for structs. A typed client always
   sends `"0s"`, which the API server treats as an explicit value, so the
   300s/60s/30s defaults never fire. YAML is unaffected.
   See `TestDurationDefaultsAreDefeatedByTypedClient`.

3. **The `always-on` + `minReplicas: 0` contradiction isn't rejected.** That
   rule lives only in the validating webhook, and the chart ships
   `manager.enableWebhooks: false` with no `ValidatingWebhookConfiguration`.
   OpenAPI can't express a cross-field constraint.
   See `TestAlwaysOnWithZeroMinReplicasIsNotRejectedBySchema`.

If any of these get fixed, the corresponding test fails loudly and tells you
to invert it.

---

## Coverage

Per-tier profiles land in `coverage/`, one file per tier so Codecov flags
stay independent. `hack/cover-filter.sh` strips code that shouldn't count:

- `zz_generated.deepcopy.go` — measures controller-gen, not this project
- `cmd/**` — process wiring; covered by e2e starting the binaries
- `examples/**`, `internal/controlplane/embed/**`

Without that filter `api/v1alpha1` reads as ~25%; the real figure for
hand-written code there is ~98%.

Targets are set in `codecov.yml`: 80% project for Go, 70% for the UI, and
**80% on the patch** — new and changed lines are held to a higher bar than
the codebase average, which is what actually moves the number.

---

## e2e

Two layers, because they're good at different things:

- **Chainsaw** (`test/e2e/chainsaw/`) — declarative YAML. "Apply this CR,
  assert these objects exist with these fields." Replaces hundreds of lines
  of Go for reconcile assertions.
- **Go** (`test/e2e/`) — behavioural assertions YAML expresses badly:
  latency budgets, protocol round-trips, streaming, the OpenAI-compatible
  routes.

```bash
make test-e2e                      # build, load into kind, run both layers
KEEP_CLUSTER=true make test-e2e    # leave the cluster up to debug
SKIP_BUILD=true make test-e2e      # reuse already-loaded images
DEPLOY_LLM=true make test-e2e      # include the llama.cpp/Qwen path (slow)
```

On failure, `hack/e2e-diagnostics.sh` dumps pod logs (including previous
containers), events, CR state and Helm values to
`/tmp/krypton-e2e-diagnostics`; CI uploads it as an artifact.

The LLM path is **nightly-only** — pulling llama.cpp plus model weights on
every PR would make the queue unusable. It runs in `e2e-nightly.yml`
alongside a Kubernetes version matrix (1.30–1.33, matching the
`kubeVersion: ">=1.30.0-0"` the chart claims).

---

## CI

`ci.yml` runs the tiers as parallel jobs and aggregates into one required
status check named `ci` — so branch protection needs one entry, not eight.

```
lint · drift · unit · envtest · store · helm · ui · e2e  →  ci
```

`drift` is worth calling out: it runs `make verify-codegen`, which fails if
`zz_generated.deepcopy.go` is stale or if the chart's CRD copies
(`deploy/helm/krypton/crds/`) have drifted from `config/crd/bases/`. Helm
does not template `crds/`, so those are two hand-maintained copies of the
same file.

---

## Adding a test

- New CRD field → add it to the envtest round-trip test. Non-negotiable;
  it's the only thing standing between a hand-written CRD and silent data
  loss.
- New chart value that changes rendering → add a case to
  `deploy/helm/krypton/tests/` and to the values matrix in
  `hack/helm-validate.sh`.
- New control-plane route → a unit test with `httptest`, plus an e2e
  assertion if it's user-facing.
- New UI component → an RTL test querying by role/label, with MSW handlers
  derived from the real Go response shapes in `ui/src/test/fixtures.ts`.
