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

// Command eval runs this directory's fixture suite (issue #106) against every configured LLM
// backend and publishes a combined report. Dev/benchmark tool only, run via `make eval` - never
// part of `make test`, `make build`, or CI: it needs real credentials and/or a real local Ollama
// instance, spends real API credits or wall time per invocation, and its purpose is a measured,
// published number (docs/design.md evidence gap #7), not a pass/fail gate. See README.md for what
// running it means and why.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/teerakarna/candor/internal/llm"
	"github.com/teerakarna/candor/internal/llm/anthropic"
	"github.com/teerakarna/candor/internal/llm/ollama"
)

const (
	reportPath = "hack/eval/report.md"

	// Shared across backends, sized for Ollama's documented CPU-inference reality (its own
	// client.go notes a 3B model takes several seconds per call), not Anthropic's faster default -
	// harmless for the faster backend, necessary for the slower one.
	callTimeout = 120 * time.Second
)

// namedBackend pairs a client with the name it's reported under - "<provider>:<model>", not just
// the provider, since which model answered is exactly what issue #107 exists to compare.
type namedBackend struct {
	name   string
	client llm.Client
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run() error {
	backends := configuredBackends()
	if len(backends) == 0 {
		return fmt.Errorf("no backend configured - set ANTHROPIC_API_KEY and/or CANDOR_OLLAMA_HOST, see hack/eval/README.md")
	}

	ctx := context.Background()
	var reports []string
	for _, b := range backends {
		reports = append(reports, Report(b.name, runFixtures(ctx, b.client)))
	}

	combined := strings.Join(reports, "\n")
	fmt.Print(combined)

	return os.WriteFile(reportPath, []byte(combined), 0o644)
}

// configuredBackends returns one entry per backend with real credentials/config present. Each is
// independently opt-in - running with only ANTHROPIC_API_KEY set behaves exactly as it did before
// this comparison existed; CANDOR_OLLAMA_HOST must be set explicitly to opt into also running the
// Ollama candidate (no silent attempt against localhost:11434 for a run that only wanted the
// hosted baseline).
func configuredBackends() []namedBackend {
	var backends []namedBackend

	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		backends = append(backends, namedBackend{
			name:   "anthropic:" + anthropic.DefaultModel,
			client: anthropic.New(apiKey),
		})
	}

	if host := os.Getenv("CANDOR_OLLAMA_HOST"); host != "" {
		model := ollama.DefaultModel
		var opts []ollama.Option
		if m := os.Getenv("CANDOR_LLM_MODEL"); m != "" {
			model = m
			opts = append(opts, ollama.WithModel(m))
		}
		backends = append(backends, namedBackend{
			name:   "ollama:" + model,
			client: ollama.New(host, opts...),
		})
	}

	return backends
}

func runFixtures(ctx context.Context, client llm.Client) []Result {
	results := make([]Result, 0, len(Fixtures))
	for _, f := range Fixtures {
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		resp, err := client.Enrich(callCtx, f.Request)
		cancel()
		results = append(results, Score(f, resp, err))
	}
	return results
}
