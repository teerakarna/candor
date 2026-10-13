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
