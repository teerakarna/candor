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
}

// SignalPolicyStatus defines the observed state of SignalPolicy.
type SignalPolicyStatus struct {
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
