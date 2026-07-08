package chain

import (
	"testing"

	"toyblockchain/block"
)

// TestNewChainHasGenesis covers the "Chain starts from a deterministic
// genesis block" acceptance scenario (FR-2): a freshly initialised
// blockchain contains exactly one block at height 0, whose previous-hash
// equals the fixed genesis value.
func TestNewChainHasGenesis(t *testing.T) {
	c := New(2, 10)
	if len(c.Blocks) != 1 {
		t.Fatalf("expected exactly 1 block after New(), got %d", len(c.Blocks))
	}
	if c.Blocks[0].Height != 0 {
		t.Errorf("expected genesis height 0, got %d", c.Blocks[0].Height)
	}
	if c.Blocks[0].PrevHash != block.GenesisPrevHash {
		t.Errorf("expected genesis PrevHash %s, got %s", block.GenesisPrevHash, c.Blocks[0].PrevHash)
	}
}

// buildHonestChain constructs a small chain with a few mined blocks and
// real transactions, used as a fixture by several tests below.
func buildHonestChain(t *testing.T) *Chain {
	t.Helper()
	c := New(2, 10)

	if err := c.AddTransaction(block.Transaction{Sender: block.FaucetAccount, Recipient: "alice", Amount: 100}); err != nil {
		t.Fatalf("faucet grant should succeed: %v", err)
	}
	if _, _, err := c.MineBlock(); err != nil {
		t.Fatalf("mining block 1 failed: %v", err)
	}

	if err := c.AddTransaction(block.Transaction{Sender: "alice", Recipient: "bob", Amount: 30}); err != nil {
		t.Fatalf("alice->bob transaction should succeed: %v", err)
	}
	if _, _, err := c.MineBlock(); err != nil {
		t.Fatalf("mining block 2 failed: %v", err)
	}

	if err := c.AddTransaction(block.Transaction{Sender: "bob", Recipient: "carol", Amount: 10}); err != nil {
		t.Fatalf("bob->carol transaction should succeed: %v", err)
	}
	if _, _, err := c.MineBlock(); err != nil {
		t.Fatalf("mining block 3 failed: %v", err)
	}

	return c
}

// TestHonestChainValidates covers the "An honest chain validates
// successfully" acceptance scenario (FR-6).
func TestHonestChainValidates(t *testing.T) {
	c := buildHonestChain(t)
	if err := c.Validate(); err != nil {
		t.Fatalf("expected honest chain to validate, got error: %v", err)
	}
}

// TestTamperDetection covers the "Tampering with a block is detected"
// acceptance scenario (FR-6): modifying a transaction inside an earlier
// block must cause validation to fail and to identify the first offending
// block.
func TestTamperDetection(t *testing.T) {
	c := buildHonestChain(t)

	// Sanity check: chain is valid before tampering.
	if err := c.Validate(); err != nil {
		t.Fatalf("chain should be valid before tampering, got: %v", err)
	}

	// Tamper with the transaction amount inside block 2 (an "early" block,
	// not the tip), without recomputing its hash -- exactly what an
	// attacker editing raw data would do.
	c.Blocks[2].Transactions[0].Amount = 999999

	err := c.Validate()
	if err == nil {
		t.Fatalf("expected validation to fail after tampering, but it passed")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if ve.BlockHeight != c.Blocks[2].Height {
		t.Errorf("expected validation to flag block height %d (the tampered block), got %d",
			c.Blocks[2].Height, ve.BlockHeight)
	}
}

// TestTamperDetectionOnPrevHash checks that an attacker who edits a
// transaction AND recomputes that block's stored hash (to hide the simple
// hash-mismatch tell) still cannot get away with it: recomputing the hash
// without redoing the proof-of-work search means the new hash almost
// certainly no longer meets the block's recorded difficulty target, so
// validation still fails at the tampered block.
func TestTamperDetectionOnPrevHash(t *testing.T) {
	c := buildHonestChain(t)

	// Simulate a "sophisticated" tamper: attacker edits block 2's data and
	// even recomputes block 2's own hash to hide the edit, but does not
	// (cannot, cheaply) redo the proof-of-work.
	c.Blocks[2].Transactions[0].Amount = 1
	c.Blocks[2].Hash = c.Blocks[2].ComputeHash()

	err := c.Validate()
	if err == nil {
		t.Fatalf("expected validation to fail, but it passed")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected a *ValidationError, got %T", err)
	}
	// Detection happens at block 2 itself: its "fixed up" hash no longer
	// satisfies the proof-of-work target recorded for that block, because
	// the attacker did not re-mine it. This demonstrates that patching the
	// stored hash alone is not enough to hide a tamper.
	if ve.BlockHeight != c.Blocks[2].Height {
		t.Errorf("expected the tamper to be detected at block %d, got %d", c.Blocks[2].Height, ve.BlockHeight)
	}
}

// TestOverspendingRejected covers the "An overspending transaction is
// rejected" acceptance scenario (FR-4): given an account whose balance is
// 100, attempting to send 150 must be rejected and the balance must be
// unchanged.
func TestOverspendingRejected(t *testing.T) {
	c := New(1, 10)
	if err := c.AddTransaction(block.Transaction{Sender: block.FaucetAccount, Recipient: "alice", Amount: 100}); err != nil {
		t.Fatalf("faucet grant should succeed: %v", err)
	}
	if _, _, err := c.MineBlock(); err != nil {
		t.Fatalf("mining failed: %v", err)
	}

	l, err := c.Ledger()
	if err != nil {
		t.Fatalf("ledger rebuild failed: %v", err)
	}
	if got := l.Balance("alice"); got != 100 {
		t.Fatalf("expected alice's balance to be 100, got %d", got)
	}

	err = c.AddTransaction(block.Transaction{Sender: "alice", Recipient: "bob", Amount: 150})
	if err == nil {
		t.Fatalf("expected overspending transaction to be rejected")
	}

	l2, err := c.Ledger()
	if err != nil {
		t.Fatalf("ledger rebuild failed: %v", err)
	}
	if got := l2.Balance("alice"); got != 100 {
		t.Fatalf("expected alice's balance to remain 100 after rejected tx, got %d", got)
	}
}

// TestMalformedTransactionRejected checks non-positive amounts are refused
// (part of FR-4).
func TestMalformedTransactionRejected(t *testing.T) {
	c := New(1, 10)
	err := c.AddTransaction(block.Transaction{Sender: "alice", Recipient: "bob", Amount: 0})
	if err == nil {
		t.Fatalf("expected non-positive amount to be rejected")
	}
}

// TestSaveAndLoadRoundTrip covers FR-8 (persistence): a saved chain must
// reload to an equivalent, still-valid chain.
func TestSaveAndLoadRoundTrip(t *testing.T) {
	c := buildHonestChain(t)

	dir := t.TempDir()
	path := dir + "/chain.json"

	if err := c.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded.Blocks) != len(c.Blocks) {
		t.Fatalf("expected %d blocks after reload, got %d", len(c.Blocks), len(loaded.Blocks))
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("reloaded chain should still validate, got: %v", err)
	}
}

// TestMineBlockNoPending ensures mining with an empty pool is rejected
// with a clear error rather than producing an empty/junk block.
func TestMineBlockNoPending(t *testing.T) {
	c := New(1, 10)
	_, _, err := c.MineBlock()
	if err == nil {
		t.Fatalf("expected error when mining with no pending transactions")
	}
}
