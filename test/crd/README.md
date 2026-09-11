# Test-only CRDs

Third-party CRDs needed for envtest to accept objects Candor watches but doesn't own. Not part of
the shipped Helm chart - these are fixtures for `make test`/`make test-e2e` only.

| File | Source | Pinned |
|---|---|---|
| `aquasecurity.github.io_vulnerabilityreports.yaml` | [aquasecurity/trivy-operator](https://github.com/aquasecurity/trivy-operator/blob/main/deploy/helm/crds/aquasecurity.github.io_vulnerabilityreports.yaml) | 2026-09-11 |

Refresh manually (no automated dependency on trivy-operator's release cycle - a schema change
there should be a deliberate, reviewed update here, not something that silently drifts):

```bash
gh api repos/aquasecurity/trivy-operator/contents/deploy/helm/crds/aquasecurity.github.io_vulnerabilityreports.yaml \
  --jq '.content' | base64 -d > test/crd/aquasecurity.github.io_vulnerabilityreports.yaml
```

After refreshing, check `internal/provider/trivy/translate.go`'s field references still match.
