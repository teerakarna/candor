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
	"strings"
	"testing"
)

func TestConfiguredBackends_NoneSetReturnsEmpty(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CANDOR_OLLAMA_HOST", "")

	if got := configuredBackends(); len(got) != 0 {
		t.Errorf("configuredBackends() = %v, want empty with no credentials set", got)
	}
}

func TestConfiguredBackends_AnthropicOnlyIsBackwardCompatible(t *testing.T) {
	// A run with only ANTHROPIC_API_KEY set must behave exactly as it did before this comparison
	// existed - one backend, named after the model actually used.
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("CANDOR_OLLAMA_HOST", "")

	got := configuredBackends()

	if len(got) != 1 {
		t.Fatalf("configuredBackends() = %d backends, want 1", len(got))
	}
	if !strings.HasPrefix(got[0].name, "anthropic:") {
		t.Errorf("name = %q, want an anthropic: prefix", got[0].name)
	}
}

func TestConfiguredBackends_OllamaRequiresExplicitHost(t *testing.T) {
	// No silent attempt against localhost:11434 for a run that only asked for the hosted baseline.
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("CANDOR_OLLAMA_HOST", "")

	for _, b := range configuredBackends() {
		if strings.HasPrefix(b.name, "ollama:") {
			t.Errorf("got an ollama backend with CANDOR_OLLAMA_HOST unset: %q", b.name)
		}
	}
}

func TestConfiguredBackends_BothSetRunsBoth(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("CANDOR_OLLAMA_HOST", "http://localhost:11434")
	t.Setenv("CANDOR_LLM_MODEL", "")

	got := configuredBackends()

	if len(got) != 2 {
		t.Fatalf("configuredBackends() = %d backends, want 2", len(got))
	}
	if !strings.HasPrefix(got[1].name, "ollama:") {
		t.Errorf("name = %q, want an ollama: prefix", got[1].name)
	}
}

func TestConfiguredBackends_CustomOllamaModelIsReflectedInName(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CANDOR_OLLAMA_HOST", "http://localhost:11434")
	t.Setenv("CANDOR_LLM_MODEL", "qwen2.5:7b")

	got := configuredBackends()

	if len(got) != 1 || got[0].name != "ollama:qwen2.5:7b" {
		t.Errorf("configuredBackends() = %v, want a single ollama:qwen2.5:7b backend", got)
	}
}
