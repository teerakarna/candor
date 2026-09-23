// Package llm defines the pluggable interface every LLM backend implements, and the narrow
// request/response shape that crosses it. Anthropic (internal/llm/anthropic) is v1's only real
// implementation - Ollama/OpenAI are later implementations of this same interface, which is the
// actual proof the abstraction holds (see docs/design.md's delivery slices).
package llm

import "context"

// Request is the deterministic facts about one Finding, and nothing else - the narrowest input
// that can produce an informed enrichment. No raw signal payloads, no other Findings' data, no
// cluster state beyond what's already in Finding.Spec. Every field here is treated as untrusted
// data by the implementation (see internal/llm/anthropic's prompt), never as instructions.
type Request struct {
	Provider string
	Severity string
	Summary  string
	Kind     string
	Name     string
}

// Hypothesis is one possible explanation for a Finding, with the model's own confidence in it.
// A Response never carries a single asserted cause - only ranked hypotheses (docs/design.md
// pillar 4) - because the evidence base this project is built on is explicit that a single
// confident wrong answer is worse than an admittedly uncertain one.
type Hypothesis struct {
	Cause      string
	Confidence float64 // 0.0-1.0
	Rationale  string

	// RecommendedAction is the model's suggestion for which action from Candor's fixed catalog
	// (candorv1alpha1.ActionNotify / ActionProposePullRequest) this hypothesis warrants. Advisory
	// only - see candorv1alpha1.Hypothesis.RecommendedAction's doc comment for why this can never
	// itself authorize a write. This package intentionally doesn't import api/v1alpha1 to name the
	// two values directly (llm stays decoupled from the CRD types, matching how Confidence's own
	// 0.0-1.0-vs-0-100 conversion already crosses that boundary in
	// internal/controller.FindingReconciler); an empty or unrecognised string is always valid input
	// here; the CRD boundary is where it gets validated.
	RecommendedAction string
}

// Response is one enrichment result.
type Response struct {
	Hypotheses []Hypothesis
}

// Client enriches a Request into a Response. Implementations own their own retry/backoff
// decisions internally where that matters (e.g. rate limits); a returned error causes the
// caller (FindingReconciler) to requeue with controller-runtime's own backoff, so implementations
// don't need to hand-roll that themselves.
type Client interface {
	Enrich(ctx context.Context, req Request) (Response, error)
}
