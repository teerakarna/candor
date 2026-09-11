# Grafana dashboard preview

A local `docker compose` stack for visually checking
[`charts/chart/files/grafana-dashboard.json`](../../charts/chart/files/grafana-dashboard.json)
without needing a real Kubernetes cluster, a real Candor deployment, or real Trivy scan data.

Three services:

- **fake-metrics** (`fake_metrics.py`) - serves `/metrics` in Prometheus text format, using the
  exact metric and label names `internal/metrics/metrics.go` registers for real
  (`candor_llm_calls_total`, `candor_enrichment_skipped_total`,
  `candor_verification_transitions_total`, `candor_signalpolicy_budget_calls_used`/`_limit`).
  Values drift a little every few seconds so the dashboard's timeseries panels actually move.
- **prometheus** - scrapes fake-metrics every 5s.
- **grafana** - both the Prometheus datasource and the dashboard itself are auto-provisioned
  (`provisioning/`) - no manual "Add data source" or "Import dashboard" click-through.

Dev tool only: not part of the release build, not shipped in the chart, no Go package imports it.

## Run it

```sh
cd hack/grafana-preview
docker compose up
```

Open [http://localhost:3000](http://localhost:3000) (`admin` / `admin`) - the "Candor Overview"
dashboard is already there, already pointed at Prometheus, already showing data.

```sh
docker compose down
```

when done.

## What this does and doesn't prove

Confirms the dashboard JSON is structurally valid, provisions cleanly, its `${DS_PROMETHEUS}`
datasource-template variable resolves, and the panels render against data shaped like the real
metrics. It does **not** prove the operator itself emits correct values - that's covered
separately by `internal/metrics` and the reconciler tests. It's a "does this look right" tool for
a human, not a substitute for the CI dashboard-import check in `.github/workflows/ci.yml`.
