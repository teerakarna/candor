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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SignalPolicySpec defines the desired state of SignalPolicy.
//
// A SignalPolicy governs signals in its own namespace only - the same convention as
// ResourceQuota, LimitRange, and NetworkPolicy. There is no cross-namespace targeting field;
// a team opts a namespace in by creating a SignalPolicy inside it, and RBAC on SignalPolicy
// itself controls who can do that.
type SignalPolicySpec struct {
	// providers this policy enables, e.g. ["trivy"]. Deliberately a free-form string list rather
	// than a closed CRD enum: each provider's own reconciler simply ignores policies that don't
	// name it, so adding a new provider later never requires a schema migration here.
	// +kubebuilder:validation:MinItems=1
	Providers []string `json:"providers"`

	// minSeverity is the minimum signal severity that produces a Finding. Signals below this
	// threshold are ignored entirely - they never reach a Finding at all, so they can't
	// contribute noise even before suppression exists (see docs/design.md pillar 3).
	// +kubebuilder:validation:Enum=LOW;MEDIUM;HIGH;CRITICAL
	// +kubebuilder:default=HIGH
	// +optional
	MinSeverity string `json:"minSeverity,omitempty"`

	// budget bounds LLM enrichment spend for this policy's namespace over a rolling window.
	// Optional - omitted means unlimited, relying entirely on the fingerprint gate (slice 3) to
	// bound cost. When the limit is reached, enrichment degrades to deterministic-only findings
	// until the window resets - it never silently keeps spending past the ceiling.
	// +optional
	Budget *Budget `json:"budget,omitempty"`
}

// Budget bounds LLM enrichment calls over a rolling window.
type Budget struct {
	// maxCalls is the maximum number of LLM enrichment calls allowed within one window.
	// +kubebuilder:validation:Minimum=1
	// +required
	MaxCalls int32 `json:"maxCalls"`

	// windowSeconds is the rolling window length. Rolling from whenever the window last reset,
	// not calendar-aligned - simpler to reason about, no timezone/cron edge cases.
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:default=86400
	// +optional
	WindowSeconds int32 `json:"windowSeconds,omitempty"`
}

// SignalPolicyStatus defines the observed state of SignalPolicy.
type SignalPolicyStatus struct {
	// budgetWindowStart is when the current budget window began. Unset means no window is active
	// yet - no enrichment call has been checked against this policy's budget since the last reset.
	// +optional
	BudgetWindowStart *metav1.Time `json:"budgetWindowStart,omitempty"`

	// budgetCallsUsed is the number of LLM calls made within the current budget window.
	// +optional
	BudgetCallsUsed int32 `json:"budgetCallsUsed,omitempty"`

	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the SignalPolicy resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Providers",type=string,JSONPath=".spec.providers"
// +kubebuilder:printcolumn:name="MinSeverity",type=string,JSONPath=".spec.minSeverity"
// +kubebuilder:printcolumn:name="BudgetUsed",type=integer,JSONPath=".status.budgetCallsUsed"
// +kubebuilder:printcolumn:name="BudgetMax",type=integer,JSONPath=".spec.budget.maxCalls"

// SignalPolicy is the Schema for the signalpolicies API
type SignalPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SignalPolicy
	// +required
	Spec SignalPolicySpec `json:"spec"`

	// status defines the observed state of SignalPolicy
	// +optional
	Status SignalPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SignalPolicyList contains a list of SignalPolicy
type SignalPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SignalPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &SignalPolicy{}, &SignalPolicyList{})
		return nil
	})
}
