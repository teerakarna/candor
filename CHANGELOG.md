# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Project scaffolding (kubebuilder), CI, OSS boilerplate, release automation, repo governance.
- `SignalPolicy` and `Finding` CRDs with real fields (deterministic slice only — no LLM yet).
  `Suppression` remains scaffolded, not yet implemented (a later slice).
- First signal provider: Trivy Operator `VulnerabilityReport` → `Finding`, gated by a
  `SignalPolicy` in the same namespace. Read-only RBAC on the external CRD; skips registering
  itself (rather than crashing the manager) if Trivy Operator isn't installed.
- Content-addressed fingerprinting (`Finding.status.fingerprint`): the core cost-accountability
  mechanism (docs/design.md pillar 2). Re-ingesting unchanged content never looks new, and content
  actually changing makes a settled Finding "need enrichment" again. No LLM call exists yet to
  gate (slice 4) - this slice proves the mechanism the real gate gets built directly on top of.
- LLM enrichment (`internal/llm`, `internal/llm/anthropic`): `FindingReconciler` calls Anthropic
  exactly once per distinct fingerprint, gated by the mechanism from the previous entry - the
  actual "N reconciles, one call" claim is now a passing test, not just a design intention.
  Response is grammar-constrained to ranked hypotheses with confidence via Anthropic's structured
  outputs - never a single asserted cause, and no free-form output path for injected scanner data
  to escape through. Opt-in: no `ANTHROPIC_API_KEY` means deterministic findings only, a fully
  supported configuration, not a degraded one.
- Budget ceiling on LLM enrichment spend: `SignalPolicy.spec.budget` (max calls per rolling window,
  optional - unlimited if omitted). On exhaustion, enrichment degrades to deterministic-only
  findings (not an error) and a Warning Event fires on the `SignalPolicy`. Self-observability via
  four Prometheus metrics (`candor_llm_calls_total`, `candor_enrichment_skipped_total`,
  `candor_signalpolicy_budget_calls_used`, `candor_signalpolicy_budget_calls_limit`) exposed on the
  existing manager metrics endpoint - proven with real metric-value assertions, and a test proving a
  budget of 2 caps real LLM calls to exactly 2 across 5 distinct Findings.
- `Suppression` CRD: mute a Finding by its exact content fingerprint (`spec.fingerprint`), with a
  required `spec.reason` and optional `spec.expiresAt`. Exact by construction - if the underlying
  content actually changes, the fingerprint changes too and the Finding resurfaces on its own, no
  separate "resolved" tracking needed. Applies before the LLM gate, so it works even with no LLM
  configured. `FindingReconciler` now watches `Suppression` objects directly, so create/edit/delete
  takes effect immediately rather than waiting on the next unrelated Finding event.
- Verification loop: `Finding.status.verificationOutcome` (StillPresent/Resolved/Recurred),
  re-checked by `internal/signal.Ingest` every time a Finding's source is reconciled. A Trivy
  `VulnerabilityReport` that drops to zero vulnerabilities (or below every `SignalPolicy`'s
  threshold) now resolves its Finding instead of leaving it showing stale severity forever; a
  resolved Finding whose source produces a real signal again is marked Recurred. Published as
  `candor_verification_transitions_total` (by outcome) - the resolved:recurred ratio over time is
  Candor's accuracy signal.
- Grafana dashboard (`charts/chart/files/grafana-dashboard.json`), shipped as a ConfigMap behind
  `grafanaDashboard.enabled` (off by default), labelled for the kube-prometheus-stack Grafana
  sidecar to auto-discover. Plots LLM calls, enrichment skipped by reason, verification
  transitions, budget usage, and the enrichment-calls-avoided ratio.
- Generic webhook sink (`internal/notify`) + periodic digest: `SignalPolicy.spec.webhook.url`
  (optional, one URL per namespace) drives both an immediate notification on a Finding created,
  resolved, or recurred, and a periodic per-namespace digest (`CANDOR_DIGEST_INTERVAL`, default
  24h) tallying Findings by verification outcome and severity. Both are best-effort - a failure is
  logged and counted (`candor_webhook_sends_total`), never returned as an error.
- `candor_findings_current{namespace,severity,outcome}`: a live gauge (Prometheus `Collector`,
  computed fresh on every scrape from the manager's cached client, not a package-level counter
  like every other metric here) reporting how many Findings currently exist per severity and
  verification outcome. None of the existing metrics could answer "how many findings are open
  right now" - they're all transition counters or self-observability. This is the metric behind
  the redesigned dashboard's landing status tiles.
- `candor_verification_transitions_total` now also carries a `severity` label (previously
  `outcome` only), so Resolved/Recurred can be broken down per severity, not just in aggregate.
- Redesigned Grafana dashboard: leads with four colour-coded severity tiles (Critical/High/Medium/
  Low), each showing the current open count and the 7-day resolved count together, so the first
  thing an operator sees is what needs action versus what's been taken care of - not Candor's own
  cost metrics. Recurrence is broken out into its own panel, by severity, since a regression is a
  different signal from a new finding at the same severity. Historical trends and cost/operator-
  health panels (LLM calls, budget usage, enrichment-avoidance ratio) are still present, grouped
  under labelled rows, as drill-down rather than the first view.

### Fixed

- `candor_enrichment_skipped_total{reason=no_llm_configured}` is now actually incremented - it was
  documented in the metric's help text since the budget-ceiling slice but never wired up.

### Dev tooling

- CI now imports `charts/chart/files/grafana-dashboard.json` into a real Grafana instance
  (`grafana-dashboard` job) and checks every panel round-trips - catches the file failing to parse
  or the import silently dropping panels. Grafana's save API doesn't validate panel/query
  correctness, so this is a structural check, not full schema validation.
- `hack/grafana-preview/`: a `docker compose` stack (fake metrics generator + Prometheus + Grafana,
  both auto-provisioned) for visually checking the dashboard locally without a real cluster or
  operator - see its README.
- `render-diagrams.yml`: auto-renders `docs-site/docs/assets/*.mmd` to a matching high-resolution
  SVG whenever the source changes, committing the result back to the PR - nobody has to remember
  to run `mermaid-cli` locally after editing a diagram.
