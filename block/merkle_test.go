package block

import "testing"

func TestMerkleRootDeterministic(t *testing.T) {
	transactions := []Transaction{
		{
			Sender:    FaucetAccount,
			Recipient: "alice",
			Amount:    100,
			Nonce:     0,
		},
		{
			Sender:    FaucetAccount,
			Recipient: "bob",
			Amount:    50,
			Nonce:     0,
		},
	}

	first := CalculateMerkleRoot(transactions)
	second := CalculateMerkleRoot(transactions)

	if first != second {
		t.Fatalf(
			"expected identical Merkle roots, got %s and %s",
			first,
			second,
		)
	}
}

func TestMerkleRootChangesWhenTransactionChanges(t *testing.T) {
	original := []Transaction{
		{
			Sender:    FaucetAccount,
			Recipient: "alice",
			Amount:    100,
		},
	}

	modified := []Transaction{
		{
			Sender:    FaucetAccount,
			Recipient: "alice",
			Amount:    999,
		},
	}

	if CalculateMerkleRoot(original) ==
		CalculateMerkleRoot(modified) {
		t.Fatal(
			"Merkle root should change when transaction data changes",
		)
	}
}

func TestEmptyMerkleRootDeterministic(t *testing.T) {
	first := CalculateMerkleRoot(nil)
	second := CalculateMerkleRoot([]Transaction{})

	if first != second {
		t.Fatal(
			"nil and empty transaction lists should have the same root",
		)
	}
}
