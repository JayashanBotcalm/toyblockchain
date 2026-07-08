package block

import (
	"strings"
	"testing"
)

func sampleBlock() *Block {
	return &Block{
		Height:    1,
		Timestamp: 1700000000,
		Transactions: []Transaction{
			{Sender: "alice", Recipient: "bob", Amount: 10},
			{Sender: "bob", Recipient: "carol", Amount: 5},
		},
		PrevHash:   "deadbeef",
		Nonce:      42,
		Difficulty: 2,
	}
}

// TestHashDeterministic covers the "Block hashing is deterministic" (FR-3)
// acceptance scenario: given a block with fixed fields and a fixed nonce,
// computing its hash twice must yield identical results.
func TestHashDeterministic(t *testing.T) {
	b1 := sampleBlock()
	b2 := sampleBlock()

	h1 := b1.ComputeHash()
	h2 := b2.ComputeHash()

	if h1 != h2 {
		t.Fatalf("expected identical hashes for identical blocks, got %s vs %s", h1, h2)
	}

	// Hashing the same block object twice must also be stable.
	h1Again := b1.ComputeHash()
	if h1 != h1Again {
		t.Fatalf("hashing the same block twice produced different results: %s vs %s", h1, h1Again)
	}
}

// TestHashChangesWithContent ensures the hash actually depends on the
// fields we claim it depends on (a sanity check that ComputeHash isn't
// accidentally constant or ignoring fields).
func TestHashChangesWithContent(t *testing.T) {
	b := sampleBlock()
	original := b.ComputeHash()

	mutated := sampleBlock()
	mutated.Transactions[0].Amount = 999
	if mutated.ComputeHash() == original {
		t.Fatalf("expected hash to change when a transaction amount changes")
	}

	mutated2 := sampleBlock()
	mutated2.Nonce = 43
	if mutated2.ComputeHash() == original {
		t.Fatalf("expected hash to change when nonce changes")
	}

	mutated3 := sampleBlock()
	mutated3.PrevHash = "different"
	if mutated3.ComputeHash() == original {
		t.Fatalf("expected hash to change when PrevHash changes")
	}
}

// TestMineMeetsDifficulty covers the "A mined block satisfies the
// difficulty target" (FR-5) acceptance scenario: mining must produce a
// hash with at least N leading zero hex digits, and the returned nonce
// must reproduce that exact hash.
func TestMineMeetsDifficulty(t *testing.T) {
	const difficulty = 3
	b := NewBlock(1, []Transaction{{Sender: "alice", Recipient: "bob", Amount: 1}}, "someprevhash", difficulty)

	result := b.Mine(difficulty)

	if !strings.HasPrefix(result.Hash, strings.Repeat("0", difficulty)) {
		t.Fatalf("mined hash %s does not have %d leading zeros", result.Hash, difficulty)
	}
	if b.Hash != result.Hash {
		t.Fatalf("block.Hash (%s) does not match MineResult.Hash (%s)", b.Hash, result.Hash)
	}
	if b.Nonce != result.Nonce {
		t.Fatalf("block.Nonce (%d) does not match MineResult.Nonce (%d)", b.Nonce, result.Nonce)
	}

	// The nonce found must reproduce the exact same hash on recomputation.
	recomputed := b.ComputeHash()
	if recomputed != result.Hash {
		t.Fatalf("recomputing with the found nonce gave a different hash: %s vs %s", recomputed, result.Hash)
	}
}

// TestMeetsDifficultyEdgeCases exercises the leading-zero check directly.
func TestMeetsDifficultyEdgeCases(t *testing.T) {
	cases := []struct {
		hash       string
		difficulty int
		want       bool
	}{
		{"0000abcd", 4, true},
		{"0000abcd", 5, false},
		{"000abcd", 4, false},
		{"anything", 0, true},
		{"", 1, false},
	}
	for _, tc := range cases {
		got := MeetsDifficulty(tc.hash, tc.difficulty)
		if got != tc.want {
			t.Errorf("MeetsDifficulty(%q, %d) = %v, want %v", tc.hash, tc.difficulty, got, tc.want)
		}
	}
}

// TestTransactionValidate covers FR-4's structural checks.
func TestTransactionValidate(t *testing.T) {
	valid := Transaction{Sender: "alice", Recipient: "bob", Amount: 10}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid transaction to pass, got error: %v", err)
	}

	cases := []Transaction{
		{Sender: "alice", Recipient: "bob", Amount: 0},
		{Sender: "alice", Recipient: "bob", Amount: -5},
		{Sender: "", Recipient: "bob", Amount: 10},
		{Sender: "alice", Recipient: "", Amount: 10},
		{Sender: "alice", Recipient: "alice", Amount: 10},
	}
	for _, tx := range cases {
		if err := tx.Validate(); err == nil {
			t.Errorf("expected transaction %+v to be rejected, but it passed", tx)
		}
	}
}

// TestGenesisBlock covers the "Chain starts from a deterministic genesis
// block" (FR-2) acceptance scenario at the block level: previous-hash
// equals the fixed genesis value.
func TestGenesisBlock(t *testing.T) {
	g := NewGenesisBlock(1)
	if g.Height != 0 {
		t.Errorf("expected genesis height 0, got %d", g.Height)
	}
	if g.PrevHash != GenesisPrevHash {
		t.Errorf("expected genesis PrevHash %s, got %s", GenesisPrevHash, g.PrevHash)
	}
	if len(GenesisPrevHash) != 64 {
		t.Errorf("expected GenesisPrevHash to be 64 hex chars (SHA-256 length), got %d", len(GenesisPrevHash))
	}
}
