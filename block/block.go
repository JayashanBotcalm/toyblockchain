// Package block defines transactions, blocks, hashing, digital signatures,
// Merkle roots and concurrent Proof-of-Work mining.
package block

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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

// Transaction represents one transfer of value.
type Transaction struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Amount    int64  `json:"amount"`
	Nonce     uint64 `json:"nonce"`
	PublicKey string `json:"public_key,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// transactionPayload contains exactly the fields protected by the signature.
type transactionPayload struct {
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	Amount    int64  `json:"amount"`
	Nonce     uint64 `json:"nonce"`
}

// signingBytes converts the protected transaction fields into stable bytes.
func (t Transaction) signingBytes() ([]byte, error) {
	payload := transactionPayload{
		Sender:    t.Sender,
		Recipient: t.Recipient,
		Amount:    t.Amount,
		Nonce:     t.Nonce,
	}

	return json.Marshal(payload)
}

// AddressFromPublicKey creates a wallet address from an Ed25519 public key.
func AddressFromPublicKey(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)

	// Use the first 20 bytes to create a compact 40-character address.
	return hex.EncodeToString(sum[:20])
}

// Sign signs a normal transaction with an Ed25519 private key.
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
		return fmt.Errorf(
			"serialising transaction for signing: %w",
			err,
		)
	}

	signature := ed25519.Sign(privateKey, data)
	t.Signature = hex.EncodeToString(signature)

	return nil
}

// VerifySignature verifies transaction ownership and integrity.
func (t Transaction) VerifySignature() error {
	if t.Sender == FaucetAccount {
		if t.PublicKey != "" || t.Signature != "" {
			return fmt.Errorf(
				"faucet transaction must not contain a signature",
			)
		}

		return nil
	}

	publicKeyBytes, err := hex.DecodeString(t.PublicKey)
	if err != nil ||
		len(publicKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid transaction public key")
	}

	signatureBytes, err := hex.DecodeString(t.Signature)
	if err != nil ||
		len(signatureBytes) != ed25519.SignatureSize {
		return fmt.Errorf(
			"invalid transaction signature encoding",
		)
	}

	publicKey := ed25519.PublicKey(publicKeyBytes)

	expectedAddress := AddressFromPublicKey(publicKey)

	if expectedAddress != t.Sender {
		return fmt.Errorf(
			"public key does not own sender address",
		)
	}

	data, err := t.signingBytes()
	if err != nil {
		return fmt.Errorf(
			"serialising transaction for verification: %w",
			err,
		)
	}

	if !ed25519.Verify(
		publicKey,
		data,
		signatureBytes,
	) {
		return fmt.Errorf(
			"transaction signature verification failed",
		)
	}

	return nil
}

// Validate checks the basic transaction structure and signature.
func (t Transaction) Validate() error {
	if t.Amount <= 0 {
		return fmt.Errorf(
			"transaction amount must be positive, got %d",
			t.Amount,
		)
	}

	if strings.TrimSpace(t.Sender) == "" {
		return fmt.Errorf(
			"transaction sender must not be empty",
		)
	}

	if strings.TrimSpace(t.Recipient) == "" {
		return fmt.Errorf(
			"transaction recipient must not be empty",
		)
	}

	if t.Sender == t.Recipient {
		return fmt.Errorf(
			"sender and recipient must differ",
		)
	}

	if t.Sender == FaucetAccount {
		if t.Nonce != 0 {
			return fmt.Errorf(
				"faucet transaction nonce must be 0",
			)
		}

		return t.VerifySignature()
	}

	if t.Nonce == 0 {
		return fmt.Errorf(
			"signed transaction nonce must be greater than 0",
		)
	}

	return t.VerifySignature()
}

// Block represents one block in the blockchain.
type Block struct {
	Height       int           `json:"height"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	MerkleRoot   string        `json:"merkle_root"`
	PrevHash     string        `json:"prev_hash"`
	Nonce        uint64        `json:"nonce"`
	Difficulty   int           `json:"difficulty"`
	Hash         string        `json:"hash"`
}

// hashPayload contains the fields used to calculate the block hash.
//
// Transactions are represented by MerkleRoot rather than hashing the entire
// transaction slice directly.
type hashPayload struct {
	Height     int    `json:"height"`
	Timestamp  int64  `json:"timestamp"`
	MerkleRoot string `json:"merkle_root"`
	PrevHash   string `json:"prev_hash"`
	Nonce      uint64 `json:"nonce"`
	Difficulty int    `json:"difficulty"`
}

// ComputeHash calculates the deterministic SHA-256 block hash.
func (b *Block) ComputeHash() string {
	payload := hashPayload{
		Height:     b.Height,
		Timestamp:  b.Timestamp,
		MerkleRoot: b.MerkleRoot,
		PrevHash:   b.PrevHash,
		Nonce:      b.Nonce,
		Difficulty: b.Difficulty,
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

// MeetsDifficulty checks whether a hash begins with the required zeroes.
func MeetsDifficulty(hash string, difficulty int) bool {
	if difficulty < 0 ||
		difficulty > sha256.Size*2 {
		return false
	}

	requiredPrefix := strings.Repeat("0", difficulty)

	return strings.HasPrefix(hash, requiredPrefix)
}

// MineResult describes a successful mining operation.
type MineResult struct {
	Nonce      uint64        `json:"nonce"`
	Hash       string        `json:"hash"`
	Attempts   uint64        `json:"attempts"`
	Elapsed    time.Duration `json:"elapsed"`
	Difficulty int           `json:"difficulty"`
	Workers    int           `json:"workers"`
}

// MiningLimits controls mining safety and concurrency.
type MiningLimits struct {
	MaxAttempts uint64
	MaxNonce    uint64
	Workers     int
}

// workerResult is sent when one mining worker finds a valid nonce.
type workerResult struct {
	nonce uint64
	hash  string
}

// NewBlock creates a new block ready for mining.
func NewBlock(
	height int,
	transactions []Transaction,
	previousHash string,
	difficulty int,
) *Block {
	transactionCopy := append(
		[]Transaction(nil),
		transactions...,
	)

	return &Block{
		Height:       height,
		Timestamp:    time.Now().Unix(),
		Transactions: transactionCopy,
		MerkleRoot:   CalculateMerkleRoot(transactionCopy),
		PrevHash:     previousHash,
		Nonce:        0,
		Difficulty:   difficulty,
		Hash:         "",
	}
}

// Mine searches for a valid nonce using multiple goroutines.
//
// Example with four workers:
//
//	worker 0: 0, 4, 8, 12...
//	worker 1: 1, 5, 9, 13...
//	worker 2: 2, 6, 10, 14...
//	worker 3: 3, 7, 11, 15...
func (b *Block) Mine(
	ctx context.Context,
	limits MiningLimits,
) (MineResult, error) {
	if b.Difficulty < 0 ||
		b.Difficulty > sha256.Size*2 {
		return MineResult{}, fmt.Errorf(
			"invalid block difficulty %d",
			b.Difficulty,
		)
	}

	workerCount := limits.Workers

	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}

	if workerCount < 1 {
		workerCount = 1
	}

	miningContext, cancel := context.WithCancel(ctx)
	defer cancel()

	start := time.Now()

	resultChannel := make(chan workerResult, 1)
	workersDone := make(chan struct{})

	var workerGroup sync.WaitGroup
	var attempts atomic.Uint64
	var maximumAttemptsReached atomic.Bool

	workerGroup.Add(workerCount)

	for workerID := 0; workerID < workerCount; workerID++ {
		go func(id int) {
			defer workerGroup.Done()

			step := uint64(workerCount)

			for nonce := uint64(id); ; nonce += step {
				select {
				case <-miningContext.Done():
					return
				default:
				}

				if limits.MaxNonce > 0 &&
					nonce > limits.MaxNonce {
					return
				}

				currentAttempt := attempts.Add(1)

				if limits.MaxAttempts > 0 &&
					currentAttempt > limits.MaxAttempts {
					maximumAttemptsReached.Store(true)
					cancel()
					return
				}

				// Each worker hashes a local copy.
				// This prevents workers from changing the same block together.
				candidate := *b
				candidate.Nonce = nonce

				hash := candidate.ComputeHash()

				if MeetsDifficulty(
					hash,
					candidate.Difficulty,
				) {
					select {
					case resultChannel <- workerResult{
						nonce: nonce,
						hash:  hash,
					}:
						cancel()

					default:
					}

					return
				}

				// Prevent uint64 overflow.
				if nonce > ^uint64(0)-step {
					return
				}
			}
		}(workerID)
	}

	go func() {
		workerGroup.Wait()
		close(workersDone)
	}()

	for {
		select {
		case result := <-resultChannel:
			b.Nonce = result.nonce
			b.Hash = result.hash

			return MineResult{
				Nonce:      result.nonce,
				Hash:       result.hash,
				Attempts:   attempts.Load(),
				Elapsed:    time.Since(start),
				Difficulty: b.Difficulty,
				Workers:    workerCount,
			}, nil

		case <-ctx.Done():
			if errors.Is(
				ctx.Err(),
				context.DeadlineExceeded,
			) {
				return MineResult{}, ErrMiningTimeout
			}

			return MineResult{}, ErrMiningCancelled

		case <-workersDone:
			// A worker may have sent a valid result just before all workers
			// finished. Check the result channel once more.
			select {
			case result := <-resultChannel:
				b.Nonce = result.nonce
				b.Hash = result.hash

				return MineResult{
					Nonce:      result.nonce,
					Hash:       result.hash,
					Attempts:   attempts.Load(),
					Elapsed:    time.Since(start),
					Difficulty: b.Difficulty,
					Workers:    workerCount,
				}, nil

			default:
			}

			if maximumAttemptsReached.Load() {
				return MineResult{}, ErrMaxAttempts
			}

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
	emptyTransactions := []Transaction{}

	genesis := &Block{
		Height:       0,
		Timestamp:    0,
		Transactions: emptyTransactions,
		MerkleRoot:   CalculateMerkleRoot(emptyTransactions),
		PrevHash:     GenesisPrevHash,
		Nonce:        0,
		Difficulty:   difficulty,
		Hash:         "",
	}

	if _, err := genesis.Mine(ctx, limits); err != nil {
		return nil, fmt.Errorf(
			"mining genesis block: %w",
			err,
		)
	}

	return genesis, nil
}

// String returns a readable representation of the block.
func (b *Block) String() string {
	var builder strings.Builder

	fmt.Fprintf(
		&builder,
		"Block #%d\n",
		b.Height,
	)

	fmt.Fprintf(
		&builder,
		"  Timestamp:  %s\n",
		time.Unix(
			b.Timestamp,
			0,
		).UTC().Format(time.RFC3339),
	)

	fmt.Fprintf(
		&builder,
		"  PrevHash:   %s\n",
		b.PrevHash,
	)

	fmt.Fprintf(
		&builder,
		"  MerkleRoot: %s\n",
		b.MerkleRoot,
	)

	fmt.Fprintf(
		&builder,
		"  Hash:       %s\n",
		b.Hash,
	)

	fmt.Fprintf(
		&builder,
		"  Nonce:      %d\n",
		b.Nonce,
	)

	fmt.Fprintf(
		&builder,
		"  Difficulty: %d\n",
		b.Difficulty,
	)

	if len(b.Transactions) == 0 {
		fmt.Fprintln(
			&builder,
			"  Transactions: (none)",
		)
	} else {
		fmt.Fprintln(
			&builder,
			"  Transactions:",
		)

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
