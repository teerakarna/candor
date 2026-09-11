// Package metrics defines Candor's self-observability metrics (docs/design.md: "reconciles, LLM
// calls, tokens, spend vs budget, degraded state") and registers them with controller-runtime's
// existing metrics registry - already served on the manager's own metrics endpoint
// (cmd/main.go's metricsServerOptions), so nothing new needs standing up to expose these.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// LLMCallsTotal counts real LLM enrichment calls, by result. This is the number the whole
	// fingerprint/budget mechanism exists to keep small - watch this alongside
	// EnrichmentSkippedTotal to see the ratio directly.
	LLMCallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "candor_llm_calls_total",
		Help: "Total LLM enrichment calls made, by result (success|error).",
	}, []string{"result"})

	// EnrichmentSkippedTotal counts every time a Finding was NOT sent to the LLM, by reason. In
	// steady state (unchanged content) this should dominate LLMCallsTotal by a wide margin - that
	// ratio is the direct, visible proof of docs/design.md pillar 2.
	EnrichmentSkippedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "candor_enrichment_skipped_total",
		Help: "Total times enrichment was skipped without calling the LLM, by reason (not_needed|no_llm_configured|budget_exhausted|suppressed).",
	}, []string{"reason"})

	// BudgetCallsUsed and BudgetCallsLimit are gauges per SignalPolicy - cardinality is bounded by
	// the number of SignalPolicy objects in the cluster (real k8s objects, not user-controlled
	// free text), so this doesn't risk unbounded label cardinality.
	BudgetCallsUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "candor_signalpolicy_budget_calls_used",
		Help: "LLM calls used in the current budget window, per SignalPolicy.",
	}, []string{"namespace", "signalpolicy"})

	BudgetCallsLimit = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "candor_signalpolicy_budget_calls_limit",
		Help: "Configured max LLM calls per budget window, per SignalPolicy. Absent if the policy has no budget configured (unlimited).",
	}, []string{"namespace", "signalpolicy"})

	// VerificationTransitionsTotal is Candor's published accuracy signal (docs/design.md pillar
	// 4): every time Ingest re-checks a Finding's source and its verification outcome actually
	// changes, this counts the transition, by outcome (still_present|resolved|recurred). The
	// resolved:recurred ratio over time is the honest, provable version of "did this actually get
	// fixed" - not a calibrated accuracy score against ground truth (docs/design.md notes that as
	// a deliberately deferred, separate piece of work, not implemented here).
	VerificationTransitionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "candor_verification_transitions_total",
		Help: "Total verification outcome transitions recorded while ingesting signals, by outcome (still_present|resolved|recurred).",
	}, []string{"outcome"})

	// WebhookSendsTotal counts every internal/notify.Send call, by result. Covers both immediate
	// Finding notifications (internal/signal.Ingest) and the periodic digest
	// (internal/controller.DigestRunnable) - Send never blocks either caller on failure, so this
	// is how a broken webhook endpoint becomes visible instead of silently swallowed.
	WebhookSendsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "candor_webhook_sends_total",
		Help: "Total webhook notification/digest sends, by result (success|error).",
	}, []string{"result"})
)

func init() {
	metrics.Registry.MustRegister(LLMCallsTotal, EnrichmentSkippedTotal, BudgetCallsUsed, BudgetCallsLimit,
		VerificationTransitionsTotal, WebhookSendsTotal)
}
