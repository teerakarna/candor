// Package gitops computes concrete, mechanical fixes for a Finding and opens a pull request
// carrying one against the configured GitOps repository (docs/design.md's ProposePullRequest
// action).
//
// ComputeFix deliberately never uses the LLM's hypothesis to decide what to patch - a
// Hypothesis.RecommendedAction is an opinion the caller may act on, not a source of the change
// itself. The patch has to be mechanically derivable from the raw signal or ComputeFix returns
// ok=false, and the caller degrades to Notify. That's what keeps ProposePullRequest from ever
// emitting an LLM-invented or unfalsifiable change: the model can suggest that a fix is warranted,
// it can never supply the fix.
package gitops

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/provider/trivy"
)

// Fix is a concrete, single-value image tag bump - the only kind of patch this slice computes.
// Broader patch generation (multi-file, non-image changes) is out of scope until a real need for
// it exists.
type Fix struct {
	// Repository is the image repository the fix applies to, e.g. "ghcr.io/foo/bar".
	Repository string
	// CurrentTag is the tag currently deployed.
	CurrentTag string
	// NewTag is the tag to patch the GitOps repo to.
	NewTag string
}

// ComputeFix derives a Fix for finding by re-fetching the raw signal object its
// FindingSource.RefKind/RefName point at (see that field's doc comment: it exists precisely for
// traceability back to raw evidence). Returns ok=false, not an error, whenever no concrete fix can
// be established:
//   - the source isn't a Trivy VulnerabilityReport (the only provider ComputeFix understands so
//     far - matching internal/provider's "unrecognised, so skip" stance elsewhere in this
//     codebase);
//   - the source object no longer exists;
//   - the report doesn't name an image repository/tag;
//   - the vulnerabilities driving this Finding don't agree on a single fixed version - a
//     container image can carry multiple distinct vulnerable packages with different fixes, and
//     bumping to any one of them would not actually resolve the finding. Only a report where every
//     reported vulnerability points at the same fixedVersion produces an unambiguous, mechanical
//     patch.
func ComputeFix(ctx context.Context, c client.Client, finding *candorv1alpha1.Finding) (Fix, bool, error) {
	if finding.Spec.Source.RefKind != trivy.GroupVersionKind.Kind {
		return Fix{}, false, nil
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(trivy.GroupVersionKind)
	if err := c.Get(ctx, client.ObjectKey{Namespace: finding.Namespace, Name: finding.Spec.Source.RefName}, u); err != nil {
		if apierrors.IsNotFound(err) {
			return Fix{}, false, nil
		}
		return Fix{}, false, fmt.Errorf("getting VulnerabilityReport %s/%s: %w", finding.Namespace, finding.Spec.Source.RefName, err)
	}

	repository, _, _ := unstructured.NestedString(u.Object, "report", "artifact", "repository")
	tag, _, _ := unstructured.NestedString(u.Object, "report", "artifact", "tag")
	if repository == "" || tag == "" {
		return Fix{}, false, nil
	}

	vulnerabilities, _, _ := unstructured.NestedSlice(u.Object, "report", "vulnerabilities")
	fixedVersion, ok := uniformFixedVersion(vulnerabilities)
	if !ok || fixedVersion == tag {
		return Fix{}, false, nil
	}

	return Fix{Repository: repository, CurrentTag: tag, NewTag: fixedVersion}, true, nil
}

// uniformFixedVersion returns the single non-empty fixedVersion every vulnerability in the list
// agrees on, or ok=false if none is set or they disagree.
func uniformFixedVersion(vulnerabilities []any) (fixedVersion string, ok bool) {
	for _, v := range vulnerabilities {
		m, isMap := v.(map[string]any)
		if !isMap {
			continue
		}
		fv, _ := m["fixedVersion"].(string)
		if fv == "" {
			continue
		}
		if fixedVersion == "" {
			fixedVersion = fv
		} else if fixedVersion != fv {
			return "", false
		}
	}
	return fixedVersion, fixedVersion != ""
}
