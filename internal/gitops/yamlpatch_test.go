package gitops

import (
	"strings"
	"testing"
)

func TestSetYAMLPath_NestedScalar(t *testing.T) {
	input := `# a comment worth preserving
image:
  repository: ghcr.io/foo/bar
  tag: v1.0.0
replicas: 3
`
	out, err := setYAMLPath([]byte(input), "image.tag", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	if !strings.Contains(got, "tag: v1.2.0") {
		t.Errorf("output missing patched tag:\n%s", got)
	}
	if !strings.Contains(got, "# a comment worth preserving") {
		t.Errorf("output lost the leading comment - patch must preserve formatting:\n%s", got)
	}
	if !strings.Contains(got, "replicas: 3") {
		t.Errorf("output lost an unrelated sibling field:\n%s", got)
	}
	if strings.Contains(got, "v1.0.0") {
		t.Errorf("output still contains the old tag:\n%s", got)
	}
}

func TestSetYAMLPath_TopLevelScalar(t *testing.T) {
	out, err := setYAMLPath([]byte("tag: v1.0.0\n"), "tag", "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "tag: v2.0.0") {
		t.Errorf("output = %s, want tag: v2.0.0", out)
	}
}

func TestSetYAMLPath_MissingSegment_Errors(t *testing.T) {
	_, err := setYAMLPath([]byte("image:\n  repository: foo\n"), "image.tag", "v2.0.0")
	if err == nil {
		t.Fatal("expected an error for a path segment that doesn't exist")
	}
}

func TestSetYAMLPath_NonScalarTarget_Errors(t *testing.T) {
	_, err := setYAMLPath([]byte("image:\n  repository: foo\n  tag: v1\n"), "image", "v2.0.0")
	if err == nil {
		t.Fatal("expected an error when the path resolves to a mapping, not a scalar")
	}
}

func TestSetYAMLPath_EmptyDocument_Errors(t *testing.T) {
	_, err := setYAMLPath([]byte(""), "image.tag", "v2.0.0")
	if err == nil {
		t.Fatal("expected an error for an empty document")
	}
}
