# Candor - Claude instructions

Read `CONTRIBUTING.md` first for the human-facing contribution process (design constraints PRs
must respect, how findings get recorded) and `docs/design.md` for the design reasoning and current
delivery status. This file adds what's specific to working here as an agent.

## Before every merge

- `make build`, `make lint`, `make test` all clean, locally, before pushing - not just relying on
  CI to catch it.
- Run `/code-review` on the diff before opening a PR, sized to the change - low effort for a docs
  fix, high for anything touching the action catalog, budgets, or a new ingress surface. Green CI
  is necessary, not sufficient - two real incidents from this repo's own history make the case
  directly:
  - PR #47: `make lint-fix` touched ~20 files; only 2 were staged and pushed. CI failed on files
    that had never actually been committed, on a check that had passed locally only because the
    working tree (not the pushed tree) still had the fix applied. A `/code-review` pass against the
    actual diff being pushed would have caught the missing files before CI did.
  - Issue #52: a claim about Ollama's `format: "json"` vs. a full JSON schema was asserted as
    "confirmed" before it was actually tested, then had to be publicly corrected once it was. Green
    CI doesn't check whether a claim in a comment or commit message was verified before being written.
- Re-run `/code-review` after fixing what it finds, not just once - a second pass has caught a real
  bug the previous pass's own fix introduced elsewhere in this ecosystem (see loom's `CLAUDE.md`).
  Stop once a pass comes back clean, not before.
- If the review agent stalls or can't complete, proceed on the strength of the full automated gate
  above plus manual verification, and say so plainly in the PR description rather than presenting
  it as a completed review.
- Verify real findings by running them, not by reading the diff and reasoning about it - this
  repo's own discipline already says so (`CONTRIBUTING.md`, "Recording findings": "Include the
  measurement, not the impression"). `make dev-up` and `hack/ollama-dev/` exist specifically so a
  claim about real behavior can be checked against a real cluster or a real model instead of
  inferred.
- Sweep stray em/en dashes to plain hyphens in any file the change touches (global rule,
  `~/.claude/CLAUDE.md`) - `grep -c '—\|–'` before every commit. This is not yet true of the whole
  repo's existing prose (predates consistent enforcement); it only applies to files a change
  actually touches, not a mandate to rewrite everything at once.

## Merging

`main` is protected, including against the repo owner - no direct push, no force push, even for an
admin (`enforce_admins: true`), linear history only (squash merge).

- **Nine checks must be green**: `lint`, `test` (×3 supported Kubernetes versions), `e2e`,
  `govulncheck`, `trivy-config`, `gitleaks`, `codeql`. The branch must also be up to date with
  `main` before merging (`strict: true`) - with no merge queue on this repo, merging PR *N* pushes
  every other open PR behind `main` again; expect to re-sync and re-wait on checks when landing
  several in sequence.
- **Commits must be signed.** Already working transparently in this environment - if a push starts
  failing the signature check, that's the thing to investigate, not a reason to disable it.

## After merge

Sync local `main` from `origin/main` (fetch, then reset to it) rather than trusting the local
branch state - a squash merge rewrites history the local branch does not have.

## Issues before architecture

`GOVERNANCE.md` already requires an issue before a PR for anything touching a design constraint or
the project's stated scope. In practice this has meant: write the issue(s) first (accelerator and
its brake as separate, cross-referenced issues when a brake is required - see slice 9's #38-41 and
slice 10's #50-53 for the pattern), then implement. Don't skip straight to a PR for anything that
changes what Candor does, only for mechanical fixes to something it already does.
