# Candor

[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/teerakarna/candor/badge)](https://securityscorecards.dev/viewer/?uri=github.com/teerakarna/candor)

A Kubernetes operator that ingests signals from security/observability tools (Trivy, and more
later), uses an LLM to enrich them into ranked hypotheses, and proposes remediation as GitOps pull
requests — with bounded LLM spend and a published record of its own accuracy.

📖 **[Full docs, architecture, and the case for Candor →](https://teerakarna.github.io/candor/)**

**Status**: early build, shipped in slices — see [`docs/design.md`](docs/design.md) for exactly
what's done.

## Features

**What sets Candor apart:**

- **Bounded LLM cost, by construction** — an LLM call happens once per distinct cluster-state
  fingerprint, ever, not once per reconcile. A hard budget ceiling degrades to deterministic-only
  findings on exhaustion rather than silently spending past it.
- **GitOps-native remediation** — the default write path is a pull request against your GitOps
  repo, never a direct cluster mutation.
- **Published accuracy** — every finding is re-checked automatically, and its outcome (resolved,
  still present, recurred) is a queryable Prometheus metric, not a claim in a README.
- **Suppression that resurfaces on its own** — mute a fingerprint with a required reason and
  optional expiry; if the underlying content actually changes, it stops matching and comes back
  automatically. Nothing to remember to clean up.
- **Untrusted-input safe** — all ingested telemetry is treated as data, never instructions. Model
  output is grammar-constrained to a fixed action catalog; it can't emit a free-form action.

**What you'd expect from a tool in this space, done properly:**

- Provider-based signal ingestion (Trivy today; the interface is provider-agnostic)
- Fully Kubernetes-native — `kubectl get findings`, no separate UI or database to run
- Prometheus metrics and a ready-made Grafana dashboard, shipped in the Helm chart
- A generic webhook sink and periodic digest — one JSON payload shape, works with Slack, Teams,
  PagerDuty, or anything that can receive a POST
- Every release is signed, with a CycloneDX SBOM and build provenance attached

## Quickstart

```sh
helm install candor oci://ghcr.io/teerakarna/charts/candor --version <version> \
  --namespace candor-system --create-namespace
```

Opt a namespace in:

```sh
kubectl apply -n <your-namespace> -f - <<EOF
apiVersion: candor.dev/v1alpha1
kind: SignalPolicy
metadata:
  name: default
spec:
  providers: [trivy]
  minSeverity: HIGH
EOF
```

If [Trivy Operator](https://github.com/aquasecurity/trivy-operator) is already scanning that
namespace, you'll see results as soon as it produces a `VulnerabilityReport`:

```sh
kubectl get findings -n <your-namespace>
```

That's it — deterministic findings work with zero further configuration. See
[Configuration](#configuration) below to enable LLM enrichment, a budget ceiling, suppression,
and notifications.

## Configuration

LLM enrichment (ranked hypotheses on each `Finding`) is opt-in - without an API key it's simply not
active, not degraded:

```sh
kubectl create secret generic candor-llm --namespace candor-system \
  --from-literal=anthropic-api-key=<your key>
```

Uses [Anthropic's structured outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs)
so enrichment output is grammar-constrained to the hypotheses schema - see
[`internal/llm/anthropic`](internal/llm/anthropic) and [`SECURITY.md`](SECURITY.md) for why that
matters given the input is untrusted scanner data. Override the model with
`CANDOR_LLM_MODEL` (default: `claude-sonnet-5`) via `manager.envOverrides` in the Helm chart's
values.

### Bounding the cost

```yaml
apiVersion: candor.dev/v1alpha1
kind: SignalPolicy
metadata:
  name: default
spec:
  providers: [trivy]
  minSeverity: HIGH
  budget:
    maxCalls: 50
    windowSeconds: 86400
```

On exhaustion, enrichment degrades to deterministic-only findings for the rest of the window.

### Suppressing a Finding

Mute a specific Finding by its exact content fingerprint:

```sh
kubectl get finding <name> -o jsonpath='{.status.fingerprint}'

kubectl apply -f - <<EOF
apiVersion: candor.dev/v1alpha1
kind: Suppression
metadata:
  name: known-false-positive
spec:
  fingerprint: "<the fingerprint above>"
  reason: "known false positive - tracked in TICKET-123"
  # expiresAt: "2026-12-31T00:00:00Z"   # omit for indefinite
EOF
```

Matching is exact by construction: if the underlying signal's content actually changes, its
fingerprint changes too, the Suppression no longer matches, and the Finding resurfaces on its own
- there's nothing to remember to delete or update.

### Webhook notifications and digests

Set `spec.webhook.url` on a `SignalPolicy` to get a JSON POST on every Finding created, resolved,
or recurred in that namespace, plus a periodic digest (`CANDOR_DIGEST_INTERVAL`, default `24h`)
tallying Findings by verification outcome and severity:

```yaml
apiVersion: candor.dev/v1alpha1
kind: SignalPolicy
metadata:
  name: team-a
spec:
  providers: [trivy]
  webhook:
    url: https://example.com/hooks/candor
```

It's a plain JSON POST with no vendor-specific formatting - point it at whatever turns JSON into a
Slack/Teams/PagerDuty message (a relay, a low-code webhook, etc.).

### Grafana dashboard

Ship a pre-built dashboard (LLM calls, enrichment skipped by reason, verification transitions,
budget usage, cost-avoidance ratio) as a ConfigMap the kube-prometheus-stack Grafana sidecar
auto-discovers:

```sh
helm upgrade --install candor oci://ghcr.io/teerakarna/charts/candor \
  --set prometheus.enabled=true --set grafanaDashboard.enabled=true
```

## Installing without Helm

Each [release](https://github.com/teerakarna/candor/releases) attaches a versioned `install.yaml`
(all resources, generated fresh at release time — not a stale copy on `main`):

```sh
kubectl apply -f https://github.com/teerakarna/candor/releases/download/<tag>/install.yaml
```

Both install paths are produced by the same release pipeline — nothing hand-built or committed to
`main`, so what you install is always a specific, versioned, signed release.

## Building from source (contributors)

### Prerequisites

- go version v1.24.6+
- docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster

### Build, deploy, and run against a dev cluster

```sh
make docker-build docker-push IMG=<some-registry>/candor:tag
make install                          # CRDs
make deploy IMG=<some-registry>/candor:tag
kubectl apply -k config/samples/      # sample SignalPolicy
```

> If you hit RBAC errors, make sure you're logged in with sufficient cluster privileges.

To tear back down:

```sh
kubectl delete -k config/samples/
make undeploy
make uninstall
```

The Helm chart source lives under `charts/chart/` (generated via `kubebuilder edit
--plugins=helm/v2-alpha --output-dir=charts`, regenerate the same way after changing the API or
RBAC). Published to the OCI registry on every tagged release, alongside the signed image.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) — in particular the design constraints PRs must respect
(bounded LLM calls, read-only cluster access by default, no free-form model-selected actions).

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026 Albert Asawaroengchai.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
