package gitops

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-github/v76/github"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
)

// Opener opens a pull request carrying fix against repo. GitHubOpener is the real implementation;
// tests substitute a fake pointed at an httptest.Server (see github_test.go) - the same
// "fake but interface-real" posture internal/llm.Client already uses, so the reconciler wiring is
// exercised for real without ever making a live GitHub call in CI.
type Opener interface {
	Open(ctx context.Context, token string, repo *candorv1alpha1.GitOpsRepo, fix Fix, finding *candorv1alpha1.Finding) (prURL string, err error)
}

// GitHubOpener implements Opener against the GitHub REST API via go-github. It authenticates
// per-call with the token the caller supplies (resolved from SignalPolicy.Spec.GitOpsRepo.SecretRef
// by internal/controller.FindingReconciler, which owns all Secret reads) rather than holding one
// fixed client, since different SignalPolicies can name different repos and tokens.
type GitHubOpener struct {
	// BaseURL overrides the GitHub API base URL - for tests, pointed at an httptest.Server. Empty
	// means the real public GitHub API.
	BaseURL string
}

func (o *GitHubOpener) Open(ctx context.Context, token string, repo *candorv1alpha1.GitOpsRepo, fix Fix, finding *candorv1alpha1.Finding) (string, error) {
	client := github.NewClient(nil).WithAuthToken(token)
	if o.BaseURL != "" {
		u, err := url.Parse(o.BaseURL)
		if err != nil {
			return "", fmt.Errorf("parsing base URL: %w", err)
		}
		client.BaseURL = u
	}

	base := repo.BaseBranch
	if base == "" {
		base = "main"
	}

	baseRef, _, err := client.Git.GetRef(ctx, repo.Owner, repo.Repo, "heads/"+base)
	if err != nil {
		return "", fmt.Errorf("getting base ref heads/%s: %w", base, err)
	}

	// Fingerprint-suffixed, not just the Finding name: NeedsEnrichment only lets this run once per
	// distinct fingerprint, but a Finding can go through several fixes over its lifetime (content
	// changes, gets fixed, recurs differently) and each needs its own branch rather than colliding
	// with a leftover one from a prior attempt.
	branch := fmt.Sprintf("candor/%s-%s", finding.Name, shortFingerprint(finding.Status.Fingerprint))
	if _, _, err := client.Git.CreateRef(ctx, repo.Owner, repo.Repo, github.CreateRef{
		Ref: "refs/heads/" + branch,
		SHA: baseRef.GetObject().GetSHA(),
	}); err != nil {
		return "", fmt.Errorf("creating branch %s: %w", branch, err)
	}

	content, _, _, err := client.Repositories.GetContents(ctx, repo.Owner, repo.Repo, repo.Path, &github.RepositoryContentGetOptions{Ref: base})
	if err != nil {
		return "", fmt.Errorf("getting %s: %w", repo.Path, err)
	}
	raw, err := content.GetContent()
	if err != nil {
		return "", fmt.Errorf("decoding %s: %w", repo.Path, err)
	}

	patched, err := setYAMLPath([]byte(raw), repo.YAMLPath, fix.NewTag)
	if err != nil {
		return "", fmt.Errorf("patching %s at %s: %w", repo.Path, repo.YAMLPath, err)
	}

	title := fmt.Sprintf("candor: bump %s from %s to %s", fix.Repository, fix.CurrentTag, fix.NewTag)
	if _, _, err := client.Repositories.UpdateFile(ctx, repo.Owner, repo.Repo, repo.Path, &github.RepositoryContentFileOptions{
		Message: new(title),
		Content: patched,
		SHA:     content.SHA,
		Branch:  new(branch),
	}); err != nil {
		return "", fmt.Errorf("updating %s on %s: %w", repo.Path, branch, err)
	}

	pr, _, err := client.PullRequests.Create(ctx, repo.Owner, repo.Repo, &github.NewPullRequest{
		Title: new(title),
		Head:  new(branch),
		Base:  new(base),
		Body:  new(prBody(fix, finding)),
	})
	if err != nil {
		return "", fmt.Errorf("creating pull request: %w", err)
	}

	return pr.GetHTMLURL(), nil
}

// prBody renders the finding as the PR description - "the pull request is also a report"
// (docs/design.md): no new surface for a GitOps team to learn, the ranked hypotheses and
// confidence that would otherwise only live in the Finding object are right there in the PR.
func prBody(fix Fix, finding *candorv1alpha1.Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Candor detected: %s\n\n", finding.Spec.Summary)
	fmt.Fprintf(&b, "Bumps `%s` from `%s` to `%s`.\n", fix.Repository, fix.CurrentTag, fix.NewTag)

	if len(finding.Status.Hypotheses) > 0 {
		b.WriteString("\nRanked hypotheses:\n\n")
		for _, h := range finding.Status.Hypotheses {
			fmt.Fprintf(&b, "- **%s** (%d%% confidence): %s\n", h.Cause, h.Confidence, h.Rationale)
		}
	}

	fmt.Fprintf(&b, "\n---\nOpened automatically by [Candor](https://github.com/teerakarna/candor) for Finding `%s/%s`.\n", finding.Namespace, finding.Name)
	return b.String()
}

// shortFingerprint truncates a fingerprint hash for use in a branch name - full-length is
// unnecessary and makes branch names unwieldy; empty input (a Finding reconciled before its
// fingerprint was ever set) falls back to a fixed label rather than producing a malformed name.
func shortFingerprint(fingerprint string) string {
	const length = 12
	if fingerprint == "" {
		return "unknown"
	}
	if len(fingerprint) <= length {
		return fingerprint
	}
	return fingerprint[:length]
}
