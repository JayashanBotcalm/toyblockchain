// Package chain ties block, ledger together into an append-only,
// proof-of-work blockchain with full validation and JSON persistence.
package chain

import (
	"encoding/json"
	"fmt"
	"os"

	"toyblockchain/block"
	"toyblockchain/ledger"
)

// DefaultDifficulty is used when no explicit difficulty is configured.
// Kept low so mining finishes in well under a second on a laptop.
const DefaultDifficulty = 4

// DefaultMaxTxPerBlock caps how many pending transactions are swept into a
// single block when mining (FR-9).
const DefaultMaxTxPerBlock = 10

// Chain is the in-memory representation of the blockchain plus its pending
// transaction pool. It is the single source of truth for both block data
// and (derived) account balances.
type Chain struct {
	Blocks        []*block.Block      `json:"blocks"`
	Pending       []block.Transaction `json:"pending"`
	Difficulty    int                 `json:"difficulty"`
	MaxTxPerBlock int                 `json:"max_tx_per_block"`
}

// New creates a fresh chain containing only the genesis block (FR-2).
func New(difficulty, maxTxPerBlock int) *Chain {
	if difficulty < 0 {
		difficulty = DefaultDifficulty
	}
	if maxTxPerBlock <= 0 {
		maxTxPerBlock = DefaultMaxTxPerBlock
	}
	genesis := block.NewGenesisBlock(difficulty)
	return &Chain{
		Blocks:        []*block.Block{genesis},
		Pending:       []block.Transaction{},
		Difficulty:    difficulty,
		MaxTxPerBlock: maxTxPerBlock,
	}
}

// Ledger rebuilds and returns the current account balances by replaying
// every block currently on the chain.
func (c *Chain) Ledger() (*ledger.Ledger, error) {
	return ledger.Rebuild(c.Blocks)
}

// Latest returns the most recently added block.
func (c *Chain) Latest() *block.Block {
	return c.Blocks[len(c.Blocks)-1]
}

// AddTransaction validates a transaction against the current ledger state
// (including any already-pending transactions, so you cannot double-spend
// within the same pending pool) and, if valid, queues it for the next
// mined block (FR-4, FR-7).
func (c *Chain) AddTransaction(tx block.Transaction) error {
	l, err := c.Ledger()
	if err != nil {
		return fmt.Errorf("rebuilding ledger: %w", err)
	}
	// Apply already-pending transactions first so a sender can't queue two
	// transactions that together overspend their balance.
	for _, p := range c.Pending {
		if err := l.Apply(p); err != nil {
			// Pending pool should already be internally consistent since we
			// checked each one at AddTransaction time; treat inconsistency
			// as a serious internal error.
			return fmt.Errorf("internal error replaying pending pool: %w", err)
		}
	}
	if err := l.ValidateTransaction(tx); err != nil {
		return err
	}
	c.Pending = append(c.Pending, tx)
	return nil
}

// MineBlock takes up to MaxTxPerBlock pending transactions, builds a new
// block on top of the current tip, and performs proof-of-work until the
// difficulty target is met (FR-5). On success the block is appended to the
// chain and the mined transactions are removed from the pending pool.
func (c *Chain) MineBlock() (*block.Block, block.MineResult, error) {
	if len(c.Pending) == 0 {
		return nil, block.MineResult{}, fmt.Errorf("no pending transactions to mine")
	}

	n := len(c.Pending)
	if n > c.MaxTxPerBlock {
		n = c.MaxTxPerBlock
	}
	batch := make([]block.Transaction, n)
	copy(batch, c.Pending[:n])

	tip := c.Latest()
	newBlock := block.NewBlock(tip.Height+1, batch, tip.Hash, c.Difficulty)
	result := newBlock.Mine(c.Difficulty)

	c.Blocks = append(c.Blocks, newBlock)
	c.Pending = c.Pending[n:]

	return newBlock, result, nil
}

// ValidationError describes precisely why chain validation failed,
// including which block was the first offender (FR-6).
type ValidationError struct {
	BlockHeight int
	Reason      string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("block %d invalid: %s", e.BlockHeight, e.Reason)
}

// Validate walks the entire chain and checks, for every block:
//  1. its stored hash matches a fresh recomputation (detects tampering with
//     any field, including transactions),
//  2. its PrevHash correctly links to the previous block's stored hash,
//  3. its hash satisfies the proof-of-work target for its recorded difficulty,
//  4. its height is exactly one more than the previous block's height, and
//  5. its timestamp is not earlier than the previous block's timestamp.
//
// It also replays every transaction through the ledger to confirm no block
// contains a transaction that would have overspent at the time it was
// mined. Validation stops and returns at the FIRST offending block, as
// required by FR-6.
func (c *Chain) Validate() error {
	if len(c.Blocks) == 0 {
		return &ValidationError{BlockHeight: -1, Reason: "chain is empty"}
	}

	genesis := c.Blocks[0]
	if genesis.Height != 0 {
		return &ValidationError{BlockHeight: genesis.Height, Reason: "genesis block must have height 0"}
	}
	if genesis.PrevHash != block.GenesisPrevHash {
		return &ValidationError{BlockHeight: genesis.Height, Reason: "genesis block has wrong PrevHash"}
	}
	if recomputed := genesis.ComputeHash(); recomputed != genesis.Hash {
		return &ValidationError{BlockHeight: genesis.Height, Reason: fmt.Sprintf("stored hash %s does not match recomputed hash %s", genesis.Hash, recomputed)}
	}
	if !block.MeetsDifficulty(genesis.Hash, genesis.Difficulty) {
		return &ValidationError{BlockHeight: genesis.Height, Reason: "genesis hash does not meet its recorded difficulty"}
	}

	l := ledger.New()
	if err := l.ApplyBlock(genesis); err != nil {
		return &ValidationError{BlockHeight: genesis.Height, Reason: err.Error()}
	}

	for i := 1; i < len(c.Blocks); i++ {
		prev := c.Blocks[i-1]
		cur := c.Blocks[i]

		if cur.Height != prev.Height+1 {
			return &ValidationError{BlockHeight: cur.Height, Reason: fmt.Sprintf("height %d is not prev height %d + 1", cur.Height, prev.Height)}
		}
		if cur.Timestamp < prev.Timestamp {
			return &ValidationError{BlockHeight: cur.Height, Reason: "timestamp is earlier than previous block"}
		}
		if cur.PrevHash != prev.Hash {
			return &ValidationError{BlockHeight: cur.Height, Reason: fmt.Sprintf("prev_hash %s does not match previous block's hash %s", cur.PrevHash, prev.Hash)}
		}
		if recomputed := cur.ComputeHash(); recomputed != cur.Hash {
			return &ValidationError{BlockHeight: cur.Height, Reason: fmt.Sprintf("stored hash %s does not match recomputed hash %s", cur.Hash, recomputed)}
		}
		if !block.MeetsDifficulty(cur.Hash, cur.Difficulty) {
			return &ValidationError{BlockHeight: cur.Height, Reason: "hash does not meet its recorded difficulty target"}
		}
		if err := l.ApplyBlock(cur); err != nil {
			return &ValidationError{BlockHeight: cur.Height, Reason: err.Error()}
		}
	}

	return nil
}

// Save writes the chain to disk as JSON (FR-8).
func (c *Chain) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling chain: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing chain file %s: %w", path, err)
	}
	return nil
}

// Load reads a chain previously written by Save. If the file does not
// exist, it returns (nil, os.ErrNotExist) so callers can distinguish
// "no saved state yet" from a real I/O error.
func Load(path string) (*Chain, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Chain
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing chain file %s: %w", path, err)
	}
	return &c, nil
}
