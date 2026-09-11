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

// FindingSpec defines the desired state of Finding.
//
// This is deliberately the deterministic slice of what a Finding will eventually carry (see
// docs/design.md: "Finding ... ranked hypotheses, confidence, verification outcome"). No LLM is
// involved yet - Summary is generated straight from the signal's own data. Fields for
// hypotheses/confidence and verification outcome are added in later slices rather than reserved
// here now, so the schema only ever describes what's actually implemented. Fingerprinting
// (docs/design.md pillar 2) is on FindingStatus, not here - see that type's doc comment for why.
type FindingSpec struct {
	// source identifies the signal that produced this Finding.
	// +required
	Source FindingSource `json:"source"`

	// severity of the underlying signal - the raw signal severity, not an LLM judgement.
	// +kubebuilder:validation:Enum=LOW;MEDIUM;HIGH;CRITICAL
	// +required
	Severity string `json:"severity"`

	// summary is a short, deterministic, human-readable description generated from the signal's
	// own data.
	// +required
	Summary string `json:"summary"`
}

// FindingSource identifies where a Finding came from.
type FindingSource struct {
	// provider that produced this finding, e.g. "trivy".
	// +required
	Provider string `json:"provider"`

	// kind, name identify the Kubernetes resource this finding is about - the workload the
	// provider examined, not the signal object itself.
	// +required
	Kind string `json:"kind"`
	// +required
	Name string `json:"name"`

	// refKind, refName identify the object the signal was read from (e.g. a Trivy
	// VulnerabilityReport), for traceability back to raw evidence. Same namespace as the Finding.
	// +required
	RefKind string `json:"refKind"`
	// +required
	RefName string `json:"refName"`
}

// FindingStatus defines the observed state of Finding.
type FindingStatus struct {
	// fingerprint is a content-addressed hash of this Finding's current spec, recomputed every
	// time Spec is written (see internal/signal.Fingerprint). It is deliberately not this
	// object's identity/name - the same source (e.g. a VulnerabilityReport) keeps updating the
	// same Finding as its content changes over time, and Fingerprint is how that changing
	// content gets tracked without churning object identity.
	// +optional
	Fingerprint string `json:"fingerprint,omitempty"`

	// enrichedFingerprint is the Fingerprint value that was last successfully enriched by an LLM.
	// Empty means never enriched. Written by the enrichment reconciler (a later slice - nothing
	// sets this yet), never by the code that writes Spec/Fingerprint. When it differs from
	// Fingerprint, enrichment is needed - see internal/signal.NeedsEnrichment.
	// +optional
	EnrichedFingerprint string `json:"enrichedFingerprint,omitempty"`

	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Finding resource.
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
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=".spec.severity"
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=".spec.source.provider"
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=".spec.source.name"
// +kubebuilder:printcolumn:name="Summary",type=string,JSONPath=".spec.summary"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Finding is the Schema for the findings API
type Finding struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Finding
	// +required
	Spec FindingSpec `json:"spec"`

	// status defines the observed state of Finding
	// +optional
	Status FindingStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// FindingList contains a list of Finding
type FindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Finding `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &Finding{}, &FindingList{})
		return nil
	})
}
