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

// OperatingPolicySpec defines the desired state of OperatingPolicy.
//
// OperatingPolicy is Candor's one cluster-scoped CRD, deliberately singular: every other CRD here
// (SignalPolicy, Finding, Suppression) is namespaced, because Candor's whole model is a team opts
// their own namespace in. The action path's brakes are the one exception, because a per-namespace
// limit alone can't bound what many namespaces do collectively (docs/design.md:189, "a global
// rate limit across all namespaces, so one noisy provider cannot exhaust the whole cluster's
// allowance") - that needs exactly one object visible cluster-wide, not one per namespace.
//
// It's also where the other cluster-wide brake docs/design.md:189 requires lives: Mode is the
// panic switch, "reachable by editing a CRD, not by redeploying" - see that field's doc comment.
//
// Exactly one OperatingPolicy is meaningful, named "default" by convention
// (config/samples/candor_v1alpha1_operatingpolicy.yaml). internal/signal.FindOperatingPolicy reads
// it; OperatingPolicyReconciler flags a second object as a misconfiguration (Ready=False) rather
// than erroring or silently picking one - the same "quiet failure is the failure mode to avoid"
// stance SignalPolicyReconciler already takes for an unrecognised provider. No object at all is a
// fully supported configuration, not an error: internal/signal.CheckGlobalPullRequestBudget
// applies a conservative built-in default in that case, the same "never uncapped by omission"
// rule PullRequestBudget already follows - see that function's doc comment for the default value.
type OperatingPolicySpec struct {
	// pullRequestRateLimit bounds ProposePullRequest actions across every namespace in the
	// cluster, over a rolling window - independent of, and enforced in addition to, each
	// namespace's own SignalPolicy.spec.pullRequestBudget. A namespace under its own budget can
	// still be refused if this cluster-wide ceiling is exhausted.
	// +optional
	PullRequestRateLimit *PullRequestRateLimit `json:"pullRequestRateLimit,omitempty"`

	// mode is the panic switch docs/design.md:189 requires: "reachable by editing a CRD, not by
	// redeploying". OperatingModeAudit disables ProposePullRequest across every namespace
	// immediately - before any per-namespace or global budget is even checked, so flipping this
	// is a genuine full stop, not "still counted against the budget but not executed". Findings
	// that would have triggered a pull request are simply not acted on; nothing else about
	// Candor (enrichment, notification, verification) is affected - this is a brake on the
	// action path specifically, not a global kill switch for the whole operator.
	// +kubebuilder:validation:Enum=Active;Audit
	// +kubebuilder:default=Active
	// +optional
	Mode string `json:"mode,omitempty"`
}

// OperatingPolicy's two modes. Deliberately not named to echo the future, unrelated
// Quarantine/Enforcing mode (docs/design.md: post-v1, gated on the verification loop, and about
// direct cluster mutation) - this is a narrower, already-shipped brake on ProposePullRequest only.
const (
	// OperatingModeActive is the default: ProposePullRequest proceeds normally, subject to its
	// other brakes (per-namespace and global budgets, a mechanically verified fix).
	OperatingModeActive = "Active"

	// OperatingModeAudit is the panic switch's target state: ProposePullRequest is disabled
	// cluster-wide until Mode is changed back.
	OperatingModeAudit = "Audit"
)

// PullRequestRateLimit bounds ProposePullRequest actions across the whole cluster over a rolling
// window - the global counterpart to the per-namespace PullRequestBudget.
type PullRequestRateLimit struct {
	// maxPullRequests is the maximum number of pull requests ProposePullRequest may open across
	// every namespace within one window.
	// +kubebuilder:validation:Minimum=1
	// +required
	MaxPullRequests int32 `json:"maxPullRequests"`

	// windowSeconds is the rolling window length. Rolling from whenever the window last reset,
	// not calendar-aligned - matches PullRequestBudget.WindowSeconds for the same reason.
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:default=86400
	// +optional
	WindowSeconds int32 `json:"windowSeconds,omitempty"`
}

// OperatingPolicyStatus defines the observed state of OperatingPolicy.
type OperatingPolicyStatus struct {
	// pullRequestWindowStart is when the current global pull request window began. Unset means no
	// window is active yet - mirrors SignalPolicyStatus.PullRequestWindowStart for the same reason.
	// +optional
	PullRequestWindowStart *metav1.Time `json:"pullRequestWindowStart,omitempty"`

	// pullRequestsOpened is the number of pull requests ProposePullRequest has opened across every
	// namespace within the current window.
	// +optional
	PullRequestsOpened int32 `json:"pullRequestsOpened,omitempty"`

	// observedMode is the Mode value OperatingPolicyReconciler last saw, kept only to detect a
	// transition worth an Event (see that reconciler) - not itself the visibility mechanism.
	// spec.mode is always the source of truth for what mode Candor is actually in; read that, not
	// this.
	// +optional
	ObservedMode string `json:"observedMode,omitempty"`

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the OperatingPolicy resource. "Ready"=False with
	// reason "MultipleObjects" means more than one OperatingPolicy exists in the cluster - see
	// this type's doc comment for why only one is meaningful.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=".spec.mode"
// +kubebuilder:printcolumn:name="PRsOpened",type=integer,JSONPath=".status.pullRequestsOpened"
// +kubebuilder:printcolumn:name="PRsMax",type=integer,JSONPath=".spec.pullRequestRateLimit.maxPullRequests"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type==\"Ready\")].status"

// OperatingPolicy is the Schema for the operatingpolicies API
type OperatingPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OperatingPolicy
	// +required
	Spec OperatingPolicySpec `json:"spec"`

	// status defines the observed state of OperatingPolicy
	// +optional
	Status OperatingPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OperatingPolicyList contains a list of OperatingPolicy
type OperatingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OperatingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &OperatingPolicy{}, &OperatingPolicyList{})
		return nil
	})
}
