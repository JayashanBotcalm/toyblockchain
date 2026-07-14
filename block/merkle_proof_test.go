package block

import (
	"fmt"
	"testing"
)

func sampleTransactionsForProof(count int) []Transaction {
	transactions := make([]Transaction, 0, count)

	for i := 0; i < count; i++ {
		transactions = append(transactions, Transaction{
			Sender:    FaucetAccount,
			Recipient: fmt.Sprintf("account-%d", i),
			Amount:    int64(i + 1),
		})
	}

	return transactions
}

// TestMerkleProofVerifiesInclusion checks that a proof generated for a
// transaction, verified against the block's actual Merkle root, succeeds
// for every position in several differently-sized (even and odd)
// transaction lists.
func TestMerkleProofVerifiesInclusion(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 5, 7, 8} {
		transactions := sampleTransactionsForProof(count)
		root := CalculateMerkleRoot(transactions)

		for index := 0; index < count; index++ {
			proof, err := GenerateMerkleProof(transactions, index)
			if err != nil {
				t.Fatalf(
					"count=%d index=%d: unexpected error: %v",
					count,
					index,
					err,
				)
			}

			leafHash := TransactionHash(transactions[index])

			if !VerifyMerkleProof(leafHash, proof, root) {
				t.Errorf(
					"count=%d index=%d: expected proof to verify against root %s",
					count,
					index,
					root,
				)
			}
		}
	}
}

// TestMerkleProofFailsForWrongLeaf ensures a proof built for one
// transaction does not verify a different transaction's hash.
func TestMerkleProofFailsForWrongLeaf(t *testing.T) {
	transactions := sampleTransactionsForProof(4)
	root := CalculateMerkleRoot(transactions)

	proof, err := GenerateMerkleProof(transactions, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wrongLeafHash := TransactionHash(transactions[1])

	if VerifyMerkleProof(wrongLeafHash, proof, root) {
		t.Fatalf("expected proof for transaction 0 to fail for transaction 1's hash")
	}
}

// TestMerkleProofFailsAgainstWrongRoot ensures a valid proof does not
// verify against a root from a different transaction set.
func TestMerkleProofFailsAgainstWrongRoot(t *testing.T) {
	transactions := sampleTransactionsForProof(4)
	otherTransactions := sampleTransactionsForProof(5)
	otherRoot := CalculateMerkleRoot(otherTransactions)

	proof, err := GenerateMerkleProof(transactions, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	leafHash := TransactionHash(transactions[2])

	if VerifyMerkleProof(leafHash, proof, otherRoot) {
		t.Fatalf("expected proof to fail against an unrelated root")
	}
}

// TestMerkleProofIndexOutOfRange checks bounds validation.
func TestMerkleProofIndexOutOfRange(t *testing.T) {
	transactions := sampleTransactionsForProof(3)

	if _, err := GenerateMerkleProof(transactions, -1); err == nil {
		t.Errorf("expected error for negative index")
	}

	if _, err := GenerateMerkleProof(transactions, 3); err == nil {
		t.Errorf("expected error for out-of-range index")
	}

	if _, err := GenerateMerkleProof(nil, 0); err == nil {
		t.Errorf("expected error for empty transaction list")
	}
}

// TestMerkleProofSingleTransaction covers the trivial one-leaf case,
// where the leaf hash and the root are the same value and the proof path
// is empty.
func TestMerkleProofSingleTransaction(t *testing.T) {
	transactions := sampleTransactionsForProof(1)
	root := CalculateMerkleRoot(transactions)

	proof, err := GenerateMerkleProof(transactions, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(proof) != 0 {
		t.Errorf("expected an empty proof for a single-transaction block, got %d steps", len(proof))
	}

	leafHash := TransactionHash(transactions[0])
	if leafHash != root {
		t.Fatalf("expected single-transaction root to equal the leaf hash")
	}

	if !VerifyMerkleProof(leafHash, proof, root) {
		t.Fatalf("expected empty proof to verify trivially")
	}
}
