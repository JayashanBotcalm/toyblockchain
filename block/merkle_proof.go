package block

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ProofStep is one sibling hash on the path from a transaction leaf up to
// the Merkle root. Left reports whether the sibling belongs on the left
// of the pairing (i.e. the node being proved is the right child at this
// level); this is needed to hash the pair back together in the correct
// order when the proof is verified.
type ProofStep struct {
	Hash string `json:"hash"`
	Left bool   `json:"left"`
}

// GenerateMerkleProof returns the sibling path from the transaction at
// index up to the Merkle root of transactions, following exactly the same
// pairing and odd-level duplication rule as CalculateMerkleRoot. A caller
// can later verify that a single transaction belongs to a block using
// only its own hash, this proof, and the block's already-known Merkle
// root -- without needing every other transaction in the block.
func GenerateMerkleProof(
	transactions []Transaction,
	index int,
) ([]ProofStep, error) {
	if index < 0 || index >= len(transactions) {
		return nil, fmt.Errorf(
			"merkle proof index %d out of range for %d transactions",
			index,
			len(transactions),
		)
	}

	level := make([][]byte, 0, len(transactions))

	for _, tx := range transactions {
		hashBytes, err := hex.DecodeString(TransactionHash(tx))
		if err != nil {
			return nil, fmt.Errorf("decoding transaction hash: %w", err)
		}

		level = append(level, hashBytes)
	}

	proof := make([]ProofStep, 0)
	position := index

	for len(level) > 1 {
		// If the number of hashes is odd, duplicate the final hash. This
		// must match CalculateMerkleRoot exactly, or proofs built here
		// would not verify against roots computed there.
		if len(level)%2 != 0 {
			lastHash := level[len(level)-1]

			duplicate := make([]byte, len(lastHash))
			copy(duplicate, lastHash)

			level = append(level, duplicate)
		}

		var siblingIndex int
		var siblingIsLeft bool

		if position%2 == 0 {
			siblingIndex = position + 1
			siblingIsLeft = false
		} else {
			siblingIndex = position - 1
			siblingIsLeft = true
		}

		proof = append(proof, ProofStep{
			Hash: hex.EncodeToString(level[siblingIndex]),
			Left: siblingIsLeft,
		})

		nextLevel := make([][]byte, 0, len(level)/2)

		for i := 0; i < len(level); i += 2 {
			combined := make([]byte, 0, len(level[i])+len(level[i+1]))
			combined = append(combined, level[i]...)
			combined = append(combined, level[i+1]...)

			parentHash := sha256.Sum256(combined)
			nextLevel = append(nextLevel, parentHash[:])
		}

		level = nextLevel
		position = position / 2
	}

	return proof, nil
}

// VerifyMerkleProof recomputes a Merkle root from a single transaction
// hash and its proof path, and reports whether the result matches root.
// This lets a light client confirm one transaction is included in a block
// without downloading or hashing the block's full transaction list.
func VerifyMerkleProof(
	transactionHash string,
	proof []ProofStep,
	root string,
) bool {
	current, err := hex.DecodeString(transactionHash)
	if err != nil {
		return false
	}

	for _, step := range proof {
		sibling, err := hex.DecodeString(step.Hash)
		if err != nil {
			return false
		}

		combined := make([]byte, 0, len(current)+len(sibling))

		if step.Left {
			combined = append(combined, sibling...)
			combined = append(combined, current...)
		} else {
			combined = append(combined, current...)
			combined = append(combined, sibling...)
		}

		parentHash := sha256.Sum256(combined)
		current = parentHash[:]
	}

	return hex.EncodeToString(current) == root
}
