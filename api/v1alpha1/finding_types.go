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
// This is deliberately the deterministic slice of what a Finding carries - Summary is generated
// straight from the signal's own data, never by an LLM. LLM-derived content (hypotheses,
// confidence) lives on FindingStatus instead (see that type's doc comment for why), and
// verification outcome is a still-later slice, added when it exists rather than reserved now.
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

// Candor's fixed action catalog (docs/design.md, "Action catalog"). A Hypothesis's
// RecommendedAction is always one of these two strings, or empty - never a free-form value
// emitted by the model and executed directly (CONTRIBUTING.md, "No free-form model-selected
// actions": model output selects from the fixed catalog, it never emits an action that gets
// executed directly).
const (
	ActionNotify             = "Notify"
	ActionProposePullRequest = "ProposePullRequest"
)

// Hypothesis is one possible explanation for a Finding, with the LLM's own confidence in it.
type Hypothesis struct {
	// cause is a short description of a plausible underlying cause.
	// +required
	Cause string `json:"cause"`

	// confidence is the model's own confidence in this hypothesis, as a percentage (0-100) - an
	// integer, not a float: Kubernetes API convention discourages floats in CRD schemas
	// (serialization behaves inconsistently across client languages), so this is the model's
	// 0.0-1.0 confidence converted at the boundary (see internal/controller.FindingReconciler).
	// Not calibrated
	// against ground truth (docs/design.md's per-model accuracy measurement is a future
	// addition, not implemented) - treat as the model's stated confidence, not a verified
	// probability.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +required
	Confidence int32 `json:"confidence"`

	// rationale is a short explanation for why this cause is plausible.
	// +optional
	Rationale string `json:"rationale,omitempty"`

	// recommendedAction is the model's suggestion for which action from Candor's fixed catalog
	// this hypothesis warrants. Advisory only, never authorizing: FindingReconciler independently
	// verifies a concrete, mechanical fix exists (internal/gitops.ComputeFix) before ever acting
	// on ProposePullRequest, and an empty or unrecognised value is always treated as Notify - the
	// same "unsafe input degrades to the safe default" posture as the budget ceiling. A crafted or
	// injected value here can at most suppress an action that should have happened, never cause
	// one that shouldn't (see docs/design.md pillar 6 and the prompt-injection test in
	// internal/llm/anthropic).
	// +kubebuilder:validation:Enum=Notify;ProposePullRequest
	// +optional
	RecommendedAction string `json:"recommendedAction,omitempty"`
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
	// Empty means never enriched. Written by FindingReconciler, never by the code that writes
	// Spec/Fingerprint. When it differs from Fingerprint, enrichment is needed - see
	// internal/signal.NeedsEnrichment.
	// +optional
	EnrichedFingerprint string `json:"enrichedFingerprint,omitempty"`

	// hypotheses are the LLM's ranked, competing explanations for this Finding - never a single
	// asserted cause (docs/design.md pillar 4). Only present once EnrichedFingerprint matches
	// Fingerprint; stale entries from a prior fingerprint are overwritten wholesale on
	// re-enrichment, not merged with new ones.
	// +optional
	Hypotheses []Hypothesis `json:"hypotheses,omitempty"`

	// verificationOutcome is the result of the most recent re-check of this Finding's underlying
	// signal, written by internal/signal.Ingest every time its source is reconciled - not a
	// one-off assertion at creation time. "StillPresent" means the source was re-checked and the
	// condition persists (true for a brand new Finding too - there's no distinct "never verified"
	// state once the first reconcile has run). "Resolved" means the source no longer reports this
	// condition (e.g. a Trivy VulnerabilityReport dropped to zero matching vulnerabilities, or its
	// severity fell below every SignalPolicy's threshold) - the Finding object is kept, not
	// deleted, as the record that it happened. "Recurred" means a Finding that was Resolved had
	// its source produce a real signal again.
	//
	// Deliberately three of the design doc's four verification states, not all four: "Superseded"
	// would apply once multiple providers can report conflicting signals about the same Finding,
	// which doesn't happen yet - each Finding is keyed to exactly one (provider, source object).
	// +kubebuilder:validation:Enum=StillPresent;Resolved;Recurred
	// +optional
	VerificationOutcome string `json:"verificationOutcome,omitempty"`

	// proposedPullRequestURL is the URL of the pull request Candor opened against
	// SignalPolicy.Spec.GitOpsRepo for this Finding's top hypothesis, if any. Empty means no PR
	// has been opened - either ProposePullRequest was never selected, no concrete fix could be
	// computed, or the per-namespace pull request budget was exhausted (see internal/gitops and
	// internal/controller.FindingReconciler). Never cleared once set, even if the Finding later
	// resolves - it's the record that a PR was proposed, not a live status of that PR.
	// +optional
	ProposedPullRequestURL string `json:"proposedPullRequestURL,omitempty"`

	// proposedPullRequestFingerprint is the Fingerprint value ProposePullRequest was last attempted
	// against - the same "no repeat work for unchanged content" gate EnrichedFingerprint already
	// applies to LLM calls, applied here to pull request attempts and the budget they spend. Set
	// on a successful pull request or on a deterministic "no mechanical fix exists for this
	// content" verdict (internal/gitops.ComputeFix) - never on a transient error opening the pull
	// request, or on the per-namespace pull request budget being exhausted, so both of those retry
	// on a later reconcile rather than being given up on permanently.
	// +optional
	ProposedPullRequestFingerprint string `json:"proposedPullRequestFingerprint,omitempty"`

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
// +kubebuilder:printcolumn:name="Verified",type=string,JSONPath=".status.verificationOutcome"
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
