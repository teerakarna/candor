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

// Command eval runs this directory's fixture suite (issue #106) against the Anthropic backend
// and publishes a report. Dev/benchmark tool only, run via `make eval` - never part of `make
// test`, `make build`, or CI: it needs a real ANTHROPIC_API_KEY, spends real API credits per
// invocation, and its purpose is a measured, published number (docs/design.md evidence gap #7),
// not a pass/fail gate. See README.md for what running it means and why.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/teerakarna/candor/internal/llm/anthropic"
)

const (
	reportPath  = "hack/eval/report.md"
	callTimeout = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run() error {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is not set - see hack/eval/README.md")
	}

	client := anthropic.New(apiKey)
	ctx := context.Background()

	results := make([]Result, 0, len(Fixtures))
	for _, f := range Fixtures {
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		resp, err := client.Enrich(callCtx, f.Request)
		cancel()
		results = append(results, Score(f, resp, err))
	}

	report := Report(anthropic.DefaultModel, results)
	fmt.Print(report)

	return os.WriteFile(reportPath, []byte(report), 0o644)
}
