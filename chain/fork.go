package chain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"toyblockchain/block"
)

var (
	ErrDifferentGenesis      = errors.New("candidate chain has a different genesis block")
	ErrIncompatibleConsensus = errors.New("candidate chain uses incompatible consensus settings")
	ErrCandidateNotStronger  = errors.New("candidate chain does not have greater accumulated work")
)

// ForkResolutionResult describes the result of replacing the local chain
// with a stronger valid candidate chain.
type ForkResolutionResult struct {
	OldHeight            int
	NewHeight            int
	CommonAncestorHeight int
	OldWork              string
	NewWork              string
	PendingKept          int
	PendingDropped       int
}

// TotalWork returns the approximate accumulated Proof-of-Work of the chain.
//
// A hexadecimal leading-zero difficulty of d has expected work 16^d.
// math/big is used because the result can exceed normal integer sizes.
func (c *Chain) TotalWork() *big.Int {
	total := new(big.Int)
	base := big.NewInt(16)

	for _, currentBlock := range c.Blocks {
		if currentBlock == nil || currentBlock.Difficulty < 0 {
			continue
		}

		blockWork := new(big.Int).Exp(
			base,
			big.NewInt(int64(currentBlock.Difficulty)),
			nil,
		)

		total.Add(total, blockWork)
	}

	return total
}

// CommonAncestorHeight returns the highest height at which both chains store
// the same block hash. It returns -1 when even genesis differs.
func (c *Chain) CommonAncestorHeight(candidate *Chain) int {
	if c == nil || candidate == nil {
		return -1
	}

	limit := len(c.Blocks)
	if len(candidate.Blocks) < limit {
		limit = len(candidate.Blocks)
	}

	ancestor := -1

	for index := 0; index < limit; index++ {
		left := c.Blocks[index]
		right := candidate.Blocks[index]

		if left == nil || right == nil || left.Hash != right.Hash {
			break
		}

		ancestor = left.Height
	}

	return ancestor
}

// ResolveFork adopts a fully valid candidate chain only when it belongs to
// the same toy network and contains strictly more accumulated work.
//
// Confirmed transactions from the losing branch are not automatically put
// back into the pending pool. Existing pending transactions from both chain
// files are merged, deduplicated and revalidated against the winning chain.
func (c *Chain) ResolveFork(candidate *Chain) (*ForkResolutionResult, error) {
	if c == nil {
		return nil, fmt.Errorf("current chain is nil")
	}
	if candidate == nil {
		return nil, fmt.Errorf("candidate chain is nil")
	}

	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("current chain is invalid: %w", err)
	}
	if err := candidate.Validate(); err != nil {
		return nil, fmt.Errorf("candidate chain is invalid: %w", err)
	}

	if len(c.Blocks) == 0 || len(candidate.Blocks) == 0 ||
		c.Blocks[0].Hash != candidate.Blocks[0].Hash {
		return nil, ErrDifferentGenesis
	}

	if c.Retarget != candidate.Retarget ||
		c.MaxTxPerBlock != candidate.MaxTxPerBlock ||
		!sameDifficultySchedule(c.DifficultySchedule, candidate.DifficultySchedule) {
		return nil, ErrIncompatibleConsensus
	}

	oldWork := c.TotalWork()
	newWork := candidate.TotalWork()

	if newWork.Cmp(oldWork) <= 0 {
		return nil, ErrCandidateNotStronger
	}

	commonAncestor := c.CommonAncestorHeight(candidate)
	oldHeight := c.Latest().Height

	candidateCopy, err := cloneChain(candidate)
	if err != nil {
		return nil, fmt.Errorf("copying candidate chain: %w", err)
	}

	pendingCandidates := append(
		[]block.Transaction(nil),
		candidateCopy.Pending...,
	)
	pendingCandidates = append(pendingCandidates, c.Pending...)
	candidateCopy.Pending = nil

	kept, dropped := revalidatePending(candidateCopy, pendingCandidates)

	c.Blocks = candidateCopy.Blocks
	c.Pending = candidateCopy.Pending
	c.DifficultySchedule = candidateCopy.DifficultySchedule
	c.Retarget = candidateCopy.Retarget
	c.MaxTxPerBlock = candidateCopy.MaxTxPerBlock

	return &ForkResolutionResult{
		OldHeight:            oldHeight,
		NewHeight:            c.Latest().Height,
		CommonAncestorHeight: commonAncestor,
		OldWork:              oldWork.String(),
		NewWork:              newWork.String(),
		PendingKept:          kept,
		PendingDropped:       dropped,
	}, nil
}

func sameDifficultySchedule(left, right []DifficultyRule) bool {
	leftCopy := append([]DifficultyRule(nil), left...)
	rightCopy := append([]DifficultyRule(nil), right...)

	sort.Slice(leftCopy, func(i, j int) bool {
		return leftCopy[i].StartHeight < leftCopy[j].StartHeight
	})
	sort.Slice(rightCopy, func(i, j int) bool {
		return rightCopy[i].StartHeight < rightCopy[j].StartHeight
	})

	if len(leftCopy) != len(rightCopy) {
		return false
	}

	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}

	return true
}

func cloneChain(source *Chain) (*Chain, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}

	var clone Chain
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}

	return &clone, nil
}

func revalidatePending(target *Chain, transactions []block.Transaction) (int, int) {
	seen := make(map[string]struct{})
	kept := 0
	dropped := 0

	for _, transaction := range transactions {
		identifier := transactionIdentifier(transaction)
		if _, exists := seen[identifier]; exists {
			dropped++
			continue
		}
		seen[identifier] = struct{}{}

		if err := target.AddTransaction(transaction); err != nil {
			dropped++
			continue
		}

		kept++
	}

	return kept, dropped
}

func transactionIdentifier(transaction block.Transaction) string {
	data, err := json.Marshal(transaction)
	if err != nil {
		return fmt.Sprintf("unmarshalable:%+v", transaction)
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
