// Package chain manages the blockchain, pending transactions,
// difficulty policy, validation and safe JSON persistence.
package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"toyblockchain/block"
	"toyblockchain/ledger"
)

const DefaultDifficulty = 4

const DefaultMaxTxPerBlock = 10

// DifficultyRule records the height at which a difficulty becomes active.
//
// Example:
//
//	{StartHeight: 0, Difficulty: 4}
//	{StartHeight: 5, Difficulty: 6}
//
// Blocks 0-4 use difficulty 4.
// Block 5 and future blocks use difficulty 6.
type DifficultyRule struct {
	StartHeight int `json:"start_height"`
	Difficulty  int `json:"difficulty"`
}

// Chain is the persisted blockchain state.
type Chain struct {
	Blocks             []*block.Block      `json:"blocks"`
	Pending            []block.Transaction `json:"pending"`
	DifficultySchedule []DifficultyRule    `json:"difficulty_schedule"`
	MaxTxPerBlock      int                 `json:"max_tx_per_block"`
}

// New creates a new chain and mines its genesis block.
func New(
	ctx context.Context,
	difficulty int,
	maxTxPerBlock int,
	limits block.MiningLimits,
) (*Chain, error) {
	if difficulty < 0 {
		difficulty = DefaultDifficulty
	}

	if maxTxPerBlock <= 0 {
		maxTxPerBlock = DefaultMaxTxPerBlock
	}

	genesis, err := block.NewGenesisBlock(
		ctx,
		difficulty,
		limits,
	)
	if err != nil {
		return nil, err
	}

	return &Chain{
		Blocks:  []*block.Block{genesis},
		Pending: []block.Transaction{},
		DifficultySchedule: []DifficultyRule{
			{
				StartHeight: 0,
				Difficulty:  difficulty,
			},
		},
		MaxTxPerBlock: maxTxPerBlock,
	}, nil
}

// Latest returns the blockchain tip.
func (c *Chain) Latest() *block.Block {
	return c.Blocks[len(c.Blocks)-1]
}

// Ledger derives the current ledger from confirmed blocks.
func (c *Chain) Ledger() (*ledger.Ledger, error) {
	return ledger.Rebuild(c.Blocks)
}

// ExpectedDifficulty returns the policy difficulty for a block height.
func (c *Chain) ExpectedDifficulty(height int) int {
	rules := append(
		[]DifficultyRule(nil),
		c.DifficultySchedule...,
	)

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].StartHeight < rules[j].StartHeight
	})

	difficulty := DefaultDifficulty

	for _, rule := range rules {
		if rule.StartHeight > height {
			break
		}

		difficulty = rule.Difficulty
	}

	return difficulty
}

// SetDifficulty schedules a new difficulty from the next block.
//
// Existing blocks retain their original required difficulty.
// The new value applies to the next block and all future blocks until
// another difficulty rule is added.
func (c *Chain) SetDifficulty(difficulty int) error {
	if difficulty < 0 || difficulty > 64 {
		return fmt.Errorf(
			"difficulty must be between 0 and 64",
		)
	}

	startHeight := c.Latest().Height + 1

	for index := range c.DifficultySchedule {
		if c.DifficultySchedule[index].StartHeight == startHeight {
			c.DifficultySchedule[index].Difficulty = difficulty
			return nil
		}
	}

	c.DifficultySchedule = append(
		c.DifficultySchedule,
		DifficultyRule{
			StartHeight: startHeight,
			Difficulty:  difficulty,
		},
	)

	return nil
}

// AddTransaction validates confirmed and pending transaction state.
func (c *Chain) AddTransaction(tx block.Transaction) error {
	currentLedger, err := c.Ledger()
	if err != nil {
		return fmt.Errorf("rebuilding ledger: %w", err)
	}

	// Include pending transactions to prevent pending-pool double spending
	// and duplicate nonces.
	for _, pendingTransaction := range c.Pending {
		if err := currentLedger.Apply(pendingTransaction); err != nil {
			return fmt.Errorf(
				"invalid pending pool: %w",
				err,
			)
		}
	}

	if err := currentLedger.ValidateTransaction(tx); err != nil {
		return err
	}

	c.Pending = append(c.Pending, tx)

	return nil
}

// MineBlock mines a block using the policy difficulty expected at its height.
//
// Mining failure does not remove pending transactions and does not append
// a partially mined block.
func (c *Chain) MineBlock(
	ctx context.Context,
	limits block.MiningLimits,
) (*block.Block, block.MineResult, error) {
	if len(c.Pending) == 0 {
		return nil, block.MineResult{}, fmt.Errorf(
			"no pending transactions to mine",
		)
	}

	transactionCount := len(c.Pending)

	if transactionCount > c.MaxTxPerBlock {
		transactionCount = c.MaxTxPerBlock
	}

	batch := append(
		[]block.Transaction(nil),
		c.Pending[:transactionCount]...,
	)

	previousBlock := c.Latest()
	newHeight := previousBlock.Height + 1

	expectedDifficulty := c.ExpectedDifficulty(newHeight)

	newBlock := block.NewBlock(
		newHeight,
		batch,
		previousBlock.Hash,
		expectedDifficulty,
	)

	result, err := newBlock.Mine(ctx, limits)
	if err != nil {
		return nil, block.MineResult{}, err
	}

	c.Blocks = append(c.Blocks, newBlock)
	c.Pending = c.Pending[transactionCount:]

	return newBlock, result, nil
}

// ValidationError identifies the first invalid block.
type ValidationError struct {
	BlockHeight int
	Reason      string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf(
		"block %d invalid: %s",
		e.BlockHeight,
		e.Reason,
	)
}

// Validate performs complete blockchain and ledger validation.
func (c *Chain) Validate() error {
	if len(c.Blocks) == 0 {
		return &ValidationError{
			BlockHeight: -1,
			Reason:      "chain is empty",
		}
	}

	if len(c.DifficultySchedule) == 0 {
		return &ValidationError{
			BlockHeight: -1,
			Reason:      "difficulty schedule is empty",
		}
	}

	currentLedger := ledger.New()

	for index, currentBlock := range c.Blocks {
		expectedDifficulty := c.ExpectedDifficulty(
			currentBlock.Height,
		)

		// This prevents an attacker from changing a block's recorded
		// difficulty to a lower value.
		if currentBlock.Difficulty != expectedDifficulty {
			return &ValidationError{
				BlockHeight: currentBlock.Height,
				Reason: fmt.Sprintf(
					"recorded difficulty %d does not match expected difficulty %d",
					currentBlock.Difficulty,
					expectedDifficulty,
				),
			}
		}

		recomputedHash := currentBlock.ComputeHash()

		if recomputedHash != currentBlock.Hash {
			return &ValidationError{
				BlockHeight: currentBlock.Height,
				Reason:      "stored hash does not match recomputed hash",
			}
		}

		if !block.MeetsDifficulty(
			currentBlock.Hash,
			expectedDifficulty,
		) {
			return &ValidationError{
				BlockHeight: currentBlock.Height,
				Reason:      "hash does not meet expected difficulty",
			}
		}

		if index == 0 {
			if currentBlock.Height != 0 {
				return &ValidationError{
					BlockHeight: currentBlock.Height,
					Reason:      "genesis height must be 0",
				}
			}

			if currentBlock.PrevHash != block.GenesisPrevHash {
				return &ValidationError{
					BlockHeight: currentBlock.Height,
					Reason:      "wrong genesis previous hash",
				}
			}
		} else {
			previousBlock := c.Blocks[index-1]

			if currentBlock.Height != previousBlock.Height+1 {
				return &ValidationError{
					BlockHeight: currentBlock.Height,
					Reason:      "invalid height sequence",
				}
			}

			if currentBlock.Timestamp < previousBlock.Timestamp {
				return &ValidationError{
					BlockHeight: currentBlock.Height,
					Reason:      "timestamp earlier than previous block",
				}
			}

			if currentBlock.PrevHash != previousBlock.Hash {
				return &ValidationError{
					BlockHeight: currentBlock.Height,
					Reason:      "previous hash link mismatch",
				}
			}
		}

		// Replaying transactions validates:
		// - signatures,
		// - ownership,
		// - balances,
		// - transaction nonces,
		// - replay protection.
		if err := currentLedger.ApplyBlock(currentBlock); err != nil {
			return &ValidationError{
				BlockHeight: currentBlock.Height,
				Reason:      err.Error(),
			}
		}
	}

	return nil
}

// Save performs an atomic JSON save:
//
//  1. write chain.json.tmp,
//  2. flush it to disk,
//  3. rename it over chain.json.
//
// This prevents partially written JSON files.
func (c *Chain) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling chain: %w", err)
	}

	directory := filepath.Dir(path)

	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf(
			"creating chain directory: %w",
			err,
		)
	}

	temporaryPath := path + ".tmp"

	file, err := os.OpenFile(
		temporaryPath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0644,
	)
	if err != nil {
		return fmt.Errorf(
			"creating temporary chain file: %w",
			err,
		)
	}

	saveCompleted := false

	defer func() {
		_ = file.Close()

		if !saveCompleted {
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf(
			"writing temporary chain file: %w",
			err,
		)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf(
			"syncing temporary chain file: %w",
			err,
		)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf(
			"closing temporary chain file: %w",
			err,
		)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf(
			"atomically replacing chain file: %w",
			err,
		)
	}

	saveCompleted = true

	return nil
}

// Load reads and validates a saved blockchain.
func Load(path string) (*Chain, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var loadedChain Chain

	if err := json.Unmarshal(data, &loadedChain); err != nil {
		return nil, fmt.Errorf(
			"parsing chain file: %w",
			err,
		)
	}

	if err := loadedChain.Validate(); err != nil {
		return nil, fmt.Errorf(
			"loaded chain is invalid: %w",
			err,
		)
	}

	return &loadedChain, nil
}

// FileLock prevents two application processes from editing the same JSON
// blockchain simultaneously.
type FileLock struct {
	path string
}

// AcquireFileLock waits for exclusive ownership of path+".lock".
func AcquireFileLock(
	path string,
	timeout time.Duration,
) (*FileLock, error) {
	lockPath := path + ".lock"
	deadline := time.Now().Add(timeout)

	for {
		file, err := os.OpenFile(
			lockPath,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			0600,
		)

		if err == nil {
			_, _ = fmt.Fprintf(
				file,
				"pid=%d\n",
				os.Getpid(),
			)

			_ = file.Close()

			return &FileLock{
				path: lockPath,
			}, nil
		}

		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf(
				"creating chain lock: %w",
				err,
			)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"chain file is locked by another process",
			)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Release releases the process-level JSON file lock.
func (l *FileLock) Release() error {
	if l == nil {
		return nil
	}

	return os.Remove(l.path)
}
