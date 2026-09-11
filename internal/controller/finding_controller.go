/*
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
*/

package controller

import (
	"context"
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/llm"
	"github.com/teerakarna/candor/internal/metrics"
	"github.com/teerakarna/candor/internal/signal"
)

// FindingReconciler reconciles a Finding object: when its content needs enrichment (see
// internal/signal.NeedsEnrichment - the gate slice 3 built for exactly this), it calls LLM.Enrich
// exactly once and records the result. When it doesn't, this is a true no-op - no LLM call, no
// write. That's the entire cost-accountability claim in one sentence: unchanged content is never
// re-enriched, no matter how many times this function runs against it.
type FindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// LLM enriches Findings. nil means enrichment is disabled (no API key configured) -
	// Reconcile skips enrichment entirely rather than erroring, the same "not configured, so
	// skip" stance internal/provider takes for a missing CRD. A cluster running Candor for
	// deterministic findings only, with no LLM cost at all, is a fully supported configuration,
	// not a degraded one.
	LLM llm.Client

	// Recorder emits Kubernetes Events on state transitions worth an operator's attention (right
	// now: budget exhaustion). Required if LLM is set - see SetupWithManager.
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=candor.dev,resources=findings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=candor.dev,resources=findings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=candor.dev,resources=findings/finalizers,verbs=update
// +kubebuilder:rbac:groups=candor.dev,resources=signalpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=candor.dev,resources=signalpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *FindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.LLM == nil {
		return ctrl.Result{}, nil
	}

	finding := &candorv1alpha1.Finding{}
	if err := r.Get(ctx, req.NamespacedName, finding); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !signal.NeedsEnrichment(finding) {
		metrics.EnrichmentSkippedTotal.WithLabelValues("not_needed").Inc()
		return ctrl.Result{}, nil
	}

	// The budget lives on whichever SignalPolicy governs this Finding's provider in its
	// namespace - same lookup Ingest uses to decide whether to accept a signal at all. If the
	// policy was deleted after ingest (an edge case, not the common path), there's nothing to
	// enforce against - treated as unlimited, same as a policy with no Budget configured.
	policy, err := signal.FindPolicy(ctx, r.Client, finding.Namespace, finding.Spec.Source.Provider)
	if err != nil {
		return ctrl.Result{}, err
	}
	if policy != nil {
		allowed, err := signal.CheckBudget(ctx, r.Client, policy)
		if err != nil {
			return ctrl.Result{}, err
		}
		if policy.Spec.Budget != nil {
			metrics.BudgetCallsLimit.WithLabelValues(policy.Namespace, policy.Name).Set(float64(policy.Spec.Budget.MaxCalls))
			metrics.BudgetCallsUsed.WithLabelValues(policy.Namespace, policy.Name).Set(float64(policy.Status.BudgetCallsUsed))
		}
		if !allowed {
			metrics.EnrichmentSkippedTotal.WithLabelValues("budget_exhausted").Inc()
			// Emitted every time budget blocks a call, not just on the first transition - the
			// Kubernetes Events API already coalesces repeated identical (object, reason)
			// events into one Event with an incrementing count, so this doesn't spam.
			r.Recorder.Eventf(policy, corev1.EventTypeWarning, "BudgetExhausted",
				"enrichment for %s/%s skipped - budget exhausted (%d/%d calls this window)",
				finding.Namespace, finding.Name, policy.Status.BudgetCallsUsed, policy.Spec.Budget.MaxCalls)
			return ctrl.Result{}, nil
		}
	}

	resp, err := r.LLM.Enrich(ctx, llm.Request{
		Provider: finding.Spec.Source.Provider,
		Severity: finding.Spec.Severity,
		Summary:  finding.Spec.Summary,
		Kind:     finding.Spec.Source.Kind,
		Name:     finding.Spec.Source.Name,
	})
	if err != nil {
		metrics.LLMCallsTotal.WithLabelValues("error").Inc()
		// Returned as-is: controller-runtime requeues failed reconciles with its own
		// exponential backoff, so a transient API error (rate limit, timeout) retries on its
		// own without this needing to hand-roll that.
		log.Error(err, "LLM enrichment failed")
		return ctrl.Result{}, err
	}
	metrics.LLMCallsTotal.WithLabelValues("success").Inc()

	finding.Status.Hypotheses = toHypotheses(resp.Hypotheses)
	finding.Status.EnrichedFingerprint = finding.Status.Fingerprint
	if err := r.Status().Update(ctx, finding); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// toHypotheses converts llm.Hypothesis (Confidence as a 0.0-1.0 float64, fine for an in-process
// API response type) to the CRD's Hypothesis (Confidence as a 0-100 int32 - Kubernetes API
// convention discourages floats in CRD schemas; see the field's doc comment).
func toHypotheses(in []llm.Hypothesis) []candorv1alpha1.Hypothesis {
	out := make([]candorv1alpha1.Hypothesis, 0, len(in))
	for _, h := range in {
		out = append(out, candorv1alpha1.Hypothesis{
			Cause:      h.Cause,
			Confidence: int32(math.Round(h.Confidence * 100)),
			Rationale:  h.Rationale,
		})
	}
	return out
}

// SetupWithManager sets up the controller with the Manager.
func (r *FindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		// GetEventRecorderFor is deprecated in favour of GetEventRecorder (the events.k8s.io/v1
		// API), but controller-runtime's own internal.go still calls it internally with the same
		// suppression - the old client-go record.EventRecorder is what every reference
		// controller in the ecosystem still uses, and migrating buys nothing right now against a
		// "not yet removed" warning. Worth revisiting if it's ever actually scheduled for
		// removal.
		r.Recorder = mgr.GetEventRecorderFor("candor") //nolint:staticcheck
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&candorv1alpha1.Finding{}).
		Named("finding").
		Complete(r)
}
