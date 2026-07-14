package chain

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"toyblockchain/block"
)

var chainTestLimits = block.MiningLimits{
	MaxAttempts: 3_000_000,
	MaxNonce:    3_000_000,
}

type chainTestWallet struct {
	address    string
	privateKey ed25519.PrivateKey
}

func createChainTestWallet(t *testing.T) chainTestWallet {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate wallet: %v", err)
	}

	return chainTestWallet{
		address:    block.AddressFromPublicKey(publicKey),
		privateKey: privateKey,
	}
}

func signChainTransaction(
	t *testing.T,
	sender chainTestWallet,
	recipient string,
	amount int64,
	nonce uint64,
) block.Transaction {
	t.Helper()

	tx := block.Transaction{
		Recipient: recipient,
		Amount:    amount,
		Nonce:     nonce,
	}

	if err := tx.Sign(sender.privateKey); err != nil {
		t.Fatalf("failed to sign transaction: %v", err)
	}

	return tx
}

func createTestChain(
	t *testing.T,
	difficulty int,
) *Chain {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	testChain, err := New(
		ctx,
		difficulty,
		10,
		chainTestLimits,
	)
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}

	return testChain
}

func mineTestBlock(
	t *testing.T,
	testChain *Chain,
) *block.Block {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	minedBlock, _, err := testChain.MineBlock(
		ctx,
		chainTestLimits,
	)
	if err != nil {
		t.Fatalf("failed to mine block: %v", err)
	}

	return minedBlock
}

func TestNewChainHasGenesis(t *testing.T) {
	testChain := createTestChain(t, 2)

	if len(testChain.Blocks) != 1 {
		t.Fatalf(
			"expected one block, got %d",
			len(testChain.Blocks),
		)
	}

	genesis := testChain.Blocks[0]

	if genesis.Height != 0 {
		t.Errorf(
			"expected genesis height 0, got %d",
			genesis.Height,
		)
	}

	if genesis.PrevHash != block.GenesisPrevHash {
		t.Errorf(
			"unexpected genesis previous hash: %s",
			genesis.PrevHash,
		)
	}

	if genesis.Difficulty != 2 {
		t.Errorf(
			"expected genesis difficulty 2, got %d",
			genesis.Difficulty,
		)
	}
}

func buildHonestChain(t *testing.T) *Chain {
	t.Helper()

	testChain := createTestChain(t, 2)

	alice := createChainTestWallet(t)
	bob := createChainTestWallet(t)
	carol := createChainTestWallet(t)

	if err := testChain.AddTransaction(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
			Nonce:     0,
		},
	); err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	mineTestBlock(t, testChain)

	if err := testChain.AddTransaction(
		signChainTransaction(
			t,
			alice,
			bob.address,
			30,
			1,
		),
	); err != nil {
		t.Fatalf("Alice-to-Bob transaction failed: %v", err)
	}

	mineTestBlock(t, testChain)

	if err := testChain.AddTransaction(
		signChainTransaction(
			t,
			bob,
			carol.address,
			10,
			1,
		),
	); err != nil {
		t.Fatalf("Bob-to-Carol transaction failed: %v", err)
	}

	mineTestBlock(t, testChain)

	return testChain
}

func TestHonestChainValidates(t *testing.T) {
	testChain := buildHonestChain(t)

	if err := testChain.Validate(); err != nil {
		t.Fatalf(
			"expected honest chain to validate: %v",
			err,
		)
	}
}

func TestTamperDetection(t *testing.T) {
	testChain := buildHonestChain(t)

	testChain.Blocks[2].Transactions[0].Amount = 999999

	err := testChain.Validate()

	if err == nil {
		t.Fatal("expected tampered chain validation to fail")
	}

	validationError, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf(
			"expected ValidationError, got %T",
			err,
		)
	}

	if validationError.BlockHeight != 2 {
		t.Fatalf(
			"expected block 2 to fail, got block %d",
			validationError.BlockHeight,
		)
	}
}

func TestTamperedDifficultyRejected(t *testing.T) {
	testChain := buildHonestChain(t)

	// An attacker tries to lower the difficulty recorded in block 2.
	testChain.Blocks[2].Difficulty = 0

	// Recompute the block hash to hide the basic hash mismatch.
	testChain.Blocks[2].Hash =
		testChain.Blocks[2].ComputeHash()

	err := testChain.Validate()

	if err == nil {
		t.Fatal("expected modified difficulty to be rejected")
	}

	validationError, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf(
			"expected ValidationError, got %T",
			err,
		)
	}

	if validationError.BlockHeight != 2 {
		t.Fatalf(
			"expected block 2 to fail, got block %d",
			validationError.BlockHeight,
		)
	}
}

func TestDifficultyScheduleAppliesToFutureBlocks(t *testing.T) {
	testChain := createTestChain(t, 1)

	if err := testChain.AddTransaction(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: "first-account",
			Amount:    10,
		},
	); err != nil {
		t.Fatalf("failed to add first transaction: %v", err)
	}

	firstBlock := mineTestBlock(t, testChain)

	if firstBlock.Difficulty != 1 {
		t.Fatalf(
			"expected first block difficulty 1, got %d",
			firstBlock.Difficulty,
		)
	}

	if err := testChain.SetDifficulty(2); err != nil {
		t.Fatalf("failed to set difficulty: %v", err)
	}

	if err := testChain.AddTransaction(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: "second-account",
			Amount:    10,
		},
	); err != nil {
		t.Fatalf("failed to add second transaction: %v", err)
	}

	secondBlock := mineTestBlock(t, testChain)

	if secondBlock.Difficulty != 2 {
		t.Fatalf(
			"expected second block difficulty 2, got %d",
			secondBlock.Difficulty,
		)
	}

	// Existing blocks must keep their original difficulty.
	if firstBlock.Difficulty != 1 {
		t.Fatalf(
			"old block difficulty changed to %d",
			firstBlock.Difficulty,
		)
	}

	if testChain.ExpectedDifficulty(firstBlock.Height) != 1 {
		t.Fatal("old block policy difficulty changed")
	}

	if testChain.ExpectedDifficulty(secondBlock.Height) != 2 {
		t.Fatal("new block policy difficulty is incorrect")
	}

	if err := testChain.Validate(); err != nil {
		t.Fatalf(
			"chain with changed future difficulty should validate: %v",
			err,
		)
	}
}

func TestMiningFailureKeepsPendingTransactions(t *testing.T) {
	testChain := createTestChain(t, 1)

	if err := testChain.AddTransaction(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: "alice",
			Amount:    100,
		},
	); err != nil {
		t.Fatalf("failed to add transaction: %v", err)
	}

	if err := testChain.SetDifficulty(64); err != nil {
		t.Fatalf("failed to set difficulty: %v", err)
	}

	ctx := context.Background()

	_, _, err := testChain.MineBlock(
		ctx,
		block.MiningLimits{
			MaxAttempts: 1,
			MaxNonce:    10,
		},
	)

	if !errors.Is(err, block.ErrMaxAttempts) {
		t.Fatalf(
			"expected maximum attempts error, got %v",
			err,
		)
	}

	if len(testChain.Pending) != 1 {
		t.Fatalf(
			"expected pending transaction to remain, got %d",
			len(testChain.Pending),
		)
	}

	if len(testChain.Blocks) != 1 {
		t.Fatalf(
			"failed mining must not append block; got %d blocks",
			len(testChain.Blocks),
		)
	}
}

func TestOverspendingRejected(t *testing.T) {
	testChain := createTestChain(t, 1)

	alice := createChainTestWallet(t)
	bob := createChainTestWallet(t)

	if err := testChain.AddTransaction(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
		},
	); err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	mineTestBlock(t, testChain)

	overspend := signChainTransaction(
		t,
		alice,
		bob.address,
		150,
		1,
	)

	if err := testChain.AddTransaction(overspend); err == nil {
		t.Fatal("expected overspending transaction to be rejected")
	}

	currentLedger, err := testChain.Ledger()
	if err != nil {
		t.Fatalf("failed to rebuild ledger: %v", err)
	}

	if actual := currentLedger.Balance(alice.address); actual != 100 {
		t.Fatalf(
			"expected Alice balance 100, got %d",
			actual,
		)
	}
}

func TestUnsignedTransactionRejected(t *testing.T) {
	testChain := createTestChain(t, 1)

	err := testChain.AddTransaction(
		block.Transaction{
			Sender:    "alice",
			Recipient: "bob",
			Amount:    10,
			Nonce:     1,
		},
	)

	if err == nil {
		t.Fatal("expected unsigned transaction to be rejected")
	}
}

func TestPendingDoubleSpendRejected(t *testing.T) {
	testChain := createTestChain(t, 1)

	alice := createChainTestWallet(t)
	bob := createChainTestWallet(t)
	carol := createChainTestWallet(t)

	if err := testChain.AddTransaction(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
		},
	); err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	mineTestBlock(t, testChain)

	firstTransaction := signChainTransaction(
		t,
		alice,
		bob.address,
		80,
		1,
	)

	if err := testChain.AddTransaction(firstTransaction); err != nil {
		t.Fatalf("first pending transaction failed: %v", err)
	}

	secondTransaction := signChainTransaction(
		t,
		alice,
		carol.address,
		30,
		2,
	)

	if err := testChain.AddTransaction(secondTransaction); err == nil {
		t.Fatal("expected pending double spend to be rejected")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	testChain := buildHonestChain(t)

	directory := t.TempDir()
	path := filepath.Join(directory, "chain.json")

	if err := testChain.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loadedChain, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loadedChain.Blocks) != len(testChain.Blocks) {
		t.Fatalf(
			"expected %d blocks, got %d",
			len(testChain.Blocks),
			len(loadedChain.Blocks),
		)
	}

	if err := loadedChain.Validate(); err != nil {
		t.Fatalf(
			"loaded chain should validate: %v",
			err,
		)
	}
}

func TestAtomicSaveLeavesNoTemporaryFile(t *testing.T) {
	testChain := createTestChain(t, 1)

	directory := t.TempDir()
	path := filepath.Join(directory, "chain.json")

	if err := testChain.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"temporary save file still exists: %v",
			err,
		)
	}
}

func TestFileLockRejectsSecondProcess(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "chain.json")

	firstLock, err := AcquireFileLock(
		path,
		100*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer func() {
		_ = firstLock.Release()
	}()

	_, err = AcquireFileLock(
		path,
		200*time.Millisecond,
	)

	if err == nil {
		t.Fatal("expected second file lock to fail")
	}
}

func TestMineBlockNoPending(t *testing.T) {
	testChain := createTestChain(t, 1)

	_, _, err := testChain.MineBlock(
		context.Background(),
		chainTestLimits,
	)

	if err == nil {
		t.Fatal("expected error when mining without pending transactions")
	}
}
func TestTamperedMerkleRootRejected(t *testing.T) {
	testChain := buildHonestChain(t)

	testChain.Blocks[2].MerkleRoot = "invalid-merkle-root"

	err := testChain.Validate()
	if err == nil {
		t.Fatal("expected modified Merkle root to be rejected")
	}

	validationError, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf(
			"expected ValidationError, got %T",
			err,
		)
	}

	if validationError.BlockHeight != 2 {
		t.Fatalf(
			"expected block 2 to fail, got block %d",
			validationError.BlockHeight,
		)
	}
}

func TestTransactionTamperingBreaksMerkleRoot(t *testing.T) {
	testChain := buildHonestChain(t)

	testChain.Blocks[2].Transactions[0].Amount = 999999

	err := testChain.Validate()
	if err == nil {
		t.Fatal("expected transaction tampering to be rejected")
	}

	validationError, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf(
			"expected ValidationError, got %T",
			err,
		)
	}

	if validationError.BlockHeight != 2 {
		t.Fatalf(
			"expected block 2 to fail, got block %d",
			validationError.BlockHeight,
		)
	}
}
