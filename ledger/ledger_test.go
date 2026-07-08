package ledger

import (
	"testing"

	"toyblockchain/block"
)

func TestApplyUpdatesBalances(t *testing.T) {
	l := New()
	if err := l.Apply(block.Transaction{Sender: block.FaucetAccount, Recipient: "alice", Amount: 100}); err != nil {
		t.Fatalf("faucet apply failed: %v", err)
	}
	if got := l.Balance("alice"); got != 100 {
		t.Fatalf("expected alice balance 100, got %d", got)
	}

	if err := l.Apply(block.Transaction{Sender: "alice", Recipient: "bob", Amount: 40}); err != nil {
		t.Fatalf("alice->bob apply failed: %v", err)
	}
	if got := l.Balance("alice"); got != 60 {
		t.Fatalf("expected alice balance 60, got %d", got)
	}
	if got := l.Balance("bob"); got != 40 {
		t.Fatalf("expected bob balance 40, got %d", got)
	}
}

func TestApplyRejectsOverspend(t *testing.T) {
	l := New()
	_ = l.Apply(block.Transaction{Sender: block.FaucetAccount, Recipient: "alice", Amount: 100})

	err := l.Apply(block.Transaction{Sender: "alice", Recipient: "bob", Amount: 150})
	if err == nil {
		t.Fatalf("expected overspend to be rejected")
	}
	// Balance must be unchanged after a rejected transaction.
	if got := l.Balance("alice"); got != 100 {
		t.Fatalf("expected alice balance to remain 100, got %d", got)
	}
}

func TestApplyRejectsMalformed(t *testing.T) {
	l := New()
	_ = l.Apply(block.Transaction{Sender: block.FaucetAccount, Recipient: "alice", Amount: 100})

	cases := []block.Transaction{
		{Sender: "alice", Recipient: "bob", Amount: 0},
		{Sender: "alice", Recipient: "bob", Amount: -10},
	}
	for _, tx := range cases {
		if err := l.Apply(tx); err == nil {
			t.Errorf("expected transaction %+v to be rejected", tx)
		}
	}
	if got := l.Balance("alice"); got != 100 {
		t.Fatalf("expected alice balance to remain 100 after rejected txs, got %d", got)
	}
}

func TestRebuildFromBlocks(t *testing.T) {
	b0 := block.NewGenesisBlock(1)
	b1 := block.NewBlock(1, []block.Transaction{
		{Sender: block.FaucetAccount, Recipient: "alice", Amount: 50},
	}, b0.Hash, 1)
	b1.Mine(1)
	b2 := block.NewBlock(2, []block.Transaction{
		{Sender: "alice", Recipient: "bob", Amount: 20},
	}, b1.Hash, 1)
	b2.Mine(1)

	l, err := Rebuild([]*block.Block{b0, b1, b2})
	if err != nil {
		t.Fatalf("rebuild failed: %v", err)
	}
	if got := l.Balance("alice"); got != 30 {
		t.Fatalf("expected alice balance 30, got %d", got)
	}
	if got := l.Balance("bob"); got != 20 {
		t.Fatalf("expected bob balance 20, got %d", got)
	}
}
