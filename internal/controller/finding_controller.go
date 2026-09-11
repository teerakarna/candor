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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/llm"
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
}

// +kubebuilder:rbac:groups=candor.dev,resources=findings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=candor.dev,resources=findings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=candor.dev,resources=findings/finalizers,verbs=update

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
		return ctrl.Result{}, nil
	}

	resp, err := r.LLM.Enrich(ctx, llm.Request{
		Provider: finding.Spec.Source.Provider,
		Severity: finding.Spec.Severity,
		Summary:  finding.Spec.Summary,
		Kind:     finding.Spec.Source.Kind,
		Name:     finding.Spec.Source.Name,
	})
	if err != nil {
		// Returned as-is: controller-runtime requeues failed reconciles with its own
		// exponential backoff, so a transient API error (rate limit, timeout) retries on its
		// own without this needing to hand-roll that.
		log.Error(err, "LLM enrichment failed")
		return ctrl.Result{}, err
	}

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
	return ctrl.NewControllerManagedBy(mgr).
		For(&candorv1alpha1.Finding{}).
		Named("finding").
		Complete(r)
}
