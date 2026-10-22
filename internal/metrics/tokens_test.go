package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

const testTokenProvider = "test-a"

func tokens(direction string) float64 {
	return testutil.ToFloat64(LLMTokensTotal.WithLabelValues(testTokenProvider, direction))
}

func TestRecordLLMTokens_AddsByProviderAndDirection(t *testing.T) {
	beforeIn, beforeOut := tokens(DirectionInput), tokens(DirectionOutput)

	RecordLLMTokens(testTokenProvider, 100, 40)
	RecordLLMTokens(testTokenProvider, 20, 5)

	if got := tokens(DirectionInput) - beforeIn; got != 120 {
		t.Errorf("input tokens increased by %v, want 120", got)
	}
	if got := tokens(DirectionOutput) - beforeOut; got != 45 {
		t.Errorf("output tokens increased by %v, want 45", got)
	}
}

func TestRecordLLMTokens_ZeroCountsAndUnnamedProviderRecordNothing(t *testing.T) {
	before := testutil.CollectAndCount(LLMTokensTotal)

	RecordLLMTokens("test-zero", 0, 0)
	RecordLLMTokens("", 50, 50)

	if got := testutil.CollectAndCount(LLMTokensTotal); got != before {
		t.Errorf("series count went from %d to %d, want no new series", before, got)
	}
}
