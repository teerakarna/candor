package signal

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
)

func TestInAuditMode_NilPolicy_False(t *testing.T) {
	if InAuditMode(nil) {
		t.Error("InAuditMode(nil) = true, want false - no policy is a different condition (see CheckGlobalPullRequestBudget), not audit mode")
	}
}

func TestInAuditMode_ActiveMode_False(t *testing.T) {
	p := &candorv1alpha1.OperatingPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: defaultOperatingPolicyName},
		Spec:       candorv1alpha1.OperatingPolicySpec{Mode: candorv1alpha1.OperatingModeActive},
	}
	if InAuditMode(p) {
		t.Error("InAuditMode() = true for Mode=Active, want false")
	}
}

func TestInAuditMode_EmptyMode_False(t *testing.T) {
	// A zero-value Mode (an object written before CRD defaulting applied, or a raw client in a
	// test) must not be treated as audit mode - only an explicit "Audit" does.
	p := &candorv1alpha1.OperatingPolicy{ObjectMeta: metav1.ObjectMeta{Name: defaultOperatingPolicyName}}
	if InAuditMode(p) {
		t.Error("InAuditMode() = true for an empty Mode, want false")
	}
}

func TestInAuditMode_AuditMode_True(t *testing.T) {
	p := &candorv1alpha1.OperatingPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: defaultOperatingPolicyName},
		Spec:       candorv1alpha1.OperatingPolicySpec{Mode: candorv1alpha1.OperatingModeAudit},
	}
	if !InAuditMode(p) {
		t.Error("InAuditMode() = false for Mode=Audit, want true")
	}
}
