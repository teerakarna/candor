# Candor

A Kubernetes operator that ingests signals from security/observability tools (Trivy, and more
later), uses an LLM to enrich them into ranked hypotheses, and proposes remediation as GitOps pull
requests — with bounded LLM spend and a published record of its own accuracy.

## Why

Existing LLM-assisted Kubernetes ops tools (k8sgpt, Kubernaut, HolmesGPT) treat model invocation as
stateless, unbounded, and unaccountable — which is exactly what shows up in their own issue
trackers as runaway API spend, unshippable suppression, and no way to check whether a fix actually
worked. Candor does the same underlying job — signal in, LLM enrichment, human-reviewable action
out — engineered against those specific, evidenced failures:

- **Bounded cost**: an LLM call happens once per distinct cluster-state fingerprint, ever, not once
  per reconcile. A hard budget ceiling degrades to deterministic-only analysis rather than silently
  spending money.
- **GitOps-native**: the default write path is a pull request, not a cluster mutation — so it
  doesn't fight Flux/Argo for control of the cluster.
- **Self-measuring**: every finding is re-checked and its outcome (resolved/still-present/recurred)
  is published as metrics — an answer to the "AIOps: Prove It!" critique that most of this category
  hasn't answered.

Full rationale, competitive analysis, and evidence base: [`docs/design.md`](docs/design.md).

**Status**: early build, in slices — see the design doc's delivery-slices list for what's done.

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/candor:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/candor:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Two install paths, both produced by the release pipeline — nothing hand-built or committed to
`main`, so what you install is always a specific, versioned, signed release.

### Helm chart (recommended)

```sh
helm install candor oci://ghcr.io/teerakarna/charts/candor --version <version> \
  --namespace candor-system --create-namespace
```

Chart source lives under `charts/chart/` (generated via `kubebuilder edit
--plugins=helm/v2-alpha --output-dir=charts`, regenerate the same way after changing the API or
RBAC). Published to the OCI registry on every tagged release, alongside the signed image.

### YAML bundle

Each [release](https://github.com/teerakarna/candor/releases) attaches a versioned `install.yaml`
(all resources, generated fresh at release time — not a stale copy on `main`):

```sh
kubectl apply -f https://github.com/teerakarna/candor/releases/download/<tag>/install.yaml
```

To build it yourself: `make build-installer IMG=ghcr.io/teerakarna/candor:<tag>`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) — in particular the design constraints PRs must respect
(bounded LLM calls, read-only cluster access by default, no free-form model-selected actions).

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026 Albert Asawaroengchai.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

