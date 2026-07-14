package block

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// TransactionHash calculates a deterministic SHA-256 hash for one transaction.
func TransactionHash(tx Transaction) string {
	data, err := json.Marshal(tx)
	if err != nil {
		panic(fmt.Sprintf(
			"block: failed to marshal transaction: %v",
			err,
		))
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// CalculateMerkleRoot creates one summary hash for all transactions.
//
// Process:
//
//	transaction hashes
//	      ↓
//	combine neighbouring hashes
//	      ↓
//	hash each pair
//	      ↓
//	repeat until one root remains
//
// If a level contains an odd number of hashes, the final hash is duplicated.
func CalculateMerkleRoot(transactions []Transaction) string {
	if len(transactions) == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:])
	}

	level := make([][]byte, 0, len(transactions))

	for _, tx := range transactions {
		data, err := json.Marshal(tx)
		if err != nil {
			panic(fmt.Sprintf(
				"block: failed to marshal transaction: %v",
				err,
			))
		}

		txHash := sha256.Sum256(data)

		hashCopy := make([]byte, len(txHash))
		copy(hashCopy, txHash[:])

		level = append(level, hashCopy)
	}

	for len(level) > 1 {
		// If the number of hashes is odd, duplicate the final hash.
		if len(level)%2 != 0 {
			lastHash := level[len(level)-1]

			duplicate := make([]byte, len(lastHash))
			copy(duplicate, lastHash)

			level = append(level, duplicate)
		}

		nextLevel := make([][]byte, 0, len(level)/2)

		for index := 0; index < len(level); index += 2 {
			left := level[index]
			right := level[index+1]

			combined := make(
				[]byte,
				0,
				len(left)+len(right),
			)

			combined = append(combined, left...)
			combined = append(combined, right...)

			parentHash := sha256.Sum256(combined)

			parentCopy := make([]byte, len(parentHash))
			copy(parentCopy, parentHash[:])

			nextLevel = append(nextLevel, parentCopy)
		}

		level = nextLevel
	}

	return hex.EncodeToString(level[0])
}
