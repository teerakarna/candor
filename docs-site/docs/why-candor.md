# Why Candor

## The thesis

Existing LLM-assisted Kubernetes ops tools — [k8sgpt](https://github.com/k8sgpt-ai/k8sgpt),
[Kubernaut](https://github.com/jordigilh/kubernaut), [HolmesGPT](https://github.com/HolmesGPT/holmesgpt),
[kagent](https://github.com/kagent-dev/kagent) — treat model invocation as stateless, unbounded,
and unaccountable. Candor treats it as a metered, verifiable resource. Same job: ingest signals,
use an LLM to enrich them, help DevOps/DevSecOps/SRE act on them. The differentiation is an
architectural stance, not a feature list.

## Eight evidenced gaps, and how Candor answers them

Every row below traces to a real issue, a real benchmark, or a real practitioner quote — not a
hypothetical failure mode.

| # | Gap in the incumbents (evidenced) | Candor's answer |
|---|---|---|
| 1 | **No cost model.** [k8sgpt-operator#769](https://github.com/k8sgpt-ai/k8sgpt-operator/issues/769): 164 findings → 9,300 Bedrock calls in 3 days. [#730](https://github.com/k8sgpt-ai/k8sgpt-operator/issues/730): analysis runs every reconcile (~30s) regardless of configured interval. [#419](https://github.com/k8sgpt-ai/k8sgpt-operator/issues/419) (decouple LLM spend from reconciles) closed as stale. | Content-addressed fingerprinting: hash of resource identity + relevant spec/status + analyzer verdict. The LLM runs **once per distinct fingerprint, ever**. A hard budget ceiling per window degrades to deterministic-only findings on exhaustion, and says so loudly. |
| 2 | **No deduplication** — an alert storm costs N× the enrichment spend. | The fingerprint collapses a storm to a single enrichment call. |
| 3 | **No suppression**, so the tool recreates the fatigue it claims to fix. [k8sgpt#372](https://github.com/k8sgpt-ai/k8sgpt/issues/372) ("Exclude a list of known issues") open since May 2023, never shipped. | A `Suppression` CRD mutes a fingerprint with a required reason and optional expiry. Exact by construction — if the underlying state changes, the fingerprint changes and the finding resurfaces on its own. |
| 4 | **No verification of remediation outcomes.** k8sgpt's own `AUTO_REMEDIATION.md` lists "re-analysis proving findings are resolved" as not implemented. | Every finding carries a verification outcome — resolved, still present, or recurred — re-checked on every reconcile of its source, published as metrics and status. |
| 5 | **Writes to the cluster** — breaks GitOps shops. A practitioner, on a competing tool: *"my FluxCD is going revert since you violated principle of 'All goes through GitOps'."* | The default write path is a **pull request** against the GitOps repo. Direct cluster mutation only for resources not under GitOps management, gated behind an explicit Enforcing mode. |
| 6 | **Prompt injection unaddressed**, in a security tool. | All ingested telemetry is untrusted input by construction. Structured extraction before prompting; model output is grammar-constrained to a fixed schema — it can never emit a free-form action. |
| 7 | **Weakest model shipped as default.** [arXiv 2509.04191](https://arxiv.org/abs/2509.04191) (the KubeGuard paper) benchmarks Llama-3.1-8B at 0.79–0.81 F1 vs GPT-4o at 0.93–1.00 on exactly these manifest-analysis tasks. | A strong hosted model (Anthropic) is the default. Per-model accuracy is measured and published, not asserted. |
| 8 | **No confidence modelling, no self-metrics, no least-privilege RBAC.** Palark's k8sgpt review: non-deterministic recommendations across identical runs; once suggested rebooting the cluster; missed an `initContainer` `ErrImagePull`. | Ranked, competing hypotheses with confidence — never one asserted cause. Full self-observability via Prometheus. The operator's ServiceAccount is least-privilege and read-only by default. |

Plus a plain operational advantage: Kubernaut needs 9+ microservices. Candor is one operator
binary and a Helm chart — a difference in deployment and maintenance burden you feel immediately.

## What the strongest prior art got right — and Candor kept

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
- LangChain's own lesson: a health check fanned out to ~20 Sonnet calls per run even when
  healthy; collapsing to one Haiku call cut cost 95–99% with no loss in detection. Candor's
  fingerprinting generalizes that fix structurally, rather than requiring every integration to
  rediscover it independently.

[See the architecture :material-arrow-right:](architecture.md){ .md-button .md-button--primary }
