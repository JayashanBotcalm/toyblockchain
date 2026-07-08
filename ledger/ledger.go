// Package ledger derives account balances from a sequence of transactions
// and enforces the "no overspending" rule (FR-4).
package ledger

import (
	"fmt"

	"toyblockchain/block"
)

// Ledger tracks account balances. It is always derived data: the source of
// truth is the chain itself, and a Ledger can be rebuilt from scratch at any
// time by replaying every transaction in every block in order.
type Ledger struct {
	balances map[string]int64
}

// New returns an empty ledger.
func New() *Ledger {
	return &Ledger{balances: make(map[string]int64)}
}

// Balance returns the current balance of an account (0 if never seen).
func (l *Ledger) Balance(account string) int64 {
	return l.balances[account]
}

// Balances returns a copy of all known balances, for display purposes.
func (l *Ledger) Balances() map[string]int64 {
	out := make(map[string]int64, len(l.balances))
	for k, v := range l.balances {
		out[k] = v
	}
	return out
}

// ValidateTransaction checks a transaction against the ledger's current
// state WITHOUT applying it. It enforces FR-4: malformed transactions
// (non-positive amount, empty fields) and overspending are both rejected.
// The FaucetAccount is exempt from the balance check, standing in for a
// coinbase / minting mechanism.
func (l *Ledger) ValidateTransaction(tx block.Transaction) error {
	if err := tx.Validate(); err != nil {
		return err
	}
	if tx.Sender == block.FaucetAccount {
		return nil
	}
	if l.balances[tx.Sender] < tx.Amount {
		return fmt.Errorf("insufficient balance: %s has %d, tried to send %d",
			tx.Sender, l.balances[tx.Sender], tx.Amount)
	}
	return nil
}

// Apply validates and then applies a transaction, mutating balances. It
// returns an error (and leaves the ledger unchanged) if the transaction is
// invalid -- callers must not assume partial application on error.
func (l *Ledger) Apply(tx block.Transaction) error {
	if err := l.ValidateTransaction(tx); err != nil {
		return err
	}
	if tx.Sender != block.FaucetAccount {
		l.balances[tx.Sender] -= tx.Amount
	}
	l.balances[tx.Recipient] += tx.Amount
	return nil
}

// ApplyBlock applies every transaction in a block, in order. If any
// transaction is invalid this returns an error immediately; since blocks
// that made it onto the chain have already been checked at mining time,
// this should only fail on a corrupted / tampered chain being replayed.
func (l *Ledger) ApplyBlock(b *block.Block) error {
	for i, tx := range b.Transactions {
		if err := l.Apply(tx); err != nil {
			return fmt.Errorf("block %d tx %d: %w", b.Height, i, err)
		}
	}
	return nil
}

// Rebuild replays an entire chain (in height order) from scratch and
// returns the resulting ledger. This is the canonical way balances are
// derived (FR-4): they are never stored directly, always computed.
func Rebuild(blocks []*block.Block) (*Ledger, error) {
	l := New()
	for _, b := range blocks {
		if err := l.ApplyBlock(b); err != nil {
			return nil, err
		}
	}
	return l, nil
}
