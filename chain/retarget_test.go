package chain

import (
	"context"
	"testing"

	"toyblockchain/block"
)

func newRetargetTestChain(t *testing.T, difficulty int, config RetargetConfig) *Chain {
	t.Helper()

	chain, err := NewWithRetarget(
		context.Background(),
		difficulty,
		10,
		block.MiningLimits{
			MaxAttempts: 2_000_000,
			MaxNonce:    2_000_000,
			Workers:     1,
		},
		config,
	)
	if err != nil {
		t.Fatalf("creating retarget test chain: %v", err)
	}

	return chain
}

// addTimingBlocks appends simple timing-only blocks for testing the
// deterministic expected-difficulty calculation. These blocks are not used
// for full chain validation; no mining is needed for this policy unit test.
func addTimingBlocks(c *Chain, timestamps ...int64) {
	for index, timestamp := range timestamps {
		height := index + 1
		c.Blocks = append(c.Blocks, &block.Block{
			Height:     height,
			Timestamp:  timestamp,
			Difficulty: c.ExpectedDifficulty(height),
		})
	}
}

func TestDifficultyRetargetIncreasesWhenBlocksAreTooFast(t *testing.T) {
	config := RetargetConfig{
		Enabled:            true,
		Interval:           5,
		TargetBlockSeconds: 5,
		MinDifficulty:      1,
		MaxDifficulty:      8,
	}

	c := newRetargetTestChain(t, 3, config)
	addTimingBlocks(c, 100, 101, 102, 103, 104)

	if got := c.ExpectedDifficulty(6); got != 4 {
		t.Fatalf("expected difficulty 4 for fast blocks, got %d", got)
	}
}

func TestDifficultyRetargetDecreasesWhenBlocksAreTooSlow(t *testing.T) {
	config := RetargetConfig{
		Enabled:            true,
		Interval:           5,
		TargetBlockSeconds: 5,
		MinDifficulty:      1,
		MaxDifficulty:      8,
	}

	c := newRetargetTestChain(t, 3, config)
	addTimingBlocks(c, 100, 120, 140, 160, 180)

	if got := c.ExpectedDifficulty(6); got != 2 {
		t.Fatalf("expected difficulty 2 for slow blocks, got %d", got)
	}
}

func TestDifficultyRetargetKeepsDifficultyNearTarget(t *testing.T) {
	config := RetargetConfig{
		Enabled:            true,
		Interval:           5,
		TargetBlockSeconds: 5,
		MinDifficulty:      1,
		MaxDifficulty:      8,
	}

	c := newRetargetTestChain(t, 3, config)
	addTimingBlocks(c, 100, 105, 110, 115, 120)

	if got := c.ExpectedDifficulty(6); got != 3 {
		t.Fatalf("expected difficulty to remain 3, got %d", got)
	}
}

func TestDifficultyRetargetRespectsBounds(t *testing.T) {
	config := RetargetConfig{
		Enabled:            true,
		Interval:           5,
		TargetBlockSeconds: 5,
		MinDifficulty:      2,
		MaxDifficulty:      3,
	}

	fast := newRetargetTestChain(t, 3, config)
	addTimingBlocks(fast, 100, 101, 102, 103, 104)
	if got := fast.ExpectedDifficulty(6); got != 3 {
		t.Fatalf("expected maximum difficulty 3, got %d", got)
	}

	slow := newRetargetTestChain(t, 2, config)
	addTimingBlocks(slow, 100, 120, 140, 160, 180)
	if got := slow.ExpectedDifficulty(6); got != 2 {
		t.Fatalf("expected minimum difficulty 2, got %d", got)
	}
}

func TestManualDifficultyRuleOverridesRetargetAtSameHeight(t *testing.T) {
	config := RetargetConfig{
		Enabled:            true,
		Interval:           5,
		TargetBlockSeconds: 5,
		MinDifficulty:      1,
		MaxDifficulty:      8,
	}

	c := newRetargetTestChain(t, 3, config)
	addTimingBlocks(c, 100, 101, 102, 103, 104)

	c.DifficultySchedule = append(c.DifficultySchedule, DifficultyRule{
		StartHeight: 6,
		Difficulty:  7,
	})

	if got := c.ExpectedDifficulty(6); got != 7 {
		t.Fatalf("expected manual difficulty 7 to override retarget, got %d", got)
	}
}

func TestRetargetConfigurationCannotChangeAfterMiningStarts(t *testing.T) {
	c := newRetargetTestChain(t, 3, DefaultRetargetConfig)
	addTimingBlocks(c, 100)

	err := c.ConfigureRetarget(RetargetConfig{
		Enabled:            true,
		Interval:           5,
		TargetBlockSeconds: 5,
		MinDifficulty:      1,
		MaxDifficulty:      8,
	})
	if err == nil {
		t.Fatal("expected retarget configuration change to be rejected")
	}
}
