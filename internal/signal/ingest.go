package signal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/metrics"
	"github.com/teerakarna/candor/internal/notify"
)

// Result reports what Ingest did, so callers (and tests) can observe the outcome without Ingest
// depending on a particular logger.
type Result string

const (
	// ResultCreated means a new Finding was created.
	ResultCreated Result = "created"
	// ResultUpdated means an existing Finding was brought up to date.
	ResultUpdated Result = "updated"
	// ResultUnchanged means an existing Finding already matched - no write happened.
	ResultUnchanged Result = "unchanged"
	// ResultFiltered means no Finding was produced: either no SignalPolicy in the signal's
	// namespace enables this provider, or the signal's severity is below every policy that does.
	// No Finding previously existed for this source either - there was nothing to resolve.
	ResultFiltered Result = "filtered"
	// ResultResolved means the signal was filtered (same reasons as ResultFiltered), but a
	// previously-open Finding existed for this exact source - it has been marked Resolved rather
	// than left stale. See Finding.Status.VerificationOutcome.
	ResultResolved Result = "resolved"
)

// Ingest is the single path from a Signal to a Finding. It looks up whether any SignalPolicy in
// the signal's own namespace opts into this provider at this severity (SignalPolicy governs
// signals in its own namespace only - same convention as ResourceQuota/NetworkPolicy, not a
// separate namespace-selector field), and if so, creates or updates the corresponding Finding.
//
// owner becomes the Finding's owner reference (e.g. the originating VulnerabilityReport), so the
// Finding is garbage-collected automatically when its source signal object is deleted.
func Ingest(ctx context.Context, c client.Client, scheme *runtime.Scheme, sig Signal, owner client.Object) (Result, error) {
	policies := &candorv1alpha1.SignalPolicyList{}
	if err := c.List(ctx, policies, client.InNamespace(sig.Namespace)); err != nil {
		return "", fmt.Errorf("listing SignalPolicies in namespace %s: %w", sig.Namespace, err)
	}

	if !anyPolicyAccepts(policies.Items, sig) {
		resolvedFinding, err := resolveIfOpen(ctx, c, sig.Namespace, findingName(sig))
		if err != nil {
			return "", err
		}
		if resolvedFinding != nil {
			if webhook := webhookFor(policies.Items, sig.Provider); webhook != nil {
				notifyEvent(ctx, webhook.URL, notify.KindFindingResolved, resolvedFinding)
			}
			return ResultResolved, nil
		}
		return ResultFiltered, nil
	}

	finding := &candorv1alpha1.Finding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      findingName(sig),
			Namespace: sig.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, c, finding, func() error {
		finding.Spec = candorv1alpha1.FindingSpec{
			Source: candorv1alpha1.FindingSource{
				Provider: sig.Provider,
				Kind:     sig.Kind,
				Name:     sig.Name,
				RefKind:  sig.RefKind,
				RefName:  sig.RefName,
			},
			Severity: sig.Severity,
			Summary:  sig.Summary,
		}
		if owner != nil {
			return controllerutil.SetOwnerReference(owner, finding, scheme)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("upserting Finding %s/%s: %w", sig.Namespace, finding.Name, err)
	}

	// Fingerprint and VerificationOutcome are status fields, and status is a separate subresource
	// on Finding - the CreateOrUpdate above never persists them, so they need their own write.
	// Skipped entirely when neither actually changes, both to avoid a needless resourceVersion
	// bump on every reconcile of truly unchanged content, and because that "no write for no
	// change" property is the same cost-consciousness this whole mechanism enforces on the LLM
	// call Fingerprint gates.
	//
	// previousOutcome is read here, before either field is overwritten below - finding.Status is
	// whatever CreateOrUpdate fetched from the API (its mutate closure above only touched .Spec),
	// so on a brand new Finding this is the zero value, and on an existing one it's the outcome
	// from the last time Ingest ran.
	previousOutcome := finding.Status.VerificationOutcome
	newOutcome := VerificationStillPresent
	if previousOutcome == VerificationResolved {
		newOutcome = VerificationRecurred
	}

	fp := Fingerprint(sig)
	fingerprintChanged := finding.Status.Fingerprint != fp
	outcomeChanged := finding.Status.VerificationOutcome != newOutcome

	if fingerprintChanged {
		finding.Status.Fingerprint = fp
	}
	if outcomeChanged {
		finding.Status.VerificationOutcome = newOutcome
		metrics.VerificationTransitionsTotal.WithLabelValues(verificationMetricLabel[newOutcome]).Inc()
	}
	if fingerprintChanged || outcomeChanged {
		if err := c.Status().Update(ctx, finding); err != nil {
			return "", fmt.Errorf("updating status on Finding %s/%s: %w", sig.Namespace, finding.Name, err)
		}
	}

	var finalResult Result
	switch result {
	case controllerutil.OperationResultCreated:
		finalResult = ResultCreated
	case controllerutil.OperationResultUpdated:
		finalResult = ResultUpdated
	default:
		finalResult = ResultUnchanged
	}

	// Notify on a brand new Finding, or on an existing one recurring - not on every routine
	// content refresh (ResultUpdated with no outcome change) or unchanged reconcile, which would
	// just be noise.
	if webhook := webhookFor(policies.Items, sig.Provider); webhook != nil {
		switch {
		case finalResult == ResultCreated:
			notifyEvent(ctx, webhook.URL, notify.KindFindingCreated, finding)
		case outcomeChanged && newOutcome == VerificationRecurred:
			notifyEvent(ctx, webhook.URL, notify.KindFindingRecurred, finding)
		}
	}

	return finalResult, nil
}

// webhookFor returns the Webhook configured by the first policy in policies that enables
// provider, or nil if none is configured. Reuses the policies slice Ingest already fetched,
// rather than issuing a second List - the same "first policy for this provider governs shared
// config" convention CheckBudget's caller already relies on for Budget.
func webhookFor(policies []candorv1alpha1.SignalPolicy, provider string) *candorv1alpha1.Webhook {
	for i := range policies {
		if providerEnabled(policies[i].Spec.Providers, provider) && policies[i].Spec.Webhook != nil {
			return policies[i].Spec.Webhook
		}
	}
	return nil
}

// notifyEvent sends a Finding notification best-effort: a failure is logged and counted, never
// returned as an error - a broken webhook endpoint must not make Finding reconciliation fail.
func notifyEvent(ctx context.Context, url, kind string, f *candorv1alpha1.Finding) {
	event := notify.Event{
		Kind: kind, Namespace: f.Namespace, Finding: f.Name,
		Severity: f.Spec.Severity, Summary: f.Spec.Summary,
	}
	if err := notify.Send(ctx, url, event); err != nil {
		metrics.WebhookSendsTotal.WithLabelValues("error").Inc()
		logf.FromContext(ctx).Error(err, "sending webhook notification", "kind", kind, "finding", f.Name)
		return
	}
	metrics.WebhookSendsTotal.WithLabelValues("success").Inc()
}

// anyPolicyAccepts reports whether at least one SignalPolicy enables sig.Provider at a
// MinSeverity the signal clears. If multiple policies in a namespace enable the same provider at
// different thresholds, the signal only needs to clear one of them - each policy is an
// independent opt-in, not a combined gate.
func anyPolicyAccepts(policies []candorv1alpha1.SignalPolicy, sig Signal) bool {
	for _, p := range policies {
		if !providerEnabled(p.Spec.Providers, sig.Provider) {
			continue
		}
		threshold := p.Spec.MinSeverity
		if threshold == "" {
			threshold = SeverityHigh // matches the CRD's +kubebuilder:default
		}
		if AtLeast(sig.Severity, threshold) {
			return true
		}
	}
	return false
}

func providerEnabled(providers []string, provider string) bool {
	return slices.Contains(providers, provider)
}

// verificationMetricLabel maps a VerificationOutcome constant (PascalCase, matching Kubernetes API
// enum convention) to the snake_case label VerificationTransitionsTotal uses (matching every other
// Prometheus label in this codebase).
var verificationMetricLabel = map[string]string{
	VerificationStillPresent: "still_present",
	VerificationResolved:     "resolved",
	VerificationRecurred:     "recurred",
}

// resolveIfOpen marks the Finding at namespace/name Resolved, if one exists and isn't already,
// and returns it (for the caller to notify with) - or nil, nil as a no-op (not an error) in the
// common case: most filtered signals never had a Finding to begin with, and a Finding already
// Resolved doesn't need writing, or reporting, again. Called when a signal is filtered (no
// SignalPolicy accepts it - either none opts into this provider, or its severity fell below every
// threshold that does), so a Finding whose underlying condition has genuinely gone away doesn't
// sit there stale forever showing the old severity.
func resolveIfOpen(ctx context.Context, c client.Client, namespace, name string) (*candorv1alpha1.Finding, error) {
	finding := &candorv1alpha1.Finding{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, finding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting Finding %s/%s: %w", namespace, name, err)
	}
	if finding.Status.VerificationOutcome == VerificationResolved {
		return nil, nil
	}

	finding.Status.VerificationOutcome = VerificationResolved
	if err := c.Status().Update(ctx, finding); err != nil {
		return nil, fmt.Errorf("resolving Finding %s/%s: %w", namespace, name, err)
	}
	metrics.VerificationTransitionsTotal.WithLabelValues(verificationMetricLabel[VerificationResolved]).Inc()
	return finding, nil
}

// findingName derives a stable Finding object name from the signal's source identity (not its
// content), so re-ingesting from the same source updates one object instead of creating
// duplicates as its content changes over time. This is deliberately not the content-addressed
// fingerprint from docs/design.md pillar 2 - that's Fingerprint, stored on Status (see
// fingerprint.go), and it changes when content changes precisely so it can gate enrichment and
// let a suppressed Finding resurface. Object identity and content fingerprint answer different
// questions on purpose: "which Finding is this" vs. "has this exact content been seen before."
func findingName(sig Signal) string {
	h := sha256.Sum256([]byte(sig.Provider + "/" + sig.RefKind + "/" + sig.RefName))
	return fmt.Sprintf("%s-%s", sig.Provider, hex.EncodeToString(h[:])[:12])
}
