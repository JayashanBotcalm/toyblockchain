package block

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

var testMiningLimits = MiningLimits{
	MaxAttempts: 2_000_000,
	MaxNonce:    2_000_000,
}

func createSignedTransaction(
	t *testing.T,
	recipient string,
	amount int64,
	nonce uint64,
) Transaction {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}

	tx := Transaction{
		Recipient: recipient,
		Amount:    amount,
		Nonce:     nonce,
	}

	if err := tx.Sign(privateKey); err != nil {
		t.Fatalf("failed to sign transaction: %v", err)
	}

	return tx
}

func sampleBlock() *Block {
	transactions := []Transaction{
		{
			Sender:    FaucetAccount,
			Recipient: "alice",
			Amount:    10,
			Nonce:     0,
		},
	}

	return &Block{
		Height:       1,
		Timestamp:    1700000000,
		Transactions: transactions,
		MerkleRoot:   CalculateMerkleRoot(transactions),
		PrevHash:     "deadbeef",
		Nonce:        42,
		Difficulty:   2,
	}
}

func TestHashDeterministic(t *testing.T) {
	firstBlock := sampleBlock()
	secondBlock := sampleBlock()

	firstHash := firstBlock.ComputeHash()
	secondHash := secondBlock.ComputeHash()

	if firstHash != secondHash {
		t.Fatalf(
			"expected identical hashes, got %s and %s",
			firstHash,
			secondHash,
		)
	}

	recomputedHash := firstBlock.ComputeHash()

	if firstHash != recomputedHash {
		t.Fatalf(
			"same block produced different hashes: %s and %s",
			firstHash,
			recomputedHash,
		)
	}
}

func TestHashChangesWithContent(t *testing.T) {
	originalBlock := sampleBlock()
	originalHash := originalBlock.ComputeHash()

	changedTransaction := sampleBlock()
	changedTransaction.Transactions[0].Amount = 999

	changedTransaction.MerkleRoot = CalculateMerkleRoot(
		changedTransaction.Transactions,
	)

	if changedTransaction.ComputeHash() == originalHash {
		t.Fatal("hash should change when transaction amount changes")
	}

	changedNonce := sampleBlock()
	changedNonce.Nonce = 43

	if changedNonce.ComputeHash() == originalHash {
		t.Fatal("hash should change when nonce changes")
	}

	changedPreviousHash := sampleBlock()
	changedPreviousHash.PrevHash = "different"

	if changedPreviousHash.ComputeHash() == originalHash {
		t.Fatal("hash should change when previous hash changes")
	}

	changedDifficulty := sampleBlock()
	changedDifficulty.Difficulty = 3

	if changedDifficulty.ComputeHash() == originalHash {
		t.Fatal("hash should change when difficulty changes")
	}
}

func TestMineMeetsDifficulty(t *testing.T) {
	const difficulty = 3

	testBlock := NewBlock(
		1,
		[]Transaction{
			{
				Sender:    FaucetAccount,
				Recipient: "alice",
				Amount:    1,
				Nonce:     0,
			},
		},
		"someprevhash",
		difficulty,
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	result, err := testBlock.Mine(ctx, testMiningLimits)
	if err != nil {
		t.Fatalf("mining failed: %v", err)
	}

	expectedPrefix := strings.Repeat("0", difficulty)

	if !strings.HasPrefix(result.Hash, expectedPrefix) {
		t.Fatalf(
			"hash %s does not start with %s",
			result.Hash,
			expectedPrefix,
		)
	}

	if testBlock.Hash != result.Hash {
		t.Fatalf(
			"block hash %s does not match result hash %s",
			testBlock.Hash,
			result.Hash,
		)
	}

	if testBlock.Nonce != result.Nonce {
		t.Fatalf(
			"block nonce %d does not match result nonce %d",
			testBlock.Nonce,
			result.Nonce,
		)
	}

	if recomputed := testBlock.ComputeHash(); recomputed != result.Hash {
		t.Fatalf(
			"recomputed hash %s does not match mined hash %s",
			recomputed,
			result.Hash,
		)
	}

	if result.Difficulty != difficulty {
		t.Fatalf(
			"expected difficulty %d, got %d",
			difficulty,
			result.Difficulty,
		)
	}
}

func TestMineDoesNotReplaceBlockDifficulty(t *testing.T) {
	const assignedDifficulty = 2

	testBlock := NewBlock(
		1,
		[]Transaction{
			{
				Sender:    FaucetAccount,
				Recipient: "alice",
				Amount:    10,
			},
		},
		"previous",
		assignedDifficulty,
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	result, err := testBlock.Mine(ctx, testMiningLimits)
	if err != nil {
		t.Fatalf("mining failed: %v", err)
	}

	if testBlock.Difficulty != assignedDifficulty {
		t.Fatalf(
			"block difficulty changed: got %d, want %d",
			testBlock.Difficulty,
			assignedDifficulty,
		)
	}

	if result.Difficulty != assignedDifficulty {
		t.Fatalf(
			"result difficulty is %d, want %d",
			result.Difficulty,
			assignedDifficulty,
		)
	}
}

func TestMineStopsAtMaximumAttempts(t *testing.T) {
	testBlock := NewBlock(
		1,
		[]Transaction{
			{
				Sender:    FaucetAccount,
				Recipient: "alice",
				Amount:    1,
			},
		},
		"previous",
		64,
	)

	limits := MiningLimits{
		MaxAttempts: 1,
		MaxNonce:    100,
	}

	_, err := testBlock.Mine(context.Background(), limits)

	if !errors.Is(err, ErrMaxAttempts) {
		t.Fatalf(
			"expected ErrMaxAttempts, got %v",
			err,
		)
	}
}

func TestMineCanBeCancelled(t *testing.T) {
	testBlock := NewBlock(
		1,
		[]Transaction{
			{
				Sender:    FaucetAccount,
				Recipient: "alice",
				Amount:    1,
			},
		},
		"previous",
		64,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := testBlock.Mine(
		ctx,
		MiningLimits{
			MaxAttempts: 1_000_000,
			MaxNonce:    1_000_000,
		},
	)

	if !errors.Is(err, ErrMiningCancelled) {
		t.Fatalf(
			"expected ErrMiningCancelled, got %v",
			err,
		)
	}
}

func TestMeetsDifficultyEdgeCases(t *testing.T) {
	tests := []struct {
		hash       string
		difficulty int
		expected   bool
	}{
		{
			hash:       "0000abcd",
			difficulty: 4,
			expected:   true,
		},
		{
			hash:       "0000abcd",
			difficulty: 5,
			expected:   false,
		},
		{
			hash:       "000abcd",
			difficulty: 4,
			expected:   false,
		},
		{
			hash:       "anything",
			difficulty: 0,
			expected:   true,
		},
		{
			hash:       "",
			difficulty: 1,
			expected:   false,
		},
		{
			hash:       "0000",
			difficulty: -1,
			expected:   false,
		},
		{
			hash:       strings.Repeat("0", 64),
			difficulty: 65,
			expected:   false,
		},
	}

	for _, test := range tests {
		actual := MeetsDifficulty(
			test.hash,
			test.difficulty,
		)

		if actual != test.expected {
			t.Errorf(
				"MeetsDifficulty(%q, %d) = %v, want %v",
				test.hash,
				test.difficulty,
				actual,
				test.expected,
			)
		}
	}
}

func TestSignedTransactionValidate(t *testing.T) {
	tx := createSignedTransaction(
		t,
		"recipient-address",
		10,
		1,
	)

	if err := tx.Validate(); err != nil {
		t.Fatalf(
			"expected signed transaction to be valid: %v",
			err,
		)
	}
}

func TestUnsignedNormalTransactionRejected(t *testing.T) {
	tx := Transaction{
		Sender:    "alice",
		Recipient: "bob",
		Amount:    10,
		Nonce:     1,
	}

	if err := tx.Validate(); err == nil {
		t.Fatal("expected unsigned normal transaction to be rejected")
	}
}

func TestModifiedSignedTransactionRejected(t *testing.T) {
	tx := createSignedTransaction(
		t,
		"recipient-address",
		10,
		1,
	)

	tx.Amount = 999

	if err := tx.Validate(); err == nil {
		t.Fatal("expected modified signed transaction to be rejected")
	}
}

func TestFaucetTransactionValidate(t *testing.T) {
	tx := Transaction{
		Sender:    FaucetAccount,
		Recipient: "alice",
		Amount:    100,
		Nonce:     0,
	}

	if err := tx.Validate(); err != nil {
		t.Fatalf(
			"expected faucet transaction to be valid: %v",
			err,
		)
	}
}

func TestMalformedTransactionsRejected(t *testing.T) {
	tests := []Transaction{
		{
			Sender:    FaucetAccount,
			Recipient: "bob",
			Amount:    0,
		},
		{
			Sender:    FaucetAccount,
			Recipient: "bob",
			Amount:    -5,
		},
		{
			Sender:    "",
			Recipient: "bob",
			Amount:    10,
		},
		{
			Sender:    FaucetAccount,
			Recipient: "",
			Amount:    10,
		},
		{
			Sender:    FaucetAccount,
			Recipient: FaucetAccount,
			Amount:    10,
		},
	}

	for _, tx := range tests {
		if err := tx.Validate(); err == nil {
			t.Errorf(
				"expected transaction %+v to be rejected",
				tx,
			)
		}
	}
}

func TestGenesisBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	genesis, err := NewGenesisBlock(
		ctx,
		1,
		testMiningLimits,
	)
	if err != nil {
		t.Fatalf("failed to create genesis block: %v", err)
	}

	if genesis.Height != 0 {
		t.Errorf(
			"expected genesis height 0, got %d",
			genesis.Height,
		)
	}

	if genesis.Timestamp != 0 {
		t.Errorf(
			"expected deterministic genesis timestamp 0, got %d",
			genesis.Timestamp,
		)
	}

	if genesis.PrevHash != GenesisPrevHash {
		t.Errorf(
			"expected genesis previous hash %s, got %s",
			GenesisPrevHash,
			genesis.PrevHash,
		)
	}

	if len(GenesisPrevHash) != 64 {
		t.Errorf(
			"expected 64-character previous hash, got %d",
			len(GenesisPrevHash),
		)
	}

	if !MeetsDifficulty(
		genesis.Hash,
		genesis.Difficulty,
	) {
		t.Fatal("genesis hash does not meet its difficulty")
	}
}

func TestGenesisDeterministicAcrossWorkerCounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := NewGenesisBlock(
		ctx,
		2,
		MiningLimits{MaxAttempts: 2_000_000, MaxNonce: 2_000_000, Workers: 1},
	)
	if err != nil {
		t.Fatalf("creating first genesis: %v", err)
	}

	second, err := NewGenesisBlock(
		ctx,
		2,
		MiningLimits{MaxAttempts: 2_000_000, MaxNonce: 2_000_000, Workers: 8},
	)
	if err != nil {
		t.Fatalf("creating second genesis: %v", err)
	}

	if first.Hash != second.Hash || first.Nonce != second.Nonce {
		t.Fatalf(
			"genesis must be deterministic: first nonce/hash=%d/%s second=%d/%s",
			first.Nonce,
			first.Hash,
			second.Nonce,
			second.Hash,
		)
	}
}
