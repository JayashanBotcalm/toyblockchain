// Package ledger derives balances and transaction nonces by replaying
// the blockchain transaction history.
package ledger

import (
	"fmt"

	"toyblockchain/block"
)

// Ledger stores derived account balances and the last accepted transaction
// nonce for each sender.
type Ledger struct {
	balances map[string]int64
	nonces   map[string]uint64
}

// New creates an empty ledger.
func New() *Ledger {
	return &Ledger{
		balances: make(map[string]int64),
		nonces:   make(map[string]uint64),
	}
}

// Balance returns an account balance.
func (l *Ledger) Balance(account string) int64 {
	return l.balances[account]
}

// NextNonce returns the next required nonce for a sender.
//
// Example:
//
//	last accepted nonce = 3
//	next required nonce = 4
func (l *Ledger) NextNonce(account string) uint64 {
	return l.nonces[account] + 1
}

// Balances returns a safe copy for display.
func (l *Ledger) Balances() map[string]int64 {
	result := make(map[string]int64, len(l.balances))

	for account, balance := range l.balances {
		result[account] = balance
	}

	return result
}

// ValidateTransaction validates:
//
//   - transaction structure,
//   - digital signature,
//   - sender ownership,
//   - replay nonce,
//   - sender balance.
func (l *Ledger) ValidateTransaction(tx block.Transaction) error {
	if err := tx.Validate(); err != nil {
		return err
	}

	if tx.Sender == block.FaucetAccount {
		return nil
	}

	expectedNonce := l.NextNonce(tx.Sender)

	if tx.Nonce != expectedNonce {
		return fmt.Errorf(
			"invalid transaction nonce for %s: got %d, want %d",
			tx.Sender,
			tx.Nonce,
			expectedNonce,
		)
	}

	if l.balances[tx.Sender] < tx.Amount {
		return fmt.Errorf(
			"insufficient balance: %s has %d, tried to send %d",
			tx.Sender,
			l.balances[tx.Sender],
			tx.Amount,
		)
	}

	return nil
}

// Apply validates and applies one transaction.
func (l *Ledger) Apply(tx block.Transaction) error {
	if err := l.ValidateTransaction(tx); err != nil {
		return err
	}

	if tx.Sender != block.FaucetAccount {
		l.balances[tx.Sender] -= tx.Amount
		l.nonces[tx.Sender] = tx.Nonce
	}

	l.balances[tx.Recipient] += tx.Amount

	return nil
}

// ApplyBlock applies all transactions in their original order.
func (l *Ledger) ApplyBlock(b *block.Block) error {
	for index, tx := range b.Transactions {
		if err := l.Apply(tx); err != nil {
			return fmt.Errorf(
				"block %d tx %d: %w",
				b.Height,
				index,
				err,
			)
		}
	}

	return nil
}

// Rebuild recreates the ledger from the blockchain.
func Rebuild(blocks []*block.Block) (*Ledger, error) {
	result := New()

	for _, b := range blocks {
		if err := result.ApplyBlock(b); err != nil {
			return nil, err
		}
	}

	return result, nil
}
