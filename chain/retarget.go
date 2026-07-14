package chain

import "fmt"

// RetargetConfig controls automatic difficulty adjustment.
//
// The first automatic retarget happens before block Interval+1. This avoids
// using the deterministic genesis timestamp (zero) in the timing window.
type RetargetConfig struct {
	Enabled            bool  `json:"enabled"`
	Interval           int   `json:"interval"`
	TargetBlockSeconds int64 `json:"target_block_seconds"`
	MinDifficulty      int   `json:"min_difficulty"`
	MaxDifficulty      int   `json:"max_difficulty"`
}

// DefaultRetargetConfig preserves the existing manual-difficulty behaviour.
// Users may enable automatic retargeting when creating a new chain.
var DefaultRetargetConfig = RetargetConfig{
	Enabled:            false,
	Interval:           5,
	TargetBlockSeconds: 5,
	MinDifficulty:      1,
	MaxDifficulty:      8,
}

// Validate checks whether the retarget policy is usable and deterministic.
func (r RetargetConfig) Validate() error {
	if r.Interval < 2 {
		return fmt.Errorf("retarget interval must be at least 2 blocks")
	}
	if r.TargetBlockSeconds <= 0 {
		return fmt.Errorf("target block time must be greater than 0 seconds")
	}
	if r.MinDifficulty < 0 || r.MinDifficulty > 64 {
		return fmt.Errorf("minimum difficulty must be between 0 and 64")
	}
	if r.MaxDifficulty < 0 || r.MaxDifficulty > 64 {
		return fmt.Errorf("maximum difficulty must be between 0 and 64")
	}
	if r.MinDifficulty > r.MaxDifficulty {
		return fmt.Errorf("minimum difficulty must not exceed maximum difficulty")
	}
	return nil
}

// ConfigureRetarget sets the automatic retarget policy.
// It is intentionally allowed only before normal blocks are mined because
// changing this policy later would change the expected rules for history.
func (c *Chain) ConfigureRetarget(config RetargetConfig) error {
	if len(c.Blocks) != 1 || c.Blocks[0].Height != 0 {
		return fmt.Errorf("retarget configuration can only be changed before mining the first normal block")
	}
	if err := config.Validate(); err != nil {
		return err
	}
	c.Retarget = config
	return nil
}

// NextRetargetHeight returns the next block height where automatic
// retargeting will be calculated. A return value of -1 means disabled.
func (c *Chain) NextRetargetHeight() int {
	if !c.Retarget.Enabled {
		return -1
	}

	interval := c.Retarget.Interval
	nextHeight := c.Latest().Height + 1
	firstRetargetHeight := interval + 1

	if nextHeight <= firstRetargetHeight {
		return firstRetargetHeight
	}

	offset := (nextHeight - firstRetargetHeight) % interval
	if offset == 0 {
		return nextHeight
	}

	return nextHeight + (interval - offset)
}

// shouldRetarget reports whether the supplied block height starts a new
// automatic-difficulty period.
func (c *Chain) shouldRetarget(height int) bool {
	if !c.Retarget.Enabled {
		return false
	}
	if height <= c.Retarget.Interval {
		return false
	}
	return (height-1)%c.Retarget.Interval == 0
}

// retargetDifficulty calculates the difficulty for block height using the
// previous complete timing window.
func (c *Chain) retargetDifficulty(height, current int) int {
	windowStart := height - c.Retarget.Interval
	windowEnd := height - 1

	// During mining and validation, all previous blocks must already exist.
	if windowStart < 1 || windowEnd >= len(c.Blocks) {
		return current
	}

	first := c.Blocks[windowStart]
	last := c.Blocks[windowEnd]

	actualSeconds := last.Timestamp - first.Timestamp
	expectedSeconds := int64(c.Retarget.Interval-1) * c.Retarget.TargetBlockSeconds

	next := current

	// Less than half the target time means blocks were too fast.
	if actualSeconds < expectedSeconds/2 {
		next++
	} else if actualSeconds > expectedSeconds*2 {
		// More than twice the target time means blocks were too slow.
		next--
	}

	if next < c.Retarget.MinDifficulty {
		next = c.Retarget.MinDifficulty
	}
	if next > c.Retarget.MaxDifficulty {
		next = c.Retarget.MaxDifficulty
	}

	return next
}
