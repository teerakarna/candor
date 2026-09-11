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
