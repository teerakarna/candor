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
	"errors"
	"strings"
	"testing"
	"time"

	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/llm"
)

func fixture(wantAction string, wantCause ...string) Fixture {
	return Fixture{Name: "f", WantAction: wantAction, WantCauseContains: wantCause}
}

func TestScore_CorrectActionAndCause(t *testing.T) {
	f := fixture(candorv1alpha1.ActionProposePullRequest, "jndi", "log4j")
	resp := llm.Response{Hypotheses: []llm.Hypothesis{
		{Cause: "JNDI injection in log4j", RecommendedAction: candorv1alpha1.ActionProposePullRequest},
	}}

	r := Score(f, resp, nil)

	if !r.ActionCorrect {
		t.Error("ActionCorrect = false, want true")
	}
	if !r.CauseCorrect {
		t.Error("CauseCorrect = false, want true")
	}
}

func TestScore_WrongAction(t *testing.T) {
	f := fixture(candorv1alpha1.ActionProposePullRequest, "jndi")
	resp := llm.Response{Hypotheses: []llm.Hypothesis{
		{Cause: "JNDI injection", RecommendedAction: candorv1alpha1.ActionNotify},
	}}

	if r := Score(f, resp, nil); r.ActionCorrect {
		t.Error("ActionCorrect = true, want false")
	}
}

func TestScore_OnlyTopHypothesisCounts(t *testing.T) {
	// The second hypothesis matches on both dimensions; the first (what production actually
	// reads - internal/controller/finding_controller.go) does not. The fixture suite must grade
	// the same hypothesis the reconciler gates on, not whichever one happens to be right.
	f := fixture(candorv1alpha1.ActionProposePullRequest, "jndi")
	resp := llm.Response{Hypotheses: []llm.Hypothesis{
		{Cause: "unrelated misconfiguration", RecommendedAction: candorv1alpha1.ActionNotify},
		{Cause: "JNDI injection", RecommendedAction: candorv1alpha1.ActionProposePullRequest},
	}}

	r := Score(f, resp, nil)

	if r.ActionCorrect || r.CauseCorrect {
		t.Errorf("Score = %+v, want both false (only Hypotheses[0] should be graded)", r)
	}
}

func TestScore_CauseMatchIsCaseInsensitiveAnyOf(t *testing.T) {
	f := fixture(candorv1alpha1.ActionNotify, "RBAC", "cluster-admin")
	resp := llm.Response{Hypotheses: []llm.Hypothesis{
		{Cause: "overly broad rbac grant", RecommendedAction: candorv1alpha1.ActionNotify},
	}}

	if r := Score(f, resp, nil); !r.CauseCorrect {
		t.Error("CauseCorrect = false, want true (case-insensitive substring match)")
	}
}

func TestScore_ErrorScoresBothFalse(t *testing.T) {
	f := fixture(candorv1alpha1.ActionNotify, "anything")

	r := Score(f, llm.Response{}, errors.New("rate limited"))

	if r.ActionCorrect || r.CauseCorrect {
		t.Errorf("Score = %+v, want both false on error", r)
	}
	if r.Err == nil {
		t.Error("Err = nil, want the original error preserved")
	}
}

func TestScore_EmptyHypothesesScoresBothFalse(t *testing.T) {
	f := fixture(candorv1alpha1.ActionNotify, "anything")

	r := Score(f, llm.Response{Hypotheses: nil}, nil)

	if r.ActionCorrect || r.CauseCorrect {
		t.Errorf("Score = %+v, want both false on no hypotheses", r)
	}
}

func TestReport_CountsAndFailuresAreVisible(t *testing.T) {
	results := []Result{
		{Fixture: fixture(candorv1alpha1.ActionProposePullRequest), ActionCorrect: true, CauseCorrect: true},
		{
			Fixture: fixture(candorv1alpha1.ActionNotify),
			Err:     errors.New("timeout"),
		},
	}

	report := Report("test-model", results)

	if !strings.Contains(report, "Action accuracy: 1/2") {
		t.Errorf("report missing action accuracy line:\n%s", report)
	}
	if !strings.Contains(report, "Cause accuracy: 1/2") {
		t.Errorf("report missing cause accuracy line:\n%s", report)
	}
	if !strings.Contains(report, "timeout") {
		t.Errorf("report does not surface the fixture's error:\n%s", report)
	}
}

// runOf builds a RunSummary over the real fixture list where the first actionPasses fixtures
// score correct on action, and every fixture reports the given tokens.
func runOf(actionPasses int, in, out int, elapsed time.Duration) RunSummary {
	results := make([]Result, len(Fixtures))
	for i, f := range Fixtures {
		results[i] = Result{Fixture: f, ActionCorrect: i < actionPasses,
			Response: llm.Response{Usage: llm.Usage{Provider: "ollama", InputTokens: in, OutputTokens: out}}}
	}
	return RunSummary{Results: results, Elapsed: elapsed}
}

func TestReportRuns_ShowsSpreadErrorsTokensAndPerFixturePasses(t *testing.T) {
	all, half := len(Fixtures), len(Fixtures)/2
	r1 := runOf(all, 100, 40, 2*time.Minute)
	r2 := runOf(half, 200, 60, 4*time.Minute)
	r2.Results[0].Err = errors.New("timeout")
	r2.Results[0].ActionCorrect = false // what Score does for an errored call

	report := ReportRuns("ollama:test", []RunSummary{r1, r2})

	for _, want := range []string{
		"(2 runs)",
		"Action accuracy: 69% mean (38% to 100%) over 2 runs",
		"Errored calls: 1 of 16",
		"1200 input and 400 output tokens", // per-run totals across the fixtures, averaged over runs
		"3m0s wall time",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	// The first fixture passed action in run 1 only: run 2's call for it errored.
	if !strings.Contains(report, "| "+Fixtures[0].Name+" | 1/2 |") {
		t.Errorf("report missing per-fixture pass count for %s:\n%s", Fixtures[0].Name, report)
	}
}

func TestReportRuns_NoRuns(t *testing.T) {
	if got := ReportRuns("m", nil); !strings.Contains(got, "No runs") {
		t.Errorf("ReportRuns with no runs = %q, want a No runs note", got)
	}
}
