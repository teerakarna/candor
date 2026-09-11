## What and why

## Test plan

- [ ] `make build`
- [ ] `make test` (envtest)
- [ ] `make lint`
- [ ] `make manifests generate` run and output committed, if any `_types.go` file changed
- [ ] Doesn't bypass the fingerprint/budget layer, grant cluster write RBAC, or let model output
      select a free-form action (see CONTRIBUTING.md's design constraints)
