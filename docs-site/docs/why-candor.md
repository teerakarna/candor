# Why Candor

## The thesis

Candor treats LLM invocation as a metered, verifiable resource, not a stateless, unbounded one.
Ingest a signal, use an LLM to enrich it, help DevOps/DevSecOps/SRE act on it — the job itself is
familiar. Candor's focus is the engineering discipline around that job: what it costs, what it
writes, and whether it was actually right.

## Design principles

Each of these is a real, load-bearing property of Candor — not a feature list, an architectural
stance.

**Bounded cost, by construction.** Content-addressed fingerprinting hashes resource identity,
relevant state, and signal verdict; the LLM runs once per distinct fingerprint, ever, not once
per reconcile. A hard budget ceiling per window degrades to deterministic-only findings on
exhaustion, and says so loudly rather than silently spending past it. LangChain's own published
lesson is the generalizable version of this: a health check that fanned out to ~20 Sonnet calls
per run even when healthy dropped to one Haiku call with no loss in detection once someone
thought to dedupe first. Candor bakes that discipline into the architecture rather than leaving
each integration to rediscover it.

**Deduplication and suppression.** An identical or repeated signal collapses into a single
enrichment, not one call each. A `Suppression` CRD mutes a specific fingerprint with a required
reason and optional expiry — exact by construction, so if the underlying state actually changes,
the fingerprint changes with it and the finding resurfaces on its own. Nothing to remember to
clean up.

**Verification, not assertion.** Every finding carries a verification outcome — resolved, still
present, or recurred — re-checked automatically on every reconcile of its source, and published
as both status and a Prometheus metric.

**GitOps-native writes.** The default write path is a pull request against your GitOps repo, not
a cluster mutation, so it never fights Flux or Argo for control of your cluster. Direct mutation
is reserved for resources outside GitOps management, gated behind an explicit mode you opt into.

**Untrusted input by design.** All ingested telemetry — scanner output, log lines, anything from
outside the cluster — is treated as untrusted data, never as instructions. Structured extraction
happens before prompting, and model output is grammar-constrained to a fixed schema: it can
select from a known action catalog, and can never emit a free-form action.

**A strong default model, with accuracy you can check.** Published benchmarks on exactly this
class of manifest-analysis task ([arXiv 2509.04191](https://arxiv.org/abs/2509.04191)) show a
real accuracy gap between small local models and larger hosted ones. Candor defaults to a strong
hosted model, and measures and publishes per-model accuracy rather than asserting it.

**Confidence, observability, and least privilege.** Findings carry ranked, competing hypotheses
with confidence — never a single asserted cause. Every part of the pipeline is self-observable
via Prometheus. The operator's ServiceAccount is least-privilege and read-only by default.

## What good prior art got right — and Candor kept

Kubernetes signal-to-remediation tooling has converged on some genuinely good ideas. Candor keeps
them rather than reinventing:

- **The provider pattern.** Don't bundle Falco or Trivy inside the operator — consume their CRs
  and webhooks. Lightweight, no vendor lock-in.
- **Non-destructive quarantine** *(post-v1)*. Strip the workload label so the Service stops
  routing, rather than killing the pod — traffic stops, forensics survive.
- **Circuit breakers** *(post-v1)*: max-disruption percentage, a global rate limit, a panic
  switch that drops to audit-only and pages a human.
- **Audit-by-default**, and a hard rule that non-deterministic models never autonomously execute
  a destructive action.

## The trust problem

> "Data on how often your system produces useful, actionable results."
>
> — Majors & Hebert, SREcon25, [*AIOps: Prove It!*](https://www.usenix.org/conference/srecon25americas/presentation/majors)

- The Register (696 respondents): 60% cite lack of trust as the top AIOps adoption barrier; 59%
  require near-perfect accuracy before adoption.
- *"A single confident answer that's wrong is worse than no answer, because it sends a human down
  a road with the agent's credibility behind it."* — Hacker News, HyperProbe thread.

Candor's answer is structural, not a promise: publish the verification outcome of every finding,
as a queryable number, from v1.

[See the architecture :material-arrow-right:](architecture.md){ .md-button .md-button--primary }
