package signal

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
)

const (
	// A local constant, not internal/provider/trivy's ProviderName - signal is provider-agnostic,
	// and importing trivy here would cycle back (trivy imports signal). Ingest doesn't care what
	// the provider is called; these tests just need a consistent, arbitrary value.
	testProvider  = "trivy"
	testNamespace = "team-a"
	testKind      = "Deployment"
	testResource  = "api"
	testRefKind   = "VulnerabilityReport"
	testRefName   = "api-abc123"
)

func newFakeClient(t *testing.T, objs ...client.Object) (client.Client, *runtime.Scheme) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&candorv1alpha1.Finding{}).Build()
	return c, scheme
}

// policy builds a SignalPolicy in testNamespace - every test in this file exercises the same
// namespace, so it isn't a parameter.
func policy(name string, providers []string, minSeverity string) *candorv1alpha1.SignalPolicy {
	return &candorv1alpha1.SignalPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       candorv1alpha1.SignalPolicySpec{Providers: providers, MinSeverity: minSeverity},
	}
}

func testSignal(severity string) Signal {
	return Signal{
		Provider: testProvider, Severity: severity, Namespace: testNamespace,
		Kind: testKind, Name: testResource, RefKind: testRefKind, RefName: testRefName,
	}
}

func TestIngest_NoPolicy_Filtered(t *testing.T) {
	c, scheme := newFakeClient(t)

	result, err := Ingest(context.Background(), c, scheme, testSignal(SeverityCritical), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultFiltered {
		t.Errorf("result = %q, want %q (no SignalPolicy in namespace)", result, ResultFiltered)
	}
}

func TestIngest_BelowThreshold_Filtered(t *testing.T) {
	c, scheme := newFakeClient(t, policy("policy", []string{testProvider}, SeverityCritical))

	result, err := Ingest(context.Background(), c, scheme, testSignal(SeverityHigh), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultFiltered {
		t.Errorf("result = %q, want %q (HIGH below policy's CRITICAL threshold)", result, ResultFiltered)
	}
}

func TestIngest_WrongProvider_Filtered(t *testing.T) {
	c, scheme := newFakeClient(t, policy("policy", []string{"falco"}, SeverityLow))

	result, err := Ingest(context.Background(), c, scheme, testSignal(SeverityCritical), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultFiltered {
		t.Errorf("result = %q, want %q (policy doesn't enable trivy)", result, ResultFiltered)
	}
}

func TestIngest_Creates(t *testing.T) {
	c, scheme := newFakeClient(t, policy("policy", []string{testProvider}, SeverityHigh))
	sig := testSignal(SeverityCritical)
	sig.Summary = "3 CRITICAL vulns"

	result, err := Ingest(context.Background(), c, scheme, sig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultCreated {
		t.Fatalf("result = %q, want %q", result, ResultCreated)
	}

	findings := &candorv1alpha1.FindingList{}
	if err := c.List(context.Background(), findings); err != nil {
		t.Fatal(err)
	}
	if len(findings.Items) != 1 {
		t.Fatalf("got %d Findings, want 1", len(findings.Items))
	}
	f := findings.Items[0]
	if f.Spec.Severity != SeverityCritical || f.Spec.Summary != "3 CRITICAL vulns" || f.Spec.Source.Provider != testProvider {
		t.Errorf("Finding spec = %+v, unexpected content", f.Spec)
	}
}

func TestIngest_SameSignalTwice_UpdatesNotDuplicates(t *testing.T) {
	c, scheme := newFakeClient(t, policy("policy", []string{testProvider}, SeverityLow))
	sig := testSignal(SeverityHigh)
	sig.Summary = "first"

	if _, err := Ingest(context.Background(), c, scheme, sig, nil); err != nil {
		t.Fatal(err)
	}

	sig.Summary = "updated"
	result, err := Ingest(context.Background(), c, scheme, sig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultUpdated {
		t.Errorf("result = %q, want %q", result, ResultUpdated)
	}

	findings := &candorv1alpha1.FindingList{}
	if err := c.List(context.Background(), findings); err != nil {
		t.Fatal(err)
	}
	if len(findings.Items) != 1 {
		t.Fatalf("got %d Findings for the same signal ingested twice, want 1 (re-ingest must update, not duplicate)", len(findings.Items))
	}
	if findings.Items[0].Spec.Summary != "updated" {
		t.Errorf("Summary = %q, want %q", findings.Items[0].Spec.Summary, "updated")
	}
}

func TestIngest_MultiplePolicies_AnyMatchAccepts(t *testing.T) {
	// A LOW-threshold policy for another provider must not block a HIGH-threshold trivy policy
	// from accepting a signal that clears it.
	c, scheme := newFakeClient(t,
		policy("policy1", []string{"falco"}, SeverityLow),
		policy("policy2", []string{testProvider}, SeverityHigh),
	)

	result, err := Ingest(context.Background(), c, scheme, testSignal(SeverityCritical), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != ResultCreated {
		t.Errorf("result = %q, want %q", result, ResultCreated)
	}
}
