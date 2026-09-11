package provider

import (
	"testing"

	"github.com/teerakarna/candor/internal/provider/trivy"
)

// Known can't reference trivy.ProviderName directly (see the comment in provider.go - it would
// cycle with trivy's own tests), so this catches the two ever drifting apart by hand.
func TestKnown_MatchesTrivyProviderName(t *testing.T) {
	if !IsKnown(trivy.ProviderName) {
		t.Errorf("Known = %v does not contain trivy.ProviderName (%q) - these must be kept in sync by hand, see provider.go", Known, trivy.ProviderName)
	}
}
