package ledger

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"toyblockchain/block"
)

var ledgerTestLimits = block.MiningLimits{
	MaxAttempts: 2_000_000,
	MaxNonce:    2_000_000,
}

type testWallet struct {
	address    string
	privateKey ed25519.PrivateKey
}

func newTestWallet(t *testing.T) testWallet {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate wallet: %v", err)
	}

	return testWallet{
		address:    block.AddressFromPublicKey(publicKey),
		privateKey: privateKey,
	}
}

func signedTransaction(
	t *testing.T,
	wallet testWallet,
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

	if err := tx.Sign(wallet.privateKey); err != nil {
		t.Fatalf("failed to sign transaction: %v", err)
	}

	return tx
}

func TestApplyUpdatesBalances(t *testing.T) {
	alice := newTestWallet(t)
	bob := newTestWallet(t)

	currentLedger := New()

	faucetTransaction := block.Transaction{
		Sender:    block.FaucetAccount,
		Recipient: alice.address,
		Amount:    100,
		Nonce:     0,
	}

	if err := currentLedger.Apply(faucetTransaction); err != nil {
		t.Fatalf("faucet apply failed: %v", err)
	}

	if actual := currentLedger.Balance(alice.address); actual != 100 {
		t.Fatalf(
			"expected Alice balance 100, got %d",
			actual,
		)
	}

	transfer := signedTransaction(
		t,
		alice,
		bob.address,
		40,
		1,
	)

	if err := currentLedger.Apply(transfer); err != nil {
		t.Fatalf("signed transfer failed: %v", err)
	}

	if actual := currentLedger.Balance(alice.address); actual != 60 {
		t.Fatalf(
			"expected Alice balance 60, got %d",
			actual,
		)
	}

	if actual := currentLedger.Balance(bob.address); actual != 40 {
		t.Fatalf(
			"expected Bob balance 40, got %d",
			actual,
		)
	}

	if actual := currentLedger.NextNonce(alice.address); actual != 2 {
		t.Fatalf(
			"expected Alice next nonce 2, got %d",
			actual,
		)
	}
}

func TestApplyRejectsOverspend(t *testing.T) {
	alice := newTestWallet(t)
	bob := newTestWallet(t)

	currentLedger := New()

	err := currentLedger.Apply(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
			Nonce:     0,
		},
	)
	if err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	overspend := signedTransaction(
		t,
		alice,
		bob.address,
		150,
		1,
	)

	if err := currentLedger.Apply(overspend); err == nil {
		t.Fatal("expected overspending transaction to be rejected")
	}

	if actual := currentLedger.Balance(alice.address); actual != 100 {
		t.Fatalf(
			"expected Alice balance to remain 100, got %d",
			actual,
		)
	}

	if actual := currentLedger.Balance(bob.address); actual != 0 {
		t.Fatalf(
			"expected Bob balance to remain 0, got %d",
			actual,
		)
	}
}

func TestReplayTransactionRejected(t *testing.T) {
	alice := newTestWallet(t)
	bob := newTestWallet(t)

	currentLedger := New()

	err := currentLedger.Apply(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
		},
	)
	if err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	firstTransaction := signedTransaction(
		t,
		alice,
		bob.address,
		20,
		1,
	)

	if err := currentLedger.Apply(firstTransaction); err != nil {
		t.Fatalf("first transaction failed: %v", err)
	}

	// Applying the exact transaction again is a replay attack.
	if err := currentLedger.Apply(firstTransaction); err == nil {
		t.Fatal("expected replayed transaction to be rejected")
	}

	if actual := currentLedger.Balance(alice.address); actual != 80 {
		t.Fatalf(
			"expected Alice balance 80, got %d",
			actual,
		)
	}

	if actual := currentLedger.Balance(bob.address); actual != 20 {
		t.Fatalf(
			"expected Bob balance 20, got %d",
			actual,
		)
	}
}

func TestWrongNonceRejected(t *testing.T) {
	alice := newTestWallet(t)
	bob := newTestWallet(t)

	currentLedger := New()

	err := currentLedger.Apply(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
		},
	)
	if err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	wrongNonceTransaction := signedTransaction(
		t,
		alice,
		bob.address,
		20,
		2,
	)

	if err := currentLedger.Apply(wrongNonceTransaction); err == nil {
		t.Fatal("expected incorrect nonce to be rejected")
	}
}

func TestModifiedSignatureRejected(t *testing.T) {
	alice := newTestWallet(t)
	bob := newTestWallet(t)

	currentLedger := New()

	err := currentLedger.Apply(
		block.Transaction{
			Sender:    block.FaucetAccount,
			Recipient: alice.address,
			Amount:    100,
		},
	)
	if err != nil {
		t.Fatalf("faucet transaction failed: %v", err)
	}

	tx := signedTransaction(
		t,
		alice,
		bob.address,
		20,
		1,
	)

	// Change signed data after signing.
	tx.Amount = 30

	if err := currentLedger.Apply(tx); err == nil {
		t.Fatal("expected modified signed transaction to be rejected")
	}
}

func TestRebuildFromBlocks(t *testing.T) {
	alice := newTestWallet(t)
	bob := newTestWallet(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	genesis, err := block.NewGenesisBlock(
		ctx,
		1,
		ledgerTestLimits,
	)
	if err != nil {
		t.Fatalf("failed to create genesis block: %v", err)
	}

	firstBlock := block.NewBlock(
		1,
		[]block.Transaction{
			{
				Sender:    block.FaucetAccount,
				Recipient: alice.address,
				Amount:    50,
				Nonce:     0,
			},
		},
		genesis.Hash,
		1,
	)

	if _, err := firstBlock.Mine(ctx, ledgerTestLimits); err != nil {
		t.Fatalf("failed to mine first block: %v", err)
	}

	secondBlock := block.NewBlock(
		2,
		[]block.Transaction{
			signedTransaction(
				t,
				alice,
				bob.address,
				20,
				1,
			),
		},
		firstBlock.Hash,
		1,
	)

	if _, err := secondBlock.Mine(ctx, ledgerTestLimits); err != nil {
		t.Fatalf("failed to mine second block: %v", err)
	}

	rebuiltLedger, err := Rebuild(
		[]*block.Block{
			genesis,
			firstBlock,
			secondBlock,
		},
	)
	if err != nil {
		t.Fatalf("ledger rebuild failed: %v", err)
	}

	if actual := rebuiltLedger.Balance(alice.address); actual != 30 {
		t.Fatalf(
			"expected Alice balance 30, got %d",
			actual,
		)
	}

	if actual := rebuiltLedger.Balance(bob.address); actual != 20 {
		t.Fatalf(
			"expected Bob balance 20, got %d",
			actual,
		)
	}
}
