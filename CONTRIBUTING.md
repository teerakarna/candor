# Contributing

Candor is early — architecture agreed, build in progress in slices (see the design doc). The most
useful contribution right now is discussion on an issue before a PR, since the CRD shapes and the
fingerprinting/budget model are still settling.

## Development

- Go (see `go.mod` for the minimum version).
- [kubebuilder](https://book.kubebuilder.io/) 4.x.
- `make test` (runs `envtest` against a real control plane — not mocked) and `make lint` before
  opening a PR.
- `make manifests generate` after editing any `_types.go` file, and commit the regenerated output.
- After a CRD or RBAC change, regenerate the Helm chart:
  `kubebuilder edit --plugins=helm/v2-alpha --output-dir=charts --force`. This **will** revert
  hand-maintained things back to their generated defaults — re-apply them every time:
  - `charts/chart/values.yaml`: `manager.image.repository` back to `ghcr.io/teerakarna/candor`
  - `charts/chart/.helmignore`: the `dist/chart/*.tgz` line back to `*.tgz`
  - `.github/workflows/test-chart.yml`: triggers back to `branches: [main]`, and the
    `helm lint` path back to `./charts/chart`
  - Any other custom top-level key added to `values.yaml` by hand (e.g. `grafanaDashboard`) - the
    whole file is regenerated from the plugin's own template, so a key it doesn't already know
    about is silently dropped, not merged. A custom **template** file under `charts/chart/templates/`
    (and any file under `charts/chart/files/`) is untouched, since those aren't part of kustomize's
    output - only `values.yaml` itself gets wholesale regenerated.
  Verify with `helm lint charts/chart` and `helm template test charts/chart | grep image:` after.

## Design constraints that PRs must respect

These aren't style preferences — they're the project's whole reason to exist, each backed by a
documented failure in an incumbent tool. See the design doc's evidence base before proposing a
change that touches these:

- **No unbounded LLM calls.** Every enrichment call must go through the fingerprint + budget layer.
  A change that calls the model directly, bypassing fingerprinting, will be rejected regardless of
  how useful it seems locally.
- **Read-only cluster access by default.** The operator's ServiceAccount must not gain write RBAC
  outside the documented, gated Enforcing-mode path.
- **No free-form model-selected actions.** Model output selects from the fixed action catalog; it
  never emits an action string that gets executed directly.
- **Every finding is falsifiable.** A new finding type needs a defined verification check — "how do
  we know this cleared" — before it ships.

## Recording findings

Chat history is not a record, and a finding nobody wrote down did not happen. The test for where a
finding belongs is whether it survives the session somewhere durable.

| Situation | Where it goes |
|---|---|
| Fixed in the same change | No issue. The PR description and commit message are the record, and a better one, because they carry the fix and the evidence together |
| Found, but deferred | An issue, always. Otherwise it exists only in a conversation nobody will re-read |
| Found, won't fix, or the call belongs to someone else | An issue, for the same reason |
| Recurring, or it should shape future work | A design-doc section or constraint, not an issue |

Do not open an issue per observation. Answering "too much to keep track of" by producing more to
keep track of is the failure "Every accelerator ships with its brake" exists to prevent, and it
applies to the issue tracker as readily as to the code.

Include the measurement, not the impression. Two worked examples from this repo's own history: the
KubeGuard paper's F1 range was cited as 0.79-0.81 when the real range is 0.504-0.808, and the
citation for k8sgpt-operator#730 described a bug that had since been fixed. Both were caught by
re-checking against the live source, and both are recorded in `docs/design.md` with the correction
marked inline rather than quietly edited.

**Evidence decays.** Issues get fixed, projects ship the thing you said they never shipped, and
papers say something narrower than the summary of them. Anything cited publicly gets re-verified
first.

## Commit style

Conventional commits (`feat:`, `fix:`, `docs:`, `chore:`) where practical.

## On how this project is built

This project is built collaboratively with Claude Code. That's disclosed here plainly, and it's
visible in the commit history regardless — the point isn't the disclosure itself, it's that a
project whose whole premise is refusing to assert what it can't back with evidence would be
undermining its own thesis if it weren't upfront about how it's made. Every technical claim in
[`docs/design.md`](docs/design.md) is sourced against a cited, checkable reference, not asserted.

## Reporting a security issue

See [SECURITY.md](SECURITY.md).

## Governance

See [GOVERNANCE.md](GOVERNANCE.md) for how decisions get made.
