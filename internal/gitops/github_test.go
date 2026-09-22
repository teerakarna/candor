package gitops

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
)

const (
	originalYAML   = "image:\n  repository: ghcr.io/foo/bar\n  tag: v1.0.0\n"
	testBaseBranch = "main"
)

// fakeGitHub stands in for the real GitHub REST API, returning just enough of each real response
// shape for go-github to parse - the same "fake but interface-real" posture
// internal/llm/anthropic's tests use against a real httptest.Server, so GitHubOpener's actual HTTP
// call sequence is exercised for real without ever reaching the live API in CI.
type fakeGitHub struct {
	t              *testing.T
	baseSHA        string
	gotBranchRef   string
	gotUpdatedYAML string
	gotPRHead      string
	gotPRBase      string
}

func (f *fakeGitHub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/git/ref/heads/"):
			_, _ = fmt.Fprintf(w, `{"ref":"refs/heads/main","object":{"sha":%q,"type":"commit"}}`, f.baseSHA)

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git/refs"):
			var body struct{ Ref, SHA string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.gotBranchRef = body.Ref
			_, _ = fmt.Fprintf(w, `{"ref":%q,"object":{"sha":%q,"type":"commit"}}`, body.Ref, body.SHA)

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			content := base64.StdEncoding.EncodeToString([]byte(originalYAML))
			_, _ = fmt.Fprintf(w, `{"type":"file","encoding":"base64","content":%q,"sha":"base-file-sha","path":"values.yaml"}`, content)

		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/contents/"):
			var body struct {
				Content []byte `json:"content"`
				Branch  string `json:"branch"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.gotUpdatedYAML = string(body.Content)
			_, _ = fmt.Fprint(w, `{"content":{"sha":"new-file-sha"},"commit":{"sha":"new-commit-sha"}}`)

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls"):
			var body struct{ Head, Base string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.gotPRHead, f.gotPRBase = body.Head, body.Base
			_, _ = fmt.Fprint(w, `{"number":42,"html_url":"https://github.com/acme/gitops/pull/42"}`)

		default:
			f.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func testRepo() *candorv1alpha1.GitOpsRepo {
	return &candorv1alpha1.GitOpsRepo{
		Owner: "acme", Repo: "gitops", BaseBranch: testBaseBranch,
		Path: "values.yaml", YAMLPath: "image.tag",
	}
}

func testFindingForPR() *candorv1alpha1.Finding {
	return &candorv1alpha1.Finding{
		ObjectMeta: metav1.ObjectMeta{Name: "trivy-abc123", Namespace: "team-a"},
		Spec:       candorv1alpha1.FindingSpec{Summary: "3 critical vulnerabilities in ghcr.io/foo/bar:v1.0.0"},
		Status: candorv1alpha1.FindingStatus{
			Fingerprint: "0123456789abcdef",
			Hypotheses: []candorv1alpha1.Hypothesis{
				{Cause: "outdated base image", Confidence: 80, Rationale: "fix is in a newer tag", RecommendedAction: candorv1alpha1.ActionProposePullRequest},
			},
		},
	}
}

func TestGitHubOpener_Open_OpensExpectedPullRequest(t *testing.T) {
	fake := &fakeGitHub{t: t, baseSHA: "base-commit-sha"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	opener := &GitHubOpener{BaseURL: srv.URL + "/"}
	fix := Fix{Repository: "ghcr.io/foo/bar", CurrentTag: "v1.0.0", NewTag: "v1.2.0"}

	url, err := opener.Open(t.Context(), "test-token", testRepo(), fix, testFindingForPR())
	if err != nil {
		t.Fatal(err)
	}

	if url != "https://github.com/acme/gitops/pull/42" {
		t.Errorf("Open() url = %q, want the created PR's html_url", url)
	}
	if want := "refs/heads/candor/trivy-abc123-0123456789ab"; fake.gotBranchRef != want {
		t.Errorf("branch ref created = %q, want %q (fingerprint-suffixed, so a later fix doesn't collide)", fake.gotBranchRef, want)
	}
	if !strings.Contains(fake.gotUpdatedYAML, "tag: v1.2.0") {
		t.Errorf("updated file content = %q, want it to contain the new tag", fake.gotUpdatedYAML)
	}
	if !strings.Contains(fake.gotUpdatedYAML, "repository: ghcr.io/foo/bar") {
		t.Errorf("updated file content lost an unrelated field: %q", fake.gotUpdatedYAML)
	}
	if fake.gotPRHead != strings.TrimPrefix(fake.gotBranchRef, "refs/heads/") {
		t.Errorf("PR head = %q, want it to match the branch just created (%q)", fake.gotPRHead, fake.gotBranchRef)
	}
	if fake.gotPRBase != testBaseBranch {
		t.Errorf("PR base = %q, want %q", fake.gotPRBase, testBaseBranch)
	}
}

func TestGitHubOpener_Open_DefaultsBaseBranchToMain(t *testing.T) {
	fake := &fakeGitHub{t: t, baseSHA: "base-commit-sha"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	opener := &GitHubOpener{BaseURL: srv.URL + "/"}
	repo := testRepo()
	repo.BaseBranch = "" // unset - GitOpsRepo.BaseBranch's kubebuilder default only applies via the API server, not a plain Go struct in tests

	if _, err := opener.Open(t.Context(), "test-token", repo, Fix{Repository: "r", CurrentTag: "v1", NewTag: "v2"}, testFindingForPR()); err != nil {
		t.Fatal(err)
	}
	if fake.gotPRBase != testBaseBranch {
		t.Errorf("PR base = %q, want the hardcoded fallback %q", fake.gotPRBase, testBaseBranch)
	}
}
