// Package block defines transactions, blocks, deterministic hashing,
// digital signatures, and bounded proof-of-work mining.
package block

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const GenesisPrevHash = "0000000000000000000000000000000000000000000000000000000000000000"

const FaucetAccount = "FAUCET"

var (
	ErrMiningTimeout   = errors.New("mining timeout reached")
	ErrMaxAttempts     = errors.New("maximum mining attempts reached")
	ErrMaxNonce        = errors.New("maximum nonce reached")
	ErrMiningCancelled = errors.New("mining cancelled")
)

// Transaction represents a transfer between two accounts.
//
// Normal transactions require an Ed25519 public key and signature.
// Faucet transactions are the only unsigned transactions.
type Transaction struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Amount    int64  `json:"amount"`
	Nonce     uint64 `json:"nonce"`
	PublicKey string `json:"public_key,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// transactionPayload contains exactly the transaction fields protected
// by the digital signature.
type transactionPayload struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Amount    int64  `json:"amount"`
	Nonce     uint64 `json:"nonce"`
}

func (t Transaction) signingBytes() ([]byte, error) {
	payload := transactionPayload{
		Sender:    t.Sender,
		Recipient: t.Recipient,
		Amount:    t.Amount,
		Nonce:     t.Nonce,
	}

	return json.Marshal(payload)
}

// AddressFromPublicKey creates an account address from an Ed25519 public key.
func AddressFromPublicKey(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)

	// Twenty bytes are enough for a compact address in this toy project.
	return hex.EncodeToString(sum[:20])
}

// Sign signs a normal transaction using an Ed25519 private key.
func (t *Transaction) Sign(privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf(
			"invalid Ed25519 private key length: got %d, want %d",
			len(privateKey),
			ed25519.PrivateKeySize,
		)
	}

	publicKey := privateKey.Public().(ed25519.PublicKey)

	t.Sender = AddressFromPublicKey(publicKey)
	t.PublicKey = hex.EncodeToString(publicKey)

	data, err := t.signingBytes()
	if err != nil {
		return fmt.Errorf("serialising transaction for signing: %w", err)
	}

	signature := ed25519.Sign(privateKey, data)
	t.Signature = hex.EncodeToString(signature)

	return nil
}

// VerifySignature verifies that:
//
//  1. the public key creates the claimed sender address,
//  2. the signature was created by the matching private key,
//  3. the signed transaction fields were not modified.
func (t Transaction) VerifySignature() error {
	if t.Sender == FaucetAccount {
		if t.PublicKey != "" || t.Signature != "" {
			return fmt.Errorf("faucet transaction must not contain a signature")
		}

		return nil
	}

	publicKeyBytes, err := hex.DecodeString(t.PublicKey)
	if err != nil || len(publicKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid transaction public key")
	}

	signatureBytes, err := hex.DecodeString(t.Signature)
	if err != nil || len(signatureBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid transaction signature encoding")
	}

	publicKey := ed25519.PublicKey(publicKeyBytes)

	expectedAddress := AddressFromPublicKey(publicKey)
	if expectedAddress != t.Sender {
		return fmt.Errorf("public key does not own sender address")
	}

	data, err := t.signingBytes()
	if err != nil {
		return fmt.Errorf("serialising transaction for verification: %w", err)
	}

	if !ed25519.Verify(publicKey, data, signatureBytes) {
		return fmt.Errorf("transaction signature verification failed")
	}

	return nil
}

// Validate performs structural validation and signature verification.
//
// Balance and replay validation belong to the ledger package because they
// depend on previous blockchain state.
func (t Transaction) Validate() error {
	if t.Amount <= 0 {
		return fmt.Errorf(
			"transaction amount must be positive, got %d",
			t.Amount,
		)
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

	if t.Sender == FaucetAccount {
		if t.Nonce != 0 {
			return fmt.Errorf("faucet transaction nonce must be 0")
		}

		return t.VerifySignature()
	}

	if t.Nonce == 0 {
		return fmt.Errorf("signed transaction nonce must be greater than 0")
	}

	return t.VerifySignature()
}

// Block is one link in the blockchain.
type Block struct {
	Height       int           `json:"height"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prev_hash"`
	Nonce        uint64        `json:"nonce"`
	Difficulty   int           `json:"difficulty"`
	Hash         string        `json:"hash"`
}

// hashPayload is the deterministic block representation passed to SHA-256.
//
// Hash is deliberately excluded because a block cannot use its own unknown
// hash while calculating that hash.
type hashPayload struct {
	Height       int           `json:"height"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prev_hash"`
	Nonce        uint64        `json:"nonce"`
	Difficulty   int           `json:"difficulty"`
}

// ComputeHash returns the deterministic SHA-256 block hash.
func (b *Block) ComputeHash() string {
	payload := hashPayload{
		Height:       b.Height,
		Timestamp:    b.Timestamp,
		Transactions: b.Transactions,
		PrevHash:     b.PrevHash,
		Nonce:        b.Nonce,
		Difficulty:   b.Difficulty,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf(
			"block: failed to marshal hash payload: %v",
			err,
		))
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// MeetsDifficulty verifies the required number of leading hexadecimal zeros.
func MeetsDifficulty(hash string, difficulty int) bool {
	if difficulty < 0 || difficulty > sha256.Size*2 {
		return false
	}

	expectedPrefix := strings.Repeat("0", difficulty)

	return strings.HasPrefix(hash, expectedPrefix)
}

// MineResult describes a completed mining operation.
type MineResult struct {
	Nonce      uint64        `json:"nonce"`
	Hash       string        `json:"hash"`
	Attempts   uint64        `json:"attempts"`
	Elapsed    time.Duration `json:"elapsed"`
	Difficulty int           `json:"difficulty"`
}

// MiningLimits prevents mining from running forever.
type MiningLimits struct {
	MaxAttempts uint64
	MaxNonce    uint64
}

// NewBlock creates a block with its difficulty permanently assigned.
//
// Mine does not accept another difficulty, so it cannot silently replace
// the difficulty stored in the block.
func NewBlock(
	height int,
	transactions []Transaction,
	previousHash string,
	difficulty int,
) *Block {
	transactionCopy := append([]Transaction(nil), transactions...)

	return &Block{
		Height:       height,
		Timestamp:    time.Now().Unix(),
		Transactions: transactionCopy,
		PrevHash:     previousHash,
		Nonce:        0,
		Difficulty:   difficulty,
		Hash:         "",
	}
}

// Mine performs bounded Proof of Work.
//
// It uses only b.Difficulty. Therefore, the difficulty assigned by NewBlock
// cannot be silently changed by passing another argument.
//
// Cancellation and timeout are controlled through context.Context.
func (b *Block) Mine(
	ctx context.Context,
	limits MiningLimits,
) (MineResult, error) {
	if b.Difficulty < 0 || b.Difficulty > sha256.Size*2 {
		return MineResult{}, fmt.Errorf(
			"invalid block difficulty %d",
			b.Difficulty,
		)
	}

	start := time.Now()
	var attempts uint64

	for nonce := uint64(0); ; nonce++ {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return MineResult{}, ErrMiningTimeout
			}

			return MineResult{}, ErrMiningCancelled

		default:
		}

		if limits.MaxAttempts > 0 && attempts >= limits.MaxAttempts {
			return MineResult{}, ErrMaxAttempts
		}

		if limits.MaxNonce > 0 && nonce > limits.MaxNonce {
			return MineResult{}, ErrMaxNonce
		}

		b.Nonce = nonce
		attempts++

		hash := b.ComputeHash()

		if MeetsDifficulty(hash, b.Difficulty) {
			b.Hash = hash

			return MineResult{
				Nonce:      nonce,
				Hash:       hash,
				Attempts:   attempts,
				Elapsed:    time.Since(start),
				Difficulty: b.Difficulty,
			}, nil
		}

		// Prevent uint64 overflow from returning to nonce zero.
		if nonce == ^uint64(0) {
			return MineResult{}, ErrMaxNonce
		}
	}
}

// NewGenesisBlock creates and mines the deterministic genesis block.
func NewGenesisBlock(
	ctx context.Context,
	difficulty int,
	limits MiningLimits,
) (*Block, error) {
	genesis := &Block{
		Height:       0,
		Timestamp:    0,
		Transactions: []Transaction{},
		PrevHash:     GenesisPrevHash,
		Nonce:        0,
		Difficulty:   difficulty,
		Hash:         "",
	}

	if _, err := genesis.Mine(ctx, limits); err != nil {
		return nil, fmt.Errorf("mining genesis block: %w", err)
	}

	return genesis, nil
}

// String returns a human-readable block representation.
func (b *Block) String() string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "Block #%d\n", b.Height)
	fmt.Fprintf(
		&builder,
		"  Timestamp:  %s\n",
		time.Unix(b.Timestamp, 0).UTC().Format(time.RFC3339),
	)
	fmt.Fprintf(&builder, "  PrevHash:   %s\n", b.PrevHash)
	fmt.Fprintf(&builder, "  Hash:       %s\n", b.Hash)
	fmt.Fprintf(&builder, "  Nonce:      %d\n", b.Nonce)
	fmt.Fprintf(&builder, "  Difficulty: %d\n", b.Difficulty)

	if len(b.Transactions) == 0 {
		fmt.Fprintln(&builder, "  Transactions: (none)")
	} else {
		fmt.Fprintln(&builder, "  Transactions:")

		for _, tx := range b.Transactions {
			fmt.Fprintf(
				&builder,
				"    - %s -> %s : %d (tx nonce %d)\n",
				tx.Sender,
				tx.Recipient,
				tx.Amount,
				tx.Nonce,
			)
		}
	}

	return builder.String()
}
