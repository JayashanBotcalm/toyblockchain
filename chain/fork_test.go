package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"toyblockchain/block"
)

func newForkTestChain(t *testing.T, difficulty int) *Chain {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	currentChain, err := NewWithRetarget(
		ctx,
		difficulty,
		10,
		block.MiningLimits{MaxAttempts: 3_000_000, MaxNonce: 3_000_000, Workers: 2},
		RetargetConfig{
			Enabled:            false,
			Interval:           5,
			TargetBlockSeconds: 5,
			MinDifficulty:      0,
			MaxDifficulty:      8,
		},
	)
	if err != nil {
		t.Fatalf("creating test chain: %v", err)
	}

	return currentChain
}

func mineForkTestBlock(t *testing.T, currentChain *Chain, recipient string, amount int64) {
	t.Helper()

	if err := currentChain.AddTransaction(block.Transaction{
		Sender:    block.FaucetAccount,
		Recipient: recipient,
		Amount:    amount,
		Nonce:     0,
	}); err != nil {
		t.Fatalf("adding transaction: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, _, err := currentChain.MineBlock(
		ctx,
		block.MiningLimits{MaxAttempts: 3_000_000, MaxNonce: 3_000_000, Workers: 2},
	); err != nil {
		t.Fatalf("mining test block: %v", err)
	}
}

func TestTotalWorkIncreasesWithBlocks(t *testing.T) {
	currentChain := newForkTestChain(t, 1)
	before := currentChain.TotalWork()

	mineForkTestBlock(t, currentChain, "alice", 10)
	after := currentChain.TotalWork()

	if after.Cmp(before) <= 0 {
		t.Fatalf("expected total work to increase: before=%s after=%s", before, after)
	}
}

func TestResolveForkAdoptsStrongerCandidate(t *testing.T) {
	current := newForkTestChain(t, 1)
	candidate := newForkTestChain(t, 1)

	mineForkTestBlock(t, current, "alice", 10)
	mineForkTestBlock(t, candidate, "bob", 10)
	mineForkTestBlock(t, candidate, "carol", 10)

	result, err := current.ResolveFork(candidate)
	if err != nil {
		t.Fatalf("resolving stronger fork: %v", err)
	}

	if result.OldHeight != 1 || result.NewHeight != 2 {
		t.Fatalf("unexpected heights: old=%d new=%d", result.OldHeight, result.NewHeight)
	}
	if result.CommonAncestorHeight != 0 {
		t.Fatalf("expected common ancestor 0, got %d", result.CommonAncestorHeight)
	}
	if current.Latest().Hash != candidate.Latest().Hash {
		t.Fatal("current chain did not adopt candidate tip")
	}
	if err := current.Validate(); err != nil {
		t.Fatalf("resolved chain should validate: %v", err)
	}
}

func TestResolveForkRejectsEqualOrWeakerCandidate(t *testing.T) {
	current := newForkTestChain(t, 1)
	candidate := newForkTestChain(t, 1)

	_, err := current.ResolveFork(candidate)
	if !errors.Is(err, ErrCandidateNotStronger) {
		t.Fatalf("expected ErrCandidateNotStronger, got %v", err)
	}
}

func TestResolveForkRejectsDifferentGenesis(t *testing.T) {
	current := newForkTestChain(t, 1)
	candidate := newForkTestChain(t, 2)

	_, err := current.ResolveFork(candidate)
	if !errors.Is(err, ErrDifferentGenesis) {
		t.Fatalf("expected ErrDifferentGenesis, got %v", err)
	}
}

func TestResolveForkRejectsInvalidCandidate(t *testing.T) {
	current := newForkTestChain(t, 1)
	candidate := newForkTestChain(t, 1)
	mineForkTestBlock(t, candidate, "alice", 10)

	candidate.Blocks[1].Transactions[0].Amount = 999

	_, err := current.ResolveFork(candidate)
	if err == nil {
		t.Fatal("expected invalid candidate to be rejected")
	}
}

func TestResolveForkRevalidatesPendingTransactions(t *testing.T) {
	current := newForkTestChain(t, 1)
	candidate := newForkTestChain(t, 1)
	mineForkTestBlock(t, candidate, "alice", 100)

	validPending := block.Transaction{
		Sender:    block.FaucetAccount,
		Recipient: "pending-account",
		Amount:    5,
		Nonce:     0,
	}
	current.Pending = append(current.Pending, validPending, validPending)

	result, err := current.ResolveFork(candidate)
	if err != nil {
		t.Fatalf("resolving fork: %v", err)
	}

	if result.PendingKept != 1 || result.PendingDropped != 1 {
		t.Fatalf("unexpected pending result: kept=%d dropped=%d", result.PendingKept, result.PendingDropped)
	}
	if len(current.Pending) != 1 {
		t.Fatalf("expected one pending transaction, got %d", len(current.Pending))
	}
}
