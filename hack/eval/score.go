/*
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
*/

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/teerakarna/candor/internal/llm"
)

// Result is one fixture's outcome against a real (or, in tests, faked) llm.Response.
type Result struct {
	Fixture       Fixture
	Response      llm.Response
	Err           error
	ActionCorrect bool
	CauseCorrect  bool
}

// Score grades resp against f. A call error scores both dimensions false rather than panicking -
// a single failed fixture (rate limit, transient network) shouldn't abort the whole run; Report
// below still counts it and shows the error.
func Score(f Fixture, resp llm.Response, err error) Result {
	r := Result{Fixture: f, Response: resp, Err: err}
	if err != nil || len(resp.Hypotheses) == 0 {
		return r
	}

	// Hypotheses[0], not the highest-confidence entry - matches exactly what
	// FindingReconciler.tryProposePullRequest reads in production
	// (internal/controller/finding_controller.go), so this measures the real gate.
	top := resp.Hypotheses[0]
	r.ActionCorrect = top.RecommendedAction == f.WantAction

	cause := strings.ToLower(top.Cause)
	for _, want := range f.WantCauseContains {
		if strings.Contains(cause, strings.ToLower(want)) {
			r.CauseCorrect = true
			break
		}
	}
	return r
}

// Report renders results as the published markdown table docs/design.md's evidence gap #7 answer
// promises ("per-model accuracy is measured and published against a fixture suite, not
// asserted"). A low score is still a published result, not a build failure - see README.md for
// why this never runs as a required CI check.
func Report(model string, results []Result) string {
	actionN, causeN := 0, 0
	for _, r := range results {
		if r.ActionCorrect {
			actionN++
		}
		if r.CauseCorrect {
			causeN++
		}
	}
	n := len(results)

	var b strings.Builder
	fmt.Fprintf(&b, "# Fixture suite results - %s\n\n", model)
	fmt.Fprintf(&b, "Action accuracy: %d/%d (%.0f%%)\n\n", actionN, n, pct(actionN, n))
	fmt.Fprintf(&b, "Cause accuracy: %d/%d (%.0f%%)\n\n", causeN, n, pct(causeN, n))
	b.WriteString("| Fixture | Action | Cause | Error |\n|---|---|---|---|\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			r.Fixture.Name, mark(r.ActionCorrect), mark(r.CauseCorrect), errText(r.Err))
	}
	return b.String()
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

func mark(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// RunSummary is one full pass of the fixture suite against one backend, with what it cost to get:
// wall time, and the tokens the provider reported (zero for a backend that reports none).
type RunSummary struct {
	Results []Result
	Elapsed time.Duration
}

func (s RunSummary) accuracy() (action, cause float64) {
	actionN, causeN := 0, 0
	for _, r := range s.Results {
		if r.ActionCorrect {
			actionN++
		}
		if r.CauseCorrect {
			causeN++
		}
	}
	return pct(actionN, len(s.Results)), pct(causeN, len(s.Results))
}

func (s RunSummary) errors() int {
	n := 0
	for _, r := range s.Results {
		if r.Err != nil {
			n++
		}
	}
	return n
}

func (s RunSummary) tokens() (in, out int) {
	for _, r := range s.Results {
		in += r.Response.Usage.InputTokens
		out += r.Response.Usage.OutputTokens
	}
	return in, out
}

// ReportRuns renders several runs of the same backend. One run of eight fixtures is a very small
// sample from a non-deterministic model, so a single accuracy figure can mislead in either
// direction; showing the spread across runs, and how many calls errored outright (a model that
// does not fit in memory shows up as errors, not as a low score), is what makes a comparison
// between models worth reading.
func ReportRuns(model string, runs []RunSummary) string {
	n := len(runs)
	if n == 0 {
		return fmt.Sprintf("# Fixture suite results - %s\n\nNo runs.\n", model)
	}

	var actions, causes []float64
	var totalErrors, totalCalls, totalIn, totalOut int
	var totalElapsed time.Duration
	for _, r := range runs {
		a, c := r.accuracy()
		actions = append(actions, a)
		causes = append(causes, c)
		totalErrors += r.errors()
		totalCalls += len(r.Results)
		in, out := r.tokens()
		totalIn += in
		totalOut += out
		totalElapsed += r.Elapsed
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Fixture suite results - %s (%d runs)\n\n", model, n)
	fmt.Fprintf(&b, "Action accuracy: %s\n\n", spread(actions))
	fmt.Fprintf(&b, "Cause accuracy: %s\n\n", spread(causes))
	fmt.Fprintf(&b, "Errored calls: %d of %d\n\n", totalErrors, totalCalls)
	fmt.Fprintf(&b, "Per run, on average: %d input and %d output tokens, %s wall time\n\n",
		totalIn/n, totalOut/n, (totalElapsed / time.Duration(n)).Round(time.Second))

	b.WriteString("| Fixture | Action (passes) | Cause (passes) |\n|---|---|---|\n")
	for i, f := range Fixtures {
		actionN, causeN := 0, 0
		for _, r := range runs {
			if i < len(r.Results) {
				if r.Results[i].ActionCorrect {
					actionN++
				}
				if r.Results[i].CauseCorrect {
					causeN++
				}
			}
		}
		fmt.Fprintf(&b, "| %s | %d/%d | %d/%d |\n", f.Name, actionN, n, causeN, n)
	}
	return b.String()
}

// spread formats a series of per-run percentages as "mean (min to max)".
func spread(vals []float64) string {
	lo, hi, sum := vals[0], vals[0], 0.0
	for _, v := range vals {
		sum += v
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return fmt.Sprintf("%.0f%% mean (%.0f%% to %.0f%%) over %d runs", sum/float64(len(vals)), lo, hi, len(vals))
}
