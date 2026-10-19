package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
)

// Shared across this file purely to avoid repeating the literals (goconst).
const (
	sevCritical = "CRITICAL"
	nsTeamA     = "team-a"
)

func finding(namespace, name, severity, outcome string) *candorv1alpha1.Finding {
	return &candorv1alpha1.Finding{
		Name: name, Namespace: namespace,
		Spec: candorv1alpha1.FindingSpec{
			Source:   candorv1alpha1.FindingSource{Provider: "trivy", Kind: "Deployment", Name: "api", RefKind: "VulnerabilityReport", RefName: "api"},
			Severity: severity,
			Summary:  "test",
		},
		Status: candorv1alpha1.FindingStatus{VerificationOutcome: outcome},
	}
}

func TestFindingsCollector_GroupsByNamespaceSeverityOutcome(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		finding(nsTeamA, "f1", sevCritical, "StillPresent"),
		finding(nsTeamA, "f2", sevCritical, "StillPresent"),
		finding(nsTeamA, "f3", "HIGH", "Resolved"),
		finding("team-b", "f4", sevCritical, "Recurred"),
	).Build()

	collector := &FindingsCollector{Reader: c}
	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		namespace, severity, outcome string
		want                         float64
	}{
		{nsTeamA, sevCritical, "StillPresent", 2},
		{nsTeamA, "HIGH", "Resolved", 1},
		{"team-b", sevCritical, "Recurred", 1},
	}
	for _, c := range cases {
		got := findGauge(t, registry, c.namespace, c.severity, c.outcome)
		if got != c.want {
			t.Errorf("candor_findings_current{namespace=%q,severity=%q,outcome=%q} = %v, want %v", c.namespace, c.severity, c.outcome, got, c.want)
		}
	}
}

func TestFindingsCollector_EmptyOutcome_LabeledUnverified(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		finding(nsTeamA, "f1", "HIGH", ""),
	).Build()

	collector := &FindingsCollector{Reader: c}
	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}

	got := findGauge(t, registry, nsTeamA, "HIGH", unverifiedOutcome)
	if got != 1 {
		t.Errorf("candor_findings_current{...,outcome=Unverified} = %v, want 1", got)
	}
}

func TestFindingsCollector_NoFindings_NoMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	collector := &FindingsCollector{Reader: c}
	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == "candor_findings_current" && len(f.GetMetric()) != 0 {
			t.Errorf("expected no candor_findings_current samples with zero Findings, got %d", len(f.GetMetric()))
		}
	}
}

// findGauge gathers registry and returns the value of the one candor_findings_current sample
// matching the given labels, failing the test if there isn't exactly one.
func findGauge(t *testing.T, registry *prometheus.Registry, namespace, severity, outcome string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "candor_findings_current" {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["namespace"] == namespace && labels["severity"] == severity && labels["outcome"] == outcome {
				return m.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("no candor_findings_current sample found for namespace=%q severity=%q outcome=%q", namespace, severity, outcome)
	return 0
}

func withPR(f *candorv1alpha1.Finding, url string) *candorv1alpha1.Finding {
	f.Status.ProposedPullRequestURL = url
	return f
}

// prInfoSamples gathers registry and returns each candor_finding_pull_request_info sample as a
// label map, so a test can assert on exactly which series exist.
func prInfoSamples(t *testing.T, registry *prometheus.Registry) []map[string]string {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]string
	for _, f := range families {
		if f.GetName() != "candor_finding_pull_request_info" {
			continue
		}
		for _, m := range f.GetMetric() {
			if m.GetGauge().GetValue() != 1 {
				t.Errorf("candor_finding_pull_request_info value = %v, want 1", m.GetGauge().GetValue())
			}
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			out = append(out, labels)
		}
	}
	return out
}

func TestFindingsCollector_PullRequestInfo_OnlyOpenFindingsWithAURL(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		withPR(finding(nsTeamA, "open-crit", sevCritical, "StillPresent"), "https://example.com/pull/41"),
		withPR(finding(nsTeamA, "recurred", "HIGH", "Recurred"), "https://example.com/pull/42"),
		withPR(finding("team-b", "unverified", "LOW", ""), "https://example.com/pull/43"),
		finding("team-b", "no-pr", "MEDIUM", "StillPresent"),
		withPR(finding("team-b", "resolved", "LOW", "Resolved"), "https://example.com/pull/17"),
	).Build()

	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(&FindingsCollector{Reader: c}); err != nil {
		t.Fatal(err)
	}

	got := map[string]map[string]string{}
	for _, s := range prInfoSamples(t, registry) {
		got[s[labelFinding]] = s
	}
	if len(got) != 3 {
		t.Fatalf("got %d pull request series (%v), want 3: open-crit, recurred, unverified", len(got), got)
	}
	want := map[string]map[string]string{
		"open-crit":  {labelNamespace: nsTeamA, labelSeverity: sevCritical, labelURL: "https://example.com/pull/41"},
		"recurred":   {labelNamespace: nsTeamA, labelSeverity: "HIGH", labelURL: "https://example.com/pull/42"},
		"unverified": {labelNamespace: "team-b", labelSeverity: "LOW", labelURL: "https://example.com/pull/43"},
	}
	for name, labels := range want {
		for k, v := range labels {
			if got[name][k] != v {
				t.Errorf("%s: label %s = %q, want %q", name, k, got[name][k], v)
			}
		}
	}
	for _, absent := range []string{"no-pr", "resolved"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s should not have a pull request series", absent)
		}
	}
}

func TestFindingsCollector_PullRequestInfo_NoFindings_NoSeries(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := candorv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(&FindingsCollector{Reader: c}); err != nil {
		t.Fatal(err)
	}
	if got := prInfoSamples(t, registry); len(got) != 0 {
		t.Errorf("got %d series with zero Findings, want 0", len(got))
	}
}
