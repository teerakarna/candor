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

package controller

import (
	"math"
	"strings"
	"testing"

	"github.com/teerakarna/candor/internal/llm"
)

func TestToHypotheses(t *testing.T) {
	const firstThree = "a,b,c"
	h := func(cause string, conf float64, action string) llm.Hypothesis {
		return llm.Hypothesis{Cause: cause, Confidence: conf, Rationale: "r", RecommendedAction: action}
	}
	tests := []struct {
		name       string
		in         []llm.Hypothesis
		wantCauses string
	}{
		{"empty", nil, ""},
		{"all valid, within cap", []llm.Hypothesis{h("a", 0.9, "Notify"), h("b", 0, "Notify"), h("c", 1, "Notify")}, firstThree},
		{"over cap keeps the first three", []llm.Hypothesis{h("a", 0.9, ""), h("b", 0.5, ""), h("c", 0.3, ""), h("d", 0.1, "")}, firstThree},
		{"invalid lower entries dropped, cap applies to what is kept",
			[]llm.Hypothesis{h("a", 0.8, ""), h("neg", -0.1, ""), h("b", 0.4, ""), h("big", 85, ""), h("c", 0.2, ""), h("d", 0.1, "")}, firstThree},
		// tryProposePullRequest reads Hypotheses[0] alone, so a lower ProposePullRequest must never
		// move up into that slot when the model's own first choice is dropped.
		{"invalid first entry keeps nothing",
			[]llm.Hypothesis{h("top", 90, "Notify"), h("second", 0.3, "ProposePullRequest"), h("third", 0.2, "Notify")}, ""},
		{"NaN is invalid", []llm.Hypothesis{h("a", 0.5, ""), h("nan", math.NaN(), "")}, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := toHypotheses(tt.in)
			causes := make([]string, 0, len(out))
			for _, o := range out {
				causes = append(causes, o.Cause)
				if o.Confidence < 0 || o.Confidence > 100 {
					t.Errorf("%s: confidence %d outside the CRD's 0-100", o.Cause, o.Confidence)
				}
			}
			if got := strings.Join(causes, ","); got != tt.wantCauses {
				t.Errorf("causes = %q, want %q", got, tt.wantCauses)
			}
		})
	}
}
