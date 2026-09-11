package signal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
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
	ResultFiltered Result = "filtered"
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

	// Fingerprint is a status field, and status is a separate subresource on Finding - the
	// CreateOrUpdate above never persists it, so it needs its own write. Skipped when the value
	// is already correct, both to avoid a needless resourceVersion bump on every reconcile of
	// truly unchanged content, and because that "no write for no change" property is the same
	// cost-consciousness this whole mechanism exists to enforce on the (later) LLM call it gates.
	fp := Fingerprint(sig)
	if finding.Status.Fingerprint != fp {
		finding.Status.Fingerprint = fp
		if err := c.Status().Update(ctx, finding); err != nil {
			return "", fmt.Errorf("updating Fingerprint status on Finding %s/%s: %w", sig.Namespace, finding.Name, err)
		}
	}

	switch result {
	case controllerutil.OperationResultCreated:
		return ResultCreated, nil
	case controllerutil.OperationResultUpdated:
		return ResultUpdated, nil
	default:
		return ResultUnchanged, nil
	}
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
