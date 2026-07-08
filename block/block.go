// Package block defines the core Block and Transaction types used by the
// toy blockchain, along with deterministic hashing and mining logic.
package block

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GenesisPrevHash is the fixed, well-known previous-hash value used by the
// genesis block. It is 64 hex characters (32 bytes) of zero, matching the
// length of a SHA-256 digest so that genesis "looks like" every other block.
const GenesisPrevHash = "0000000000000000000000000000000000000000000000000000000000000000"

// FaucetAccount is a special sender name that is allowed to create funds
// out of thin air. It is exempt from balance checks in the ledger. This
// stands in for a coinbase / minting mechanism (see FR-4).
const FaucetAccount = "FAUCET"

// Transaction is the smallest unit of value transfer recorded on the chain.
type Transaction struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Amount    int64  `json:"amount"` // amount is expressed in whole "coins" (int64 to avoid float rounding issues)
}

// Validate performs the structural checks required by FR-4: the amount must
// be strictly positive, and sender/recipient must be non-empty. It does NOT
// check the sender's balance -- that is the ledger's job, because it
// requires knowledge of chain history.
func (t Transaction) Validate() error {
	if t.Amount <= 0 {
		return fmt.Errorf("transaction amount must be positive, got %d", t.Amount)
	}
	if strings.TrimSpace(t.Sender) == "" {
		return fmt.Errorf("transaction sender must not be empty")
	}
	if strings.TrimSpace(t.Recipient) == "" {
		return fmt.Errorf("transaction recipient must not be empty")
	}
	if t.Sender == t.Recipient {
		return fmt.Errorf("sender and recipient must differ")
	}
	return nil
}

// Block is a single link in the chain. It carries a batch of transactions
// and is cryptographically bound to the block before it via PrevHash.
type Block struct {
	Height       int           `json:"height"`
	Timestamp    int64         `json:"timestamp"` // Unix seconds
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prev_hash"`
	Nonce        uint64        `json:"nonce"`
	Difficulty   int           `json:"difficulty"` // number of required leading hex zeros at mining time
	Hash         string        `json:"hash"`
}

// hashPayload is the canonical, ordered representation of a block that is
// fed into SHA-256. It deliberately excludes the Hash field itself (a block
// cannot include its own hash as an input to that hash) and fixes field
// order explicitly via struct tags + Go's stable struct-to-JSON field order,
// rather than relying on map iteration (which Go randomises).
//
// Fields, in the exact order they are hashed:
//  1. height
//  2. timestamp
//  3. transactions (each: sender, recipient, amount, in that order)
//  4. prev_hash
//  5. nonce
//  6. difficulty
type hashPayload struct {
	Height       int           `json:"height"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prev_hash"`
	Nonce        uint64        `json:"nonce"`
	Difficulty   int           `json:"difficulty"`
}

// ComputeHash deterministically hashes the block's fields (excluding Hash).
// Because encoding/json always serialises struct fields in declaration
// order (never map order) and Transaction has no maps, marshalling
// hashPayload is fully deterministic across runs and machines.
func (b *Block) ComputeHash() string {
	payload := hashPayload{
		Height:       b.Height,
		Timestamp:    b.Timestamp,
		Transactions: b.Transactions,
		PrevHash:     b.PrevHash,
		Nonce:        b.Nonce,
		Difficulty:   b.Difficulty,
	}
	// json.Marshal on a struct with no maps is deterministic: field order
	// follows struct declaration order every time.
	data, err := json.Marshal(payload)
	if err != nil {
		// Transaction/Block contain only marshalable primitive types, so
		// this should never happen; a panic here indicates a programming
		// error (e.g. someone added a channel or func field).
		panic(fmt.Sprintf("block: failed to marshal hash payload: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// meetsDifficulty reports whether hash has at least `difficulty` leading
// hex zero characters.
func meetsDifficulty(hash string, difficulty int) bool {
	if difficulty <= 0 {
		return true
	}
	if difficulty > len(hash) {
		return false
	}
	return hash[:difficulty] == strings.Repeat("0", difficulty)
}

// MeetsDifficulty exposes meetsDifficulty for use by other packages (chain
// validation needs it too).
func MeetsDifficulty(hash string, difficulty int) bool {
	return meetsDifficulty(hash, difficulty)
}

// MineResult carries the outcome of a mining run, used both by the CLI (to
// report progress) and by the research report's difficulty experiments.
type MineResult struct {
	Nonce      uint64
	Hash       string
	Attempts   uint64
	Elapsed    time.Duration
	Difficulty int
}

// NewBlock constructs a block with the given height, transactions and
// previous hash, ready to be mined. Timestamp is set to "now".
func NewBlock(height int, txs []Transaction, prevHash string, difficulty int) *Block {
	return &Block{
		Height:       height,
		Timestamp:    time.Now().Unix(),
		Transactions: txs,
		PrevHash:     prevHash,
		Nonce:        0,
		Difficulty:   difficulty,
	}
}

// Mine performs proof-of-work: it searches nonce values starting at 0 until
// ComputeHash() produces a hash with at least `difficulty` leading hex
// zeros (FR-5). On success it sets b.Nonce and b.Hash and returns a
// MineResult describing the search.
func (b *Block) Mine(difficulty int) MineResult {
	b.Difficulty = difficulty
	start := time.Now()
	var attempts uint64
	for nonce := uint64(0); ; nonce++ {
		b.Nonce = nonce
		attempts++
		h := b.ComputeHash()
		if meetsDifficulty(h, difficulty) {
			b.Hash = h
			return MineResult{
				Nonce:      nonce,
				Hash:       h,
				Attempts:   attempts,
				Elapsed:    time.Since(start),
				Difficulty: difficulty,
			}
		}
	}
}

// NewGenesisBlock returns the single, deterministic block that starts every
// chain (FR-2). It is mined at the given difficulty like any other block so
// that chain validation logic can treat it uniformly, but height 0 and the
// fixed PrevHash make it recognisable as genesis.
func NewGenesisBlock(difficulty int) *Block {
	b := &Block{
		Height:       0,
		Timestamp:    0, // fixed timestamp keeps genesis fully deterministic across runs
		Transactions: []Transaction{},
		PrevHash:     GenesisPrevHash,
		Nonce:        0,
		Difficulty:   difficulty,
	}
	b.Mine(difficulty)
	return b
}

// String renders a block in a readable, multi-line form for the CLI's
// "print" command.
func (b *Block) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Block #%d\n", b.Height)
	fmt.Fprintf(&sb, "  Timestamp:  %s\n", time.Unix(b.Timestamp, 0).UTC().Format(time.RFC3339))
	fmt.Fprintf(&sb, "  PrevHash:   %s\n", b.PrevHash)
	fmt.Fprintf(&sb, "  Hash:       %s\n", b.Hash)
	fmt.Fprintf(&sb, "  Nonce:      %d\n", b.Nonce)
	fmt.Fprintf(&sb, "  Difficulty: %d\n", b.Difficulty)
	if len(b.Transactions) == 0 {
		fmt.Fprintf(&sb, "  Transactions: (none)\n")
	} else {
		fmt.Fprintf(&sb, "  Transactions:\n")
		for _, tx := range b.Transactions {
			fmt.Fprintf(&sb, "    - %s -> %s : %d\n", tx.Sender, tx.Recipient, tx.Amount)
		}
	}
	return sb.String()
}
