package engine

import (
	"testing"

	"github.com/atakankarsli/sure/internal/contracts"
)

func TestLongContextLimitsAreFloors(t *testing.T) {
	std := contracts.DefaultConfig().Limits
	got := longContextLimits(std)
	if got.MaxStateBytes != LongContextStateBytes || got.MaxRequestBytes != LongContextRequestBytes || got.MaxEstimatedTokens != LongContextEstimatedTokens {
		t.Fatalf("defaults not raised: %+v", got)
	}
	if got.MaxFileBytes != std.MaxFileBytes || got.Concurrency != std.Concurrency {
		t.Fatalf("unrelated limits changed: %+v", got)
	}
	big := std
	big.MaxStateBytes, big.MaxRequestBytes, big.MaxEstimatedTokens = 1<<20, 2<<20, 900_000
	if longContextLimits(big) != big {
		t.Fatalf("explicit larger limits must win: %+v", longContextLimits(big))
	}
}
