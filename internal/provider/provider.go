// Package provider is the parent of the individual signal-source implementations (see
// internal/provider/trivy for the first one).
package provider

import (
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Known lists the provider names a SignalPolicy may reference. Deliberately not
// []string{trivy.ProviderName} - internal/provider/trivy's own tests need to import this package
// (to exercise CRDInstalled), so this package importing trivy back would cycle. Keep this string
// in sync with trivy.ProviderName and webhook.ProviderName by hand.
var Known = []string{"trivy", "webhook"}

// IsKnown reports whether name is a recognised provider.
func IsKnown(name string) bool {
	return slices.Contains(Known, name)
}

// CRDInstalled reports whether gvk's CRD exists in the cluster the given RESTMapper is talking
// to. Every provider watches a CRD it doesn't own (that's the point - see docs/design.md's
// provider pattern), so a cluster without that CRD installed must not crash the whole manager;
// callers use this to skip registering a provider's watch instead, and log why.
func CRDInstalled(mapper meta.RESTMapper, gvk schema.GroupVersionKind) (bool, error) {
	_, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err == nil {
		return true, nil
	}
	if meta.IsNoMatchError(err) {
		return false, nil
	}
	return false, err
}
