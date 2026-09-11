package controller

import (
	"context"
	"sync/atomic"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/llm"
)

// testProvider is shared across this package's test files (finding_controller_test.go,
// signalpolicy_controller_test.go too) purely to avoid repeating the literal - it isn't
// internal/provider/trivy.ProviderName, importing that here would be a real (if harmless) coupling
// these tests don't need.
const testProvider = "trivy"

// countingLLM is a fake llm.Client that counts calls and returns a fixed response - enough to
// prove the gate around it, without needing a real API key or network access in CI.
type countingLLM struct {
	calls atomic.Int32
	resp  llm.Response
	err   error
}

func (f *countingLLM) Enrich(ctx context.Context, req llm.Request) (llm.Response, error) {
	f.calls.Add(1)
	return f.resp, f.err
}

func newTestFinding(name string) *candorv1alpha1.Finding {
	return &candorv1alpha1.Finding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: candorv1alpha1.FindingSpec{
			Source: candorv1alpha1.FindingSource{
				Provider: testProvider, Kind: "Deployment", Name: "api",
				RefKind: "VulnerabilityReport", RefName: "api-report",
			},
			Severity: "CRITICAL",
			Summary:  "3 critical vulns",
		},
		// Set by internal/signal.Ingest in production; set directly here since these tests
		// exercise FindingReconciler in isolation from Ingest.
		Status: candorv1alpha1.FindingStatus{Fingerprint: "fp-1"},
	}
}

func TestFindingReconciler_NoLLMConfigured_NoOp(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	finding := newTestFinding("f1")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(finding).WithStatusSubresource(&candorv1alpha1.Finding{}).Build()

	r := &FindingReconciler{Client: c, Scheme: scheme, LLM: nil}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: namespacedName(finding)})
	if err != nil {
		t.Fatal(err)
	}
	// No assertion beyond "didn't error" - there's no LLM to have been called, which is the point.
}

func TestFindingReconciler_NeedsEnrichment_CallsLLMOnce(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	finding := newTestFinding("f1")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(finding).WithStatusSubresource(&candorv1alpha1.Finding{}).Build()

	fakeLLM := &countingLLM{resp: llm.Response{Hypotheses: []llm.Hypothesis{
		{Cause: "outdated base image", Confidence: 0.82, Rationale: "known CVE fixed upstream"},
	}}}
	r := &FindingReconciler{Client: c, Scheme: scheme, LLM: fakeLLM}

	ctx := context.Background()
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(finding)}); err != nil {
		t.Fatal(err)
	}

	if got := fakeLLM.calls.Load(); got != 1 {
		t.Fatalf("LLM calls = %d, want 1", got)
	}

	got := &candorv1alpha1.Finding{}
	if err := c.Get(ctx, namespacedName(finding), got); err != nil {
		t.Fatal(err)
	}
	if got.Status.EnrichedFingerprint != "fp-1" {
		t.Errorf("EnrichedFingerprint = %q, want %q", got.Status.EnrichedFingerprint, "fp-1")
	}
	if len(got.Status.Hypotheses) != 1 {
		t.Fatalf("got %d hypotheses, want 1", len(got.Status.Hypotheses))
	}
	h := got.Status.Hypotheses[0]
	if h.Cause != "outdated base image" || h.Confidence != 82 || h.Rationale == "" {
		t.Errorf("hypothesis = %+v, unexpected content (want Confidence=82, the 0.82 float converted to a percentage)", h)
	}
}

// TestFindingReconciler_CostRegression is the design doc's own required check, now with a real
// (fake, but interface-real) LLM call to count: N reconciles over unchanged content must produce
// exactly one LLM call. If this regresses, Candor has the exact k8sgpt failure mode it exists to
// avoid - 9,300 calls for 164 stable findings.
func TestFindingReconciler_CostRegression(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	finding := newTestFinding("f1")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(finding).WithStatusSubresource(&candorv1alpha1.Finding{}).Build()

	fakeLLM := &countingLLM{resp: llm.Response{Hypotheses: []llm.Hypothesis{{Cause: "x", Confidence: 0.5}}}}
	r := &FindingReconciler{Client: c, Scheme: scheme, LLM: fakeLLM}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: namespacedName(finding)}

	const n = 10
	for i := range n {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}

	if got := fakeLLM.calls.Load(); got != 1 {
		t.Fatalf("LLM calls across %d reconciles of unchanged content = %d, want exactly 1", n, got)
	}

	// Now the content actually changes (a new signal ingest would update Spec + Status.Fingerprint
	// - simulated directly here since Ingest itself is tested in internal/signal).
	got := &candorv1alpha1.Finding{}
	if err := c.Get(ctx, req.NamespacedName, got); err != nil {
		t.Fatal(err)
	}
	got.Status.Fingerprint = "fp-2"
	if err := c.Status().Update(ctx, got); err != nil {
		t.Fatal(err)
	}

	for i := range n {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("post-change iteration %d: %v", i, err)
		}
	}

	if got := fakeLLM.calls.Load(); got != 2 {
		t.Fatalf("LLM calls after content changed and %d more reconciles = %d, want exactly 2 (one for each distinct fingerprint, ever)", n, got)
	}
}

func namespacedName(f *candorv1alpha1.Finding) types.NamespacedName {
	return types.NamespacedName{Namespace: f.Namespace, Name: f.Name}
}
