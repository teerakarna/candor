---
title: Candor
description: A Kubernetes operator that treats LLM invocation as a metered, verifiable resource — not a stateless, unbounded one.
hide:
  - navigation
  - toc
---

# Candor

**A Kubernetes operator that ingests security signals, enriches them with an LLM under a hard
cost ceiling, and proposes fixes as GitOps pull requests — with a published record of its own
accuracy.**

[Why Candor](why-candor.md){ .md-button .md-button--primary }
[Get started](getting-started.md){ .md-button }
[View on GitHub :fontawesome-brands-github:](https://github.com/teerakarna/candor){ .md-button }

---

## Same job, engineered against what actually breaks in production

k8sgpt, Kubernaut, and HolmesGPT already do this job: ingest a signal, ask an LLM what it means,
hand a human something actionable. Candor does the same underlying job. The difference isn't a
feature — it's that every load-bearing weak point in the incumbents shows up, with a GitHub issue
number attached, in their own trackers. Candor is engineered against those specific failures,
not against a hypothetical.

This is a **Rust-to-C++ bet**: same job, better engineering — not a novelty claim.

<div class="grid cards" markdown>

-   :material-currency-usd-off:{ .lg .middle } __Bounded cost, by construction__

    ---

    One LLM call per distinct cluster-state fingerprint, **ever** — not once per reconcile. A
    hard budget ceiling degrades to deterministic-only findings rather than silently spending
    past it. k8sgpt-operator's own tracker: 164 findings produced 9,300 model calls in three
    days. Candor's cost regression test enforces the opposite, on every PR.

-   :material-source-pull:{ .lg .middle } __GitOps-native__

    ---

    The default write path is a pull request against your GitOps repo, not a cluster mutation —
    so it never fights Flux or Argo for control. *(Landing in the next milestone — see
    [status](#status).)*

-   :material-chart-line:{ .lg .middle } __Self-measuring__

    ---

    Every finding is re-checked, and its outcome — resolved, still present, or recurred — is
    published as a Prometheus metric and a Grafana dashboard shipped in the chart. Not a claim
    in a README: a number you can query.

</div>

## The trust problem nobody in this category has answered

> "Data on how often your system produces useful, actionable results."
>
> — Majors & Hebert, SREcon25, [*AIOps: Prove It!*](https://www.usenix.org/conference/srecon25americas/presentation/majors)

60% of practitioners cite lack of trust as the top barrier to AIOps adoption; 59% require
near-perfect accuracy before they'll rely on it (The Register, 696 respondents). No OSS tool in
this space has published the number that would settle it. Candor's accuracy metric — the
resolved-vs-recurred ratio, tracked over time — is that number, and it ships in v1, not as a
someday roadmap item.

## Status

Candor is an early, actively-developed build, shipped in small, individually-verified slices —
each one tested against a real Kubernetes control plane, not mocked away. Deterministic
findings, content-addressed fingerprinting, LLM enrichment, a hard budget ceiling, Suppressions,
the verification loop, and a generic webhook sink are done. See
[the full delivery plan and rationale](https://github.com/teerakarna/candor/blob/main/docs/design.md)
for exactly what's shipped and what isn't yet.

[Read the full case for Candor :material-arrow-right:](why-candor.md){ .md-button .md-button--primary }
