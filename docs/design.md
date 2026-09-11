# Candor — design doc

## The thesis

Existing LLM-assisted Kubernetes ops tools (k8sgpt, Kubernaut, HolmesGPT, kagent) treat model
invocation as stateless, unbounded, and unaccountable. Candor treats it as a metered, verifiable
resource. Same job — ingest signals, use an LLM to enrich them, help DevOps/DevSecOps/SRE act on
them — engineered against the specific, evidenced failures the incumbents have in production.

This is a "same job, better engineering" bet (Rust to C++), not a novelty claim. The differentiation
is an architectural stance, not a feature list.

## Origin and prior art

This project's first draft was a spec for "KubeGuard" — provider-pattern signal ingestion (K8s
events, Trivy, Falco, Wiz, Prisma Cloud, Datadog CWS), a pluggable local/hosted LLM abstraction,
non-destructive quarantine, circuit breakers, and phased GitOps auto-remediation. That name collides
with an existing enterprise product, an AppsCode auth tool, and an arXiv paper — renamed to Candor.

Research before building found the exact job already has direct competitors:

- **[Kubernaut](https://github.com/jordigilh/kubernaut)** — closest analogue. Alert → LLM
  investigation → remediation, with an effectiveness monitor that scores whether fixes worked. The
  only OSS project with an outcome feedback loop, but it's deferred to V1.1 pending 8+ weeks of
  data, and is currently blocked by an unrelated TLS bug.
- **[k8sgpt](https://github.com/k8sgpt-ai/k8sgpt) /
  [k8sgpt-operator](https://github.com/k8sgpt-ai/k8sgpt-operator)** — CNCF Sandbox, ~8k stars,
  actively maintained. Deterministic analyzers + LLM explanation. Its own issue tracker is the
  primary evidence base below.
- **[HolmesGPT](https://github.com/HolmesGPT/holmesgpt)** — read-only investigation, strongest
  runbook integration in the field. Its own finding: "without runbooks, the model just guesses."
- **[kagent](https://github.com/kagent-dev/kagent)** — generic agent runtime, not a remediation
  product. Its human-in-the-loop writeup states the safety line this project also follows: "Fully
  autonomous agents are fine for read-only operations. For anything that changes state, you need a
  gate."

Given that, being *better* than these — not merely different — is the only defensible bar.

## What the original spec got right — kept

- **Provider pattern**: don't bundle Falco/Trivy inside the operator; consume their CRs and
  webhooks. Lightweight, no vendor lock-in.
- **Non-destructive quarantine**: strip the workload label so the Service stops routing, rather than
  killing the pod. Traffic stops, forensics survive.
- **Circuit breakers**: max-disruption percentage, global rate limit, a panic switch that drops to
  Audit mode and pages a human.
- **Audit-by-default**, and a hard rule that non-deterministic models never autonomously execute
  destructive actions.

## The eight gaps, and how Candor answers them

| # | Gap in the incumbents (evidenced) | Candor's answer |
|---|---|---|
| 1 | No cost model. k8sgpt-operator#769: 164 findings → 9,300 Bedrock calls in 3 days. #730: analysis runs every reconcile (~30s) regardless of configured interval, no last-analysis tracking. #419 (decouple LLM spend from reconciles) closed as stale. | Content-addressed fingerprinting: hash (resource identity + relevant spec/status subset + analyzer verdict). LLM invoked **once per distinct fingerprint, ever**. Hard budget ceiling per window; on exhaustion, degrade to deterministic-only and say so. |
| 2 | No deduplication — an alert storm costs N× | Fingerprint collapses a storm to one enrichment call. |
| 3 | No suppression, so the tool recreates the fatigue it claims to fix. k8sgpt#372 ("Exclude a list of known issues") open since May 2023, never shipped: "We have no consensus on the design yet." | `Suppression` CRD mutes a fingerprint with a required reason and optional expiry. Exact by construction — if the underlying state changes, the fingerprint changes and the finding resurfaces on its own. |
| 4 | No verification of remediation outcomes. k8sgpt's own `AUTO_REMEDIATION.md` lists "Re-analysis proving findings are resolved" as not implemented. | Every finding carries a verification outcome (resolved / still-present / recurred / superseded), re-checked after state changes, exposed as metrics and status. |
| 5 | Writes to the cluster — breaks GitOps shops. Practitioner: "my FluxCD is going revert since you violated principle of 'All goes through GitOps'." | Default write path is a **pull request** against the GitOps repo. Direct mutation only for resources not under GitOps management, gated behind Enforcing mode. |
| 6 | Prompt injection unaddressed, in a security tool | All ingested telemetry is untrusted input. Structured extraction before prompting; model output selects from a **fixed action catalog** — it can never emit a free-form action. |
| 7 | Weakest model shipped as default. arXiv 2509.04191 (the KubeGuard paper) benchmarks Llama-3.1-8B at 0.79–0.81 F1 vs GPT-4o at 0.93–1.00 on exactly these manifest-analysis tasks. | Strong hosted model (Anthropic) as the v1 default. Per-model accuracy is measured and published against a fixture suite, not asserted. |
| 8 | No confidence modelling, no self-metrics, no least-privilege RBAC. Palark's k8sgpt review: non-deterministic recommendations across identical runs; once suggested rebooting the cluster; missed an initContainer `ErrImagePull`. | Ranked competing hypotheses with confidence, never one asserted cause. Full self-observability. Operator ServiceAccount is least-privilege, read-only by default, verified by a test that attempts a write and confirms denial. |

Plus a plain operational advantage: Kubernaut needs 9+ microservices. Candor is one operator binary
and a Helm chart — a difference in deployment and maintenance burden that users feel immediately.

## The trust problem this answers

- Majors & Hebert, SREcon25, ["AIOps: Prove It!"](https://www.usenix.org/conference/srecon25americas/presentation/majors) —
  an open letter asking vendors for "data on how often your system produces useful, actionable
  results." No OSS tool in this space has answered it.
- The Register (696 respondents): 60% cite lack of trust as the top AIOps adoption barrier, 59%
  require near-perfect accuracy before adoption. DevOps.com: only 12% use AIOps day-to-day, 7.5%
  call it high-value.
- "A single confident answer that's wrong is worse than no answer, because it sends a human down a
  road with the agent's credibility behind it." (HN, HyperProbe thread)
- LangChain's own lesson, cited approvingly here: a health check fanned out to ~20 Sonnet calls per
  run even when healthy; collapsing to one Haiku call cut cost 95–99% with no loss in detection.
  Candor's fingerprinting generalizes this fix structurally rather than requiring each integration to
  discover it independently.

## Interaction surfaces

Operators surface through CRDs, metrics, and sinks. The projects that built a bespoke web UI
(kagent, Keep, NudgeBee) became multi-service platforms — exactly the complexity this project exists
to avoid. No UI, by design.

- **`Finding` CRD status** — primary surface, `kubectl get findings`.
- **Prometheus `/metrics`** — findings raised/suppressed/verified/recurred, LLM calls, tokens, spend
  vs. budget, degraded state.
- **Kubernetes Events** — picked up by whatever event exporter is already in the cluster.
- **Grafana dashboard**, shipped in the Helm chart. Not decoration — for a tool whose pitch is
  publishing its own accuracy, the dashboard is the proof surface.
- **Generic webhook sink** — one code path covers Slack/Teams/PagerDuty/anything, no per-vendor SDKs.
- **Periodic digest report** — a scheduled summary (findings raised/resolved/recurred, accuracy,
  spend vs. budget) over the webhook sink. This is the artifact the "Prove It!" critique asks for.
- **The pull request is also a report** — in GitOps mode, the PR body carries the finding, ranked
  hypotheses, confidence, and evidence. No new surface for a GitOps team to learn.

Non-goals: web UI, per-vendor chat integrations. An MCP server and a `kubectl candor` krew plugin
are plausible later additions, not v1.

## Architecture

- **Signal providers**: a Go interface normalizing heterogeneous input into a common `Signal`. v1
  ships Trivy `VulnerabilityReport` CRs and a generic webhook receiver. Falco, Alertmanager, etc.
  slot in later without changing the core.
- **Fingerprint + budget layer**: sits between signals and the model. The load-bearing
  differentiator — nothing reaches the LLM without passing through it.
- **LLM abstraction**: pluggable interface. Anthropic is the v1 implementation (structured output,
  one env var, no local-model setup friction). OpenAI/Ollama follow later as proof the interface
  actually abstracts.
- **Action catalog**: fixed and enumerated. v1 ships `Notify` and `ProposePullRequest`.
  `IsolateServiceTraffic` (the kept quarantine idea) lands only once the verification loop is
  demonstrably working — it's the highest-risk action and earns its place last.
- **CRDs**: `SignalPolicy` (scope, providers, guardrails, mode, budget), `Finding` (one per
  fingerprint; ranked hypotheses, confidence, verification outcome), `Suppression`.
- **API group**: `candor.dev/v1alpha1`.

## Delivery slices

1. Repo prep, kubebuilder scaffold, CI, OSS boilerplate, release automation (goreleaser, cosign +
   SBOM + buildx provenance on the image, Helm chart published to GHCR), repo governance
   (branch protection, required signed commits, Dependabot + auto-merge, OpenSSF Scorecard,
   GOVERNANCE.md). **Done.**
2. `SignalPolicy` + `Finding` CRDs; Trivy provider; no LLM yet — deterministic findings only.
   **Done.** Identity between reconciles is currently the signal's source (provider + originating
   object), not real content-addressed fingerprinting — that's slice 3's job. Provider skips
   registering itself if the target CRD isn't installed, rather than crashing the manager.
3. Fingerprinting + cache + the cost regression test. **Done**, scoped honestly to what's provable
   without an LLM to gate yet: `Finding.status.fingerprint` (content hash) and `.enrichedFingerprint`
   (what was last enriched) exist and are wired through `Ingest`; `NeedsEnrichment` is the gate
   slice 4's real LLM call plugs into unchanged. No separate cache store - the Finding object's own
   status field *is* the cache, so there's no new storage to add. The cost regression test proves
   `NeedsEnrichment` makes exactly one true→false transition across N ingests of identical content,
   and reverses only when content actually changes - it can't yet prove "one LLM call" literally,
   since none exists; that's slice 4's own test, trivial once this mechanism exists to sit on.
4. LLM enrichment (Anthropic) behind the interface; ranked hypotheses with confidence. **Done.**
   `internal/llm.Client` is the interface; `internal/llm/anthropic` is the only implementation so
   far. Uses Anthropic's structured outputs (grammar-constrained JSON, GA as of late 2025) rather
   than the older tool-use-emulation workaround, verified against the real, current API request
   shape before building against it. `FindingReconciler` is the gate: `NeedsEnrichment` (slice 3)
   decides whether to call at all, so the cost regression test from slice 3 is now literal - it
   counts real (fake, interface-real) LLM calls across N reconciles, not just fingerprint
   transitions. `ANTHROPIC_API_KEY` unset is a fully supported configuration (deterministic
   findings only), not an error, matching the "provider not installed" stance from slice 2.
5. Budget ceiling + degraded mode + self-metrics + K8s Events. **Done.** `SignalPolicy.spec.budget`
   (`maxCalls`, `windowSeconds`, default 24h window) is optional - omitted means unlimited, relying
   entirely on the slice 3/4 fingerprint gate to bound cost. `internal/signal.CheckBudget` tracks
   usage on `SignalPolicy.status` (`budgetWindowStart`, `budgetCallsUsed`), resets on window expiry,
   and sets a `BudgetExhausted` condition. On exhaustion, `FindingReconciler` skips the LLM call
   (degrading to deterministic-only findings, not erroring) and emits a Warning Event on the
   `SignalPolicy` so degradation is visible without polling status. Four Prometheus counters/gauges
   (`internal/metrics`) expose calls made, calls skipped by reason, and budget used vs. limit per
   policy - proven with real registered-metric assertions (`testutil.ToFloat64`), not just "the code
   compiles". `TestFindingReconciler_BudgetCostRegression` proves a budget of 2 caps real LLM calls
   to exactly 2 across 5 genuinely distinct Findings - the literal ceiling claim, enforced by test.
6. `Suppression` CRD. **Done.** `Suppression.spec` is `fingerprint` (required - the exact
   `Finding.status.fingerprint` value to mute), `reason` (required - suppression is never silent),
   and optional `expiresAt`. Matching is exact by construction: `internal/signal.FindActiveSuppression`
   matches on the literal fingerprint string, so if the underlying signal's content genuinely
   changes, its fingerprint changes too, no longer matches, and the Finding resurfaces on its own -
   there's no separate "resolved" state to fall out of sync. `FindingReconciler` checks for an
   active Suppression before the LLM gate (so it applies even with no LLM configured - suppression
   is about noise, not just cost), sets a `Suppressed` condition, and skips enrichment while active.
   It watches `Suppression` objects directly (not just `Finding`), so creating, editing, or deleting
   one re-evaluates every Finding it could affect immediately, rather than waiting for some
   unrelated event to touch them. `SuppressionReconciler` maintains its own `Expired` condition
   (requeued exactly at `expiresAt`) purely so `kubectl get suppressions` shows a lapsed one at a
   glance. Also fixed in passing: `no_llm_configured` was documented in the
   `candor_enrichment_skipped_total` metric's help text since slice 5 but never actually
   incremented - it is now.
7. Verification loop + published accuracy metrics + Grafana dashboard in the chart. **Done.**
   `Finding.status.verificationOutcome` (StillPresent/Resolved/Recurred - three of the design doc's
   four states; "Superseded" is deliberately not implemented, since it would need multiple
   providers reporting conflicting signals about the same Finding, which doesn't happen yet) is
   written by `internal/signal.Ingest` on every reconcile of the Finding's source, not asserted
   once at creation. The gap this closes: previously, a Trivy `VulnerabilityReport` whose
   vulnerabilities got fixed (or dropped below every `SignalPolicy`'s threshold) left its Finding
   showing stale severity forever - `translate()` now produces a Signal even for a clean report
   (with an empty Severity that `signal.AtLeast` guarantees never clears any real threshold), so
   `Ingest`'s existing filtered path can recognise it and resolve the Finding instead of just
   discarding the signal. `candor_verification_transitions_total` (by outcome) is the published
   accuracy signal - the resolved:recurred ratio over time is an honest, provable "did this
   actually get fixed", not a calibrated ML accuracy score against a fixture suite (that's a
   distinct, separately-deferred piece of future work, not something this slice claims). A Grafana
   dashboard (`charts/chart/files/grafana-dashboard.json`, shipped as a ConfigMap gated by
   `grafanaDashboard.enabled`, labelled for the kube-prometheus-stack sidecar to auto-discover)
   plots LLM calls, enrichment skipped by reason, verification transitions, budget usage, and the
   cost-avoidance ratio - the dashboard is the proof surface docs/design.md's interaction-surfaces
   section calls for, not decoration. Also discovered and documented: a custom top-level
   `values.yaml` key doesn't survive `kubebuilder edit --force` (the whole file is regenerated from
   the plugin's own template, not merged) - a fourth recurring hand-fix, added to CONTRIBUTING.md
   alongside the original three. Tested for real, not just "the JSON parses": CI imports the
   dashboard into a real Grafana instance and checks every panel round-trips (structural, not full
   schema validation - Grafana's save API doesn't reject a bad panel type or query), and
   `hack/grafana-preview/` is a local `docker compose` stack (fake metrics + Prometheus + Grafana,
   both auto-provisioned) for a human visual check without a real cluster.
8. Generic webhook sink + periodic digest report. **Done.** `SignalPolicy.spec.webhook.url`
   (optional) is the single opt-in for both: `internal/notify` is a small, vendor-agnostic package
   that POSTs a JSON payload and knows nothing about Slack/Teams/PagerDuty - "one code path", per
   the design. `internal/signal.Ingest` sends an immediate `Event` notification (FindingCreated,
   FindingResolved, FindingRecurred) on real transitions only - not on a routine content refresh or
   an unchanged reconcile, which would just be noise. `internal/controller.DigestRunnable` is a
   plain manager `Runnable` (a ticker loop, not a CRD-triggered reconciler - there's no event for
   "time has passed") that, on `CANDOR_DIGEST_INTERVAL` (default 24h), lists every SignalPolicy
   with a Webhook configured and sends a per-namespace `Digest` tallying Findings by verification
   outcome and severity - the "AIOps: Prove It!" artifact the evidence base calls for. Both send
   paths are best-effort: a failure is logged and counted (`candor_webhook_sends_total`), never
   returned as an error - a flaky notification endpoint must not make Finding reconciliation or
   the digest loop get stuck.
9. `ProposePullRequest` action against the GitOps repo.
10. Second provider (webhook ingest) + second LLM backend — proves both interfaces actually abstract.
11. Blog article.

Quarantine/Enforcing mode is explicitly post-v1, gated on slice 7.

Deferred, deliberately not slice 1: SLSA provenance for the goreleaser-built binary archives (the
container image already gets buildx's native provenance attestation — the binaries would need the
official `slsa-framework/slsa-github-generator` reusable workflow, which is a real new job wired to
goreleaser's checksums, not a small extension of what already exists).

## Verification plan

- `envtest` reconciler tests: fingerprint stability (same state → same hash, changed state → new
  hash), budget exhaustion → degraded mode, suppression matching and expiry, verification outcome
  transitions.
- **Cost regression test**: N reconciles over unchanged state must produce exactly **one** LLM call.
  This is the project's core claim, and it's enforced by a test, not a README assertion.
- **Prompt-injection test**: a crafted malicious log line / image annotation must not alter the
  selected action.
- **RBAC test**: attempt a write with the operator's ServiceAccount; confirm denial.
- Deployed to the existing kind + ArgoCD cluster from `gitops-demo`; run against real
  dev/preprod/prod/pr-N namespaces. Every finding manually confirmed real — zero false positives —
  before any public write-up.
