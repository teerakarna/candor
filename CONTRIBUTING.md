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

## Commit style

Conventional commits (`feat:`, `fix:`, `docs:`, `chore:`) where practical.

## Reporting a security issue

See [SECURITY.md](SECURITY.md).
