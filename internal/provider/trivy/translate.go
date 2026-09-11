// Package trivy is the first signal provider: it watches Trivy Operator's VulnerabilityReport
// custom resources and translates them into signal.Signal values. It does not import Trivy
// Operator's Go module - VulnerabilityReport is read as unstructured data, matching the design
// doc's provider pattern ("consumes native Kubernetes Custom Resources") and keeping this
// provider decoupled from any particular Trivy Operator version.
//
// Schema reference: the fields read here (report.summary.*, report.artifact.*, and the
// trivy-operator.resource.* labels) are pinned against the real upstream CRD and source at
// https://github.com/aquasecurity/trivy-operator (deploy/helm/crds/aquasecurity.github.io_vulnerabilityreports.yaml
// and pkg/trivyoperator/constants.go) - not guessed. A copy of that CRD lives at
// test/crd/aquasecurity.github.io_vulnerabilityreports.yaml for envtest to validate against.
package trivy

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/teerakarna/candor/internal/signal"
)

// ProviderName is how this provider identifies itself in Signal.Provider and how a SignalPolicy
// enables it (SignalPolicySpec.Providers).
const ProviderName = "trivy"

// GroupVersionKind of the object this provider watches.
var GroupVersionKind = schema.GroupVersionKind{
	Group:   "aquasecurity.github.io",
	Version: "v1alpha1",
	Kind:    "VulnerabilityReport",
}

// Real, pinned label keys Trivy Operator sets on every VulnerabilityReport, linking it back to
// the workload it scanned. See pkg/trivyoperator/constants.go in aquasecurity/trivy-operator.
const (
	labelResourceKind = "trivy-operator.resource.kind"
	labelResourceName = "trivy-operator.resource.name"
)

// translate reads a VulnerabilityReport and produces a Signal, or ok=false if there's nothing
// worth reporting on - either the report isn't linked to a workload (no resource labels), or it
// found zero vulnerabilities at every severity.
func translate(u *unstructured.Unstructured) (sig signal.Signal, ok bool, err error) {
	labels := u.GetLabels()
	resourceKind := labels[labelResourceKind]
	resourceName := labels[labelResourceName]
	if resourceKind == "" || resourceName == "" {
		return signal.Signal{}, false, nil
	}

	counts, found, err := unstructured.NestedMap(u.Object, "report", "summary")
	if err != nil {
		return signal.Signal{}, false, fmt.Errorf("reading report.summary: %w", err)
	}
	if !found {
		return signal.Signal{}, false, nil
	}

	critical := intCount(counts, "criticalCount")
	high := intCount(counts, "highCount")
	medium := intCount(counts, "mediumCount")
	low := intCount(counts, "lowCount")

	severity, ok := highestNonZero(critical, high, medium, low)
	if !ok {
		return signal.Signal{}, false, nil
	}

	repository, _, _ := unstructured.NestedString(u.Object, "report", "artifact", "repository")
	tag, _, _ := unstructured.NestedString(u.Object, "report", "artifact", "tag")

	summary := fmt.Sprintf("%d critical, %d high, %d medium, %d low vulnerabilities in %s:%s",
		critical, high, medium, low, repository, tag)

	return signal.Signal{
		Provider:  ProviderName,
		Severity:  severity,
		Namespace: u.GetNamespace(),
		Kind:      resourceKind,
		Name:      resourceName,
		Summary:   summary,
		RefKind:   "VulnerabilityReport",
		RefName:   u.GetName(),
	}, true, nil
}

// intCount reads an int64 count field, tolerating the field being absent (treated as zero) since
// report.summary's own schema makes most of these fields optional-in-practice even though the
// upstream CRD marks several as required - a VulnerabilityReport with no findings at all is a
// real, valid object.
func intCount(counts map[string]any, key string) int64 {
	v, ok := counts[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// highestNonZero returns the highest severity with a non-zero count, in CRITICAL > HIGH > MEDIUM
// > LOW order, or ok=false if all four are zero.
func highestNonZero(critical, high, medium, low int64) (severity string, ok bool) {
	switch {
	case critical > 0:
		return signal.SeverityCritical, true
	case high > 0:
		return signal.SeverityHigh, true
	case medium > 0:
		return signal.SeverityMedium, true
	case low > 0:
		return signal.SeverityLow, true
	default:
		return "", false
	}
}
