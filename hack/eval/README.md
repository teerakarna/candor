# Fixture suite

Measures each configured LLM backend's actual accuracy on Candor's real enrichment task, instead
of asserting it from reputation or a third-party benchmark - the fixture suite docs/design.md's
evidence gap #7 answer promises ([issue #106](https://github.com/teerakarna/candor/issues/106),
extended to Ollama candidate models by
[issue #107](https://github.com/teerakarna/candor/issues/107)).

`fixtures.go` holds the suite: eight cases, each a real-shaped `llm.Request` paired with the
answer a correct enrichment should produce. Four are real, named CVEs with a single fixed version
(Log4Shell, the Struts OGNL RCE, Heartbleed, the curl SOCKS5 overflow) where
`ProposePullRequest` is the right call. Four are cases where it isn't: two vulnerability-report
shapes with no clean single fix, and two generic-webhook-shaped findings (an RBAC
misconfiguration, a Falco runtime anomaly) that aren't dependency fixes at all.

`score.go` grades `Hypotheses[0]` specifically - the same hypothesis
`FindingReconciler.tryProposePullRequest` itself reads
(`internal/controller/finding_controller.go`) - on two dimensions: does its `RecommendedAction`
match what the fixture expects, and does its `Cause` contain at least one of the fixture's
expected substrings. `score_test.go` covers this grading logic directly, with no network call, so
it runs under the normal `make test`.

## Run it

Each backend is independently opt-in - set the credentials/config for whichever you want to run,
and `make eval` runs all of them and publishes one combined report:

```sh
export ANTHROPIC_API_KEY=...            # real key - makes real API calls, spends real credits
export CANDOR_OLLAMA_HOST=http://localhost:11434   # a real reachable Ollama instance - see hack/ollama-dev/
make eval
```

Running with only `ANTHROPIC_API_KEY` set behaves exactly as it did before #107 - one backend, one
section in the report. `CANDOR_OLLAMA_HOST` must be set explicitly; there's no silent attempt
against `localhost:11434` for a run that only wanted the hosted baseline. `CANDOR_LLM_MODEL`
overrides the Ollama candidate model (default `llama3.2:3b`, matching the model issue #52 already
verified grammar-constrained output against).

Prints the report and writes it to `report.md` in this directory. Commit the refreshed
`report.md` when you want the published numbers to reflect a new run (a new fixture, a model
change, a routine re-check) - it isn't regenerated automatically.

## Why this never runs in CI

Dev/benchmark tool only, same posture as [`hack/ollama-dev/`](../ollama-dev/) and
[`hack/grafana-preview/`](../grafana-preview/): not part of `make build`, `make lint`, or `make
test`, and not wired into any CI workflow. Three real reasons, not just "it's slow":

- It needs a real `ANTHROPIC_API_KEY` and spends real API credits on every run - the opposite of
  the budget-consciousness the rest of this project enforces on itself.
- Model output is non-deterministic, so this can't be a required, blocking CI check the way
  `make test` is - a flaky red build helps no one.
- Its purpose is a measured, published number for humans to read, not a pass/fail gate. A low
  score is still a correct, useful result - run it, publish what it says, don't fail a build over
  it.
