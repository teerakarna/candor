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

// SuppressionSpec defines the desired state of Suppression.
//
// A Suppression mutes exactly one Finding.status.fingerprint value - not a Finding object, not a
// resource name. This is deliberate (docs/design.md pillar 3): a fingerprint is a hash of the
// signal's actual content, so if the underlying state genuinely changes, the fingerprint changes
// with it, no longer matches this Suppression, and the Finding resurfaces on its own. There is no
// separate "resolved" or "still valid" tracking to get out of sync - matching is exact by
// construction.
type SuppressionSpec struct {
	// fingerprint is the exact Finding.status.fingerprint value to mute, in the Suppression's own
	// namespace.
	// +required
	Fingerprint string `json:"fingerprint"`

	// reason is a required human justification for muting this fingerprint - suppression is never
	// silent or unexplained.
	// +required
	Reason string `json:"reason"`

	// expiresAt is when this Suppression stops applying. Omitted means it applies indefinitely
	// (until deleted, or until the fingerprint itself changes).
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// SuppressionStatus defines the observed state of Suppression.
type SuppressionStatus struct {
	// conditions represent the current state of the Suppression resource.
	//
	// The "Expired" condition is True once expiresAt has passed - the Suppression object and its
	// Reason are kept as an audit trail rather than deleted, so this condition is how an operator
	// sees at a glance that it has lapsed without needing to compare timestamps by hand.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Fingerprint",type=string,JSONPath=".spec.fingerprint"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=".spec.reason"
// +kubebuilder:printcolumn:name="ExpiresAt",type=date,JSONPath=".spec.expiresAt"
// +kubebuilder:printcolumn:name="Expired",type=string,JSONPath=".status.conditions[?(@.type==\"Expired\")].status"

// Suppression is the Schema for the suppressions API
type Suppression struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Suppression
	// +required
	Spec SuppressionSpec `json:"spec"`

	// status defines the observed state of Suppression
	// +optional
	Status SuppressionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SuppressionList contains a list of Suppression
type SuppressionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Suppression `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Suppression{}, &SuppressionList{})
		return nil
	})
}
