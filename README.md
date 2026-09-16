# OpenShift Pipelines Release Tests

End-to-end test suite for [OpenShift Pipelines](https://docs.openshift.com/pipelines/latest/about/about-pipelines.html), written with [Ginkgo v2](https://onsi.github.io/ginkgo/).

## Prerequisites

- OpenShift 4.x cluster with cluster-admin access
- Go 1.24+
- [`ginkgo`](https://onsi.github.io/ginkgo/#installing-ginkgo) CLI — `go install github.com/onsi/ginkgo/v2/ginkgo@latest`
- `oc` CLI authenticated to the cluster

> **Note:** These tests support two scenarios:
> - **Fresh cluster** — operator not yet installed: run the `install` suite first to install it via OLM (see [Operator Installation](#operator-installation)).
> - **Pre-installed operator** — `TektonConfig` CR already in a ready state: skip the `olm` suite and run feature tests directly (see [Running Tests on a Pre-installed Cluster](#running-tests-on-a-pre-installed-cluster)).

## Configuration

Component versions and cluster settings are read from `env/default/default.properties`.  
Update this file for each release branch before running tests.

Key environment variables:

| Variable | Description |
|----------|-------------|
| `KUBECONFIG` | Optional kubeconfig file or platform-separated list; defaults to `~/.kube/config` |
| `PIPELINE_VERSION` | Expected Pipelines version |
| `TRIGGERS_VERSION` | Expected Triggers version |
| `OPERATOR_VERSION` | Expected Operator version |
| `CHAINS_VERSION` | Expected Chains version |
| `PAC_VERSION` | Expected PAC version |
| `GITLAB_TOKEN` | GitLab API token *(PAC tests)* |
| `GITHUB_TOKEN` | GitHub token *(resolver tests)* |
| `KO_DOCKER_REPO` | Registry for built test images |
| `CHAINS_REPOSITORY` | OCI repo to push Kaniko-built images to *(Chains TC02)* |
| `CHAINS_DOCKER_CONFIG_JSON` | Raw JSON contents of `docker/config.json` with push access to `CHAINS_REPOSITORY` *(Chains TC02)* |
| `KUEUE_NAMESPACE` | Kueue operator namespace; defaults to `openshift-kueue-operator` |
| `PIPELINE_NAMESPACE` | Pipelines operator namespace; defaults to `openshift-operators` |
| `CERTMANAGER_NAMESPACE` | cert-manager operator namespace; defaults to `cert-manager-operator` |
| `CHANNEL_KUEUE` | Kueue operator channel; defaults to `stable-v1.4` |
| `CHANNEL_PIPELINE` | Pipelines operator channel; defaults to `latest` |
| `CHANNEL_CERTMANAGER` | cert-manager operator channel; defaults to `stable-v1` |

OLM subscription defaults (in `env/default/default.properties`):

| Property | Default Value |
|----------|---------------|
| `SUBSCRIPTION_NAME` | `openshift-pipelines-operator-rh` |
| `CHANNEL` | `latest` |
| `CATALOG_SOURCE` | `redhat-operators` |

## Operator Installation

The operator is **not** assumed to be pre-installed. The `olm` test suite (`tests/olm/`) is the install step and must be run first on a fresh cluster.

### How it works

The install test (`Install openshift-pipelines operator`) is an `Ordered` Ginkgo block that runs the following steps sequentially:

1. **Create OLM Subscription** — renders `template/subscription.yaml.tmp` with values from `config.Flags` and applies it via `oc apply`.
2. **Wait for CSV** — polls until `Subscription.Status.InstalledCSV` is set and the `ClusterServiceVersion` phase reaches `Succeeded` (timeout: 8 minutes).
3. **Wait for TektonConfig CR** — ensures the `TektonConfig` CR named `config` exists and is ready.
4. **Wait for webhook deployments** — explicitly waits for `tekton-operator-proxy-webhook` and `tekton-pipelines-webhook` (both in `openshift-pipelines`) to have `AvailableReplicas ≥ 1`. This prevents a race condition on fresh CI clusters where the webhook pods are still starting when the first `oc patch TektonConfig` is attempted.
5. **Post-install configuration** — a series of ordered steps:
   - Define `artifact-hub-api` variable
   - Apply `testdata/hub/tektonhub.yaml`
   - Configure GitHub token for the git resolver
   - Configure the bundles resolver
   - Enable console plugin
   - Enable StatefulSet mode for Pipelines, Chains, and Results
   - Enable Chains signing secret generation
   - Configure Results with Loki and create the Results route
   - Apply Manual Approval Gate (`testdata/manualapprovalgate/manual-approval-gate.yaml`)
   - Validate all component deployments (Triggers, PAC, Hub, MAG, StatefulSets, CLI)
   - Validate TektonInstallerSets, RBAC, quickstarts, and auto-prune cronjob

### Subscription template (`template/subscription.yaml.tmp`)

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: {{.SubscriptionName}}
  namespace: openshift-operators
spec:
  channel: {{.Channel}}
  installPlanApproval: Automatic
  name: {{.SubscriptionName}}
  source: {{.CatalogSource}}
  sourceNamespace: openshift-marketplace
```

### Running the install, upgrade, and uninstall

```bash
# Install the operator on a fresh cluster
./scripts/run-tests.sh install
# or equivalently:
ginkgo run --label-filter=install --timeout=30m ./tests/olm/
```

To upgrade the operator, set `UPGRADE_CHANNEL` and run with the `upgrade` label:

```bash
UPGRADE_CHANNEL=pipelines-1.18 ./scripts/run-tests.sh upgrade
# or equivalently:
UPGRADE_CHANNEL=pipelines-1.18 ginkgo run --label-filter=upgrade --timeout=30m ./tests/olm/
```

To uninstall:

```bash
./scripts/run-tests.sh uninstall
# or equivalently:
ginkgo run --label-filter=uninstall --timeout=30m ./tests/olm/
```

## Running Tests on a Pre-installed Cluster

If the OpenShift Pipelines Operator is already installed and the `TektonConfig` CR is in a ready state, skip the `olm` suite entirely and run the feature tests directly. Each suite creates and cleans up its own isolated namespaces — there is no shared state with the install step.

### Quick start — sanity suite
```bash
export KUBECONFIG=/path/to/kubeconfig
./scripts/run-tests.sh sanity
```

### Run a specific suite
```bash
ginkgo run --timeout=30m ./tests/pipelines/
ginkgo run --timeout=30m ./tests/triggers/
ginkgo run --timeout=30m ./tests/operator/
```

### Run all feature tests (exclude install/upgrade/uninstall)
```bash
ginkgo run --label-filter='!install && !upgrade && !uninstall' --timeout=60m ./tests/...
```

## Running Tests on a Fresh Cluster

Run the `olm` suite first to install the operator via OLM, then run the feature suites.

```bash
# Step 1 — Install the operator
export KUBECONFIG=/path/to/kubeconfig
ginkgo run --label-filter=install --timeout=30m ./tests/olm/

# Step 2 — Run feature tests
ginkgo run --label-filter='e2e && !disconnected' --timeout=60m ./tests/...
```

## Running Tests (general options)

### Select a kubeconfig or context

`KUBECONFIG` uses the standard client-go loading rules, including merged path lists. Test flags must follow `--` so Ginkgo passes them to the test binary.

```bash
# Standard environment loading, including multiple files on Linux and macOS
export KUBECONFIG=/path/to/hub:/path/to/additional-config
./scripts/run-tests.sh sanity

# Explicit file and context with the wrapper
./scripts/run-tests.sh sanity -- --kubeconfig=/path/to/config --context=my-context

# Equivalent direct Ginkgo invocation
ginkgo run ./tests/pipelines/ -- --kubeconfig=/path/to/config --context=my-context
```

### Run with a label filter
```bash
# All e2e, skip disconnected
ginkgo run --label-filter='e2e && !disconnected' --timeout=30m ./tests/...

# Admin-only tests
ginkgo run --label-filter=admin --timeout=30m ./tests/...
```

### Run in parallel
```bash
ginkgo run -p --timeout=30m ./tests/...
```

### Generate a JUnit report
```bash
ginkgo run --label-filter=sanity --timeout=30m --junit-report=report.xml ./tests/...
```

### `run-tests.sh` modes

| Mode | Label filter | Test path |
|------|-------------|-----------|
| `install` | `install` | `tests/olm/` |
| `upgrade` | `upgrade` | `tests/olm/` |
| `uninstall` | `uninstall` | `tests/olm/` |
| `sanity` | `sanity` | `tests/...` |
| `smoke` | `smoke` | `tests/...` |
| `e2e` | `e2e` | `tests/...` |
| `e2e-connected` | `e2e && !disconnected` | `tests/...` |
| `disconnected` | `disconnected` | `tests/...` |
| `all` | *(none)* | `tests/...` |
| `<any string>` | used directly as `--label-filter` | `tests/...` |

```bash
./scripts/run-tests.sh e2e
LABEL_FILTER='triggers && sanity' ./scripts/run-tests.sh
```

## Test Areas

| Area | Labels | Test Suite |
|------|--------|------------|
| OLM / Install | `install`, `upgrade`, `uninstall` | `tests/olm/` |
| Pipelines Core | `pipelines`, `non-admin` | `tests/pipelines/` |
| Operator | `operator`, `admin` | `tests/operator/` |
| Triggers | `triggers`, `non-admin` | `tests/triggers/` |
| Ecosystem Tasks | `ecosystem`, `buildah`, `s2i`, `git-clone` | `tests/ecosystem/` |
| PAC | `pac` | `tests/pac/` |
| Chains | `chains` | `tests/chains/` |
| Results | `results` | `tests/results/` |
| Manual Approval Gate | `approvalgate` | `tests/mag/` |
| Hub | `hub` | `tests/hub/` |
| Metrics | `metrics` | `tests/metrics/` |
| Versions | `versions` | `tests/versions/` |
| Tekton Kueue | `tekton-kueue`, `install`, `workload`, `admin` | `tests/tektonkueue/` |

### Running the Tekton Kueue suite

Provide one hub kubeconfig and at least one spoke kubeconfig. Test-binary flags must follow `--`.

Bootstrap the required operators:

```bash
ginkgo run --procs=1 --label-filter='tekton-kueue && install' ./tests/tektonkueue -- \
  --kubeconfig /path/to/hub \
  --spoke-kubeconfig /path/to/spoke-1
```

Validate multi-cluster PipelineRun execution:

```bash
ginkgo run --procs=1 --grace-period=25m --label-filter='tekton-kueue && workload' ./tests/tektonkueue -- \
  --kubeconfig /path/to/hub \
  --spoke-kubeconfig /path/to/spoke-1 \
  --spoke-kubeconfig /path/to/spoke-2
```

Repeat `--spoke-kubeconfig` for additional spokes and optionally provide one matching `--spoke-context` per spoke. The workload test makes every configured spoke eligible, creates a PipelineRun on the hub, verifies execution on exactly one spoke, and verifies that terminal status returns to the hub.

The workload test temporarily configures and restores `TektonConfig.spec.scheduler`, Kueue, scoped worker credentials, RBAC, queues, and test-owned network access. Test namespaces and resources are removed even after failure. OLM operator installations remain available, matching the install suite. Log-content validation, secret synchronization, CEL routing, and NetworkPolicy behavior are separate coverage.

### Running the Chains suite

The Chains suite has two test cases with different requirements:

| Test case | What it tests | Extra env vars required |
|-----------|---------------|------------------------|
| TaskRun signing | Signature stored as annotation (`tekton` storage) | None |
| Image signing + attestation | Kaniko build → sign + attest via Rekor | `CHAINS_REPOSITORY`, `CHAINS_DOCKER_CONFIG_JSON` |

**TaskRun signing only (no registry needed):**
```bash
export KUBECONFIG=/path/to/kubeconfig
ginkgo run --label-filter='chains && sanity' --timeout=10m ./tests/chains/
```

**Full Chains suite (image signing, requires a registry):**
```bash
export KUBECONFIG=/path/to/kubeconfig
export CHAINS_REPOSITORY=quay.io/<your-org>/chainstest        # repo to push signed images to
export CHAINS_DOCKER_CONFIG_JSON=$(cat ~/.docker/config.json) # raw JSON — do NOT base64-encode

ginkgo run --label-filter=chains --timeout=30m ./tests/chains/
```

> **`CHAINS_DOCKER_CONFIG_JSON`** must be the **raw JSON** contents of a `docker/config.json`
> (i.e. the output of `cat ~/.docker/config.json`) that has push access to `CHAINS_REPOSITORY`.
> Do **not** base64-encode it — the test passes it directly to `oc create secret` as
> `--from-literal=.dockerconfigjson=<value>` which requires plain JSON.

## Local Testing Walkthrough

This section walks through the full cycle for testing changes on your own OpenShift cluster.

### 1 — One-time setup

```bash
# Clone and enter the repo
git clone https://github.com/openshift-pipelines/release-tests-ginkgo
cd release-tests-ginkgo

# Install the ginkgo CLI (if not already installed)
go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Log in to your local cluster
oc login --server=https://<api-url>:6443 -u kubeadmin -p <password>
# or point KUBECONFIG at an existing kubeconfig file:
export KUBECONFIG=$HOME/.kube/config

# Verify cluster access
oc whoami
oc get nodes
```

### 2 — Configure component versions

Edit `env/default/default.properties` to match the operator version installed on your cluster:

```bash
# Check what's currently installed
oc get csv -n openshift-operators | grep pipelines

# Then update env/default/default.properties accordingly, e.g.:
OPERATOR_VERSION = 0.79
OSP_VERSION = 1.22
PIPELINE_VERSION = v1.9
```

### 3 — Compile-check your changes

```bash
go build ./...
# or just the packages you changed:
go build ./pkg/operator/... ./pkg/diagnostics/... ./tests/olm/...
```

### 4a — Testing on a fresh cluster (operator not yet installed)

Use this path when running on a cluster where the operator has never been installed (mirrors CI exactly):

```bash
# Install the operator via OLM — runs the full ordered install sequence
# including the webhook readiness wait added in this PR
./scripts/run-tests.sh install

# Watch the install progress in another terminal:
oc get pods -n openshift-operators -w
oc get tektonconfig config -w

# Once install completes, run sanity tests to validate everything works
./scripts/run-tests.sh sanity
```

To simulate CI failure diagnostics: introduce a deliberate failure then inspect the JUnit report:

```bash
ARTIFACTS_DIR=./test-reports ./scripts/run-tests.sh install
# On failure, test-reports/junit-report.xml will contain an <operator-diagnostics>
# attachment with logs from openshift-operators pods
cat test-reports/junit-report.xml | grep -A 50 "operator-diagnostics"
```

### 4b — Testing on a cluster with the operator already installed

```bash
# Quick sanity pass — runs all sanity-labelled tests across all suites
./scripts/run-tests.sh sanity

# Run a specific suite
ginkgo run --timeout=30m ./tests/operator/
ginkgo run --timeout=30m ./tests/pipelines/
ginkgo run --timeout=30m ./tests/triggers/

# Run only the webhook-wait step in isolation
ginkgo run --label-filter=install --focus="waits for operator and pipelines webhook" \
    --timeout=5m ./tests/olm/

# Run a single named test by focusing on its description
ginkgo run --focus="validates RBAC" --timeout=5m ./tests/operator/
```

### 5 — Running with verbose output

```bash
# -v streams each spec name and GinkgoWriter output as it runs
ginkgo run -v --timeout=30m ./tests/operator/

# Show full failure detail including the cluster-diagnostics report entry
ginkgo run -v --label-filter=install --timeout=30m ./tests/olm/
```

### 6 — Running the full suite

```bash
# All feature tests — excludes install/upgrade/uninstall and disconnected
ginkgo run \
    --label-filter='!install && !upgrade && !uninstall && !disconnected' \
    --timeout=60m \
    ./tests/...
```

### 7 — Generating a JUnit report

```bash
ARTIFACTS_DIR=./test-reports ./scripts/run-tests.sh sanity
# report written to: ./test-reports/junit-report.xml
```

### 8 — Cleanup

The test framework automatically creates per-`Describe` namespaces and deletes them after each block (unless the block failed — failed namespaces are preserved for debugging).

To manually clean up any leftover test namespaces:

```bash
oc get projects | grep -E 'eco-|chains-|results-|triggers-|pipelines-' | awk '{print $1}' | xargs oc delete project
```

### Troubleshooting common local failures

| Symptom | Likely cause | Fix |
|---|---|---|
| `webhook not ready` / `connection refused` on patch | Operator webhook pod not up | The `WaitForOperatorWebhooks` step now handles this; if it still fails, check `oc get pods -n openshift-operators` |
| `TektonConfig CR not found` | OLM install slow | Increase `GINKGO_TIMEOUT`; check `oc get csv -n openshift-operators` |
| `namespace already exists` | Previous test run leaked namespaces | Run the manual cleanup command above |
| `oc: command not found` | `oc` not in PATH | Install from https://mirror.openshift.com/pub/openshift-v4/clients/ocp/ |
| Chains TC02 fails with auth error | `CHAINS_DOCKER_CONFIG_JSON` not set / wrong format | Must be raw JSON from `cat ~/.docker/config.json`, not base64 |

## Project Layout

```
tests/          # Ginkgo test suites (one directory per area)
  olm/          #   OLM install / upgrade / uninstall
  pipelines/    #   Pipelines core + resolvers
  operator/     #   Operator config (RBAC, auto-prune, HPA, etc.)
  triggers/     #   Triggers & EventListeners
  ecosystem/    #   Ecosystem tasks (buildah, s2i, jib, kn, git-clone)
  chains/       #   Tekton Chains (image signing)
  results/      #   Tekton Results
  pac/          #   Pipelines as Code
  hub/          #   Tekton Hub
  mag/          #   Manual Approval Gate
  metrics/      #   Prometheus metrics
  versions/     #   Component version verification
  tektonkueue/  #   Multi-cluster operator bootstrap
pkg/            # Shared helper packages
  clients/      #   Kubernetes/Tekton client wrappers
  oc/           #   oc CLI wrappers (Create, Apply, Delete, ...)
  olm/          #   OLM subscription helpers
  operator/     #   TektonConfig / component validation helpers
  pipelines/    #   PipelineRun validation helpers
  hooks/        #   Auto namespace-per-Describe lifecycle hook
  store/        #   Global current-namespace store
  config/       #   Reads env vars + default.properties
  wait/         #   Polling / retry utilities
testdata/       # YAML fixtures applied by tests (oc.Create / oc.Apply)
template/       # Go text/template files (e.g. subscription.yaml.tmp)
scripts/        # run-tests.sh entry point
env/default/    # default.properties — component versions and OLM config
```
