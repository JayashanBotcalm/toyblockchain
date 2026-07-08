// Package cli implements the command-line interface required by FR-7: add
// a transaction, mine a block, print the chain, validate it, and show
// balances. It works both as an interactive REPL and as a one-shot command
// runner (`toyblockchain <command> [args...]`), which makes it easy to
// script and to test.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"toyblockchain/block"
	"toyblockchain/chain"
)

// CLI bundles the chain together with the data file path used for
// persistence, and the I/O streams commands read from / write to.
type CLI struct {
	Chain    *chain.Chain
	DataFile string
	Out      io.Writer
}

// New creates a CLI wrapping the given chain.
func New(c *chain.Chain, dataFile string, out io.Writer) *CLI {
	return &CLI{Chain: c, DataFile: dataFile, Out: out}
}

func (c *CLI) printf(format string, args ...interface{}) {
	fmt.Fprintf(c.Out, format, args...)
}

// Run executes a single command given as a slice like ["addtx", "alice",
// "bob", "10"]. It returns an error for the caller to report; it never
// exits the process itself, which keeps it testable.
func (c *CLI) Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command given; try 'help'")
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help":
		c.help()
		return nil

	case "addtx":
		return c.cmdAddTx(rest)

	case "faucet":
		return c.cmdFaucet(rest)

	case "mine":
		return c.cmdMine()

	case "print":
		c.cmdPrint()
		return nil

	case "validate":
		return c.cmdValidate()

	case "balances":
		c.cmdBalances()
		return nil

	case "balance":
		return c.cmdBalance(rest)

	case "save":
		return c.cmdSave()

	case "pending":
		c.cmdPending()
		return nil

	default:
		return fmt.Errorf("unknown command %q; try 'help'", cmd)
	}
}

func (c *CLI) help() {
	c.printf(`Available commands:
  addtx <sender> <recipient> <amount>   Queue a transaction in the pending pool
  faucet <recipient> <amount>           Mint new funds to an account (testing/bootstrap)
  mine                                  Mine a new block from pending transactions
  print                                 Print the full chain
  validate                              Validate the chain and report the result
  balances                              Show balances for every known account
  balance <account>                     Show the balance of a single account
  pending                               List transactions waiting to be mined
  save                                  Persist the chain to disk immediately
  help                                  Show this message
`)
}

func (c *CLI) cmdAddTx(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: addtx <sender> <recipient> <amount>")
	}
	amount, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid amount %q: %w", args[2], err)
	}
	tx := block.Transaction{Sender: args[0], Recipient: args[1], Amount: amount}
	if err := c.Chain.AddTransaction(tx); err != nil {
		return fmt.Errorf("transaction rejected: %w", err)
	}
	c.printf("Queued transaction: %s -> %s : %d\n", tx.Sender, tx.Recipient, tx.Amount)
	return nil
}

func (c *CLI) cmdFaucet(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: faucet <recipient> <amount>")
	}
	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid amount %q: %w", args[1], err)
	}
	tx := block.Transaction{Sender: block.FaucetAccount, Recipient: args[0], Amount: amount}
	if err := c.Chain.AddTransaction(tx); err != nil {
		return fmt.Errorf("faucet transaction rejected: %w", err)
	}
	c.printf("Queued faucet grant: %s receives %d\n", tx.Recipient, tx.Amount)
	return nil
}

func (c *CLI) cmdMine() error {
	newBlock, result, err := c.Chain.MineBlock()
	if err != nil {
		return err
	}
	c.printf("Mined block #%d\n", newBlock.Height)
	c.printf("  Hash:       %s\n", result.Hash)
	c.printf("  Nonce:      %d\n", result.Nonce)
	c.printf("  Attempts:   %d\n", result.Attempts)
	c.printf("  Difficulty: %d\n", result.Difficulty)
	c.printf("  Time taken: %s\n", result.Elapsed)
	return nil
}

func (c *CLI) cmdPrint() {
	for _, b := range c.Chain.Blocks {
		c.printf("%s\n", b.String())
	}
}

func (c *CLI) cmdValidate() error {
	err := c.Chain.Validate()
	if err != nil {
		c.printf("Chain is INVALID: %v\n", err)
		return nil // validation failure is reported, not a CLI-level error
	}
	c.printf("Chain is VALID (%d blocks)\n", len(c.Chain.Blocks))
	return nil
}

func (c *CLI) cmdBalances() {
	l, err := c.Chain.Ledger()
	if err != nil {
		c.printf("Could not compute balances: %v\n", err)
		return
	}
	balances := l.Balances()
	if len(balances) == 0 {
		c.printf("(no accounts yet)\n")
		return
	}
	for account, bal := range balances {
		c.printf("  %-20s %d\n", account, bal)
	}
}

func (c *CLI) cmdBalance(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: balance <account>")
	}
	l, err := c.Chain.Ledger()
	if err != nil {
		return fmt.Errorf("could not compute balance: %w", err)
	}
	c.printf("%s: %d\n", args[0], l.Balance(args[0]))
	return nil
}

func (c *CLI) cmdPending() {
	if len(c.Chain.Pending) == 0 {
		c.printf("(no pending transactions)\n")
		return
	}
	for _, tx := range c.Chain.Pending {
		c.printf("  %s -> %s : %d\n", tx.Sender, tx.Recipient, tx.Amount)
	}
}

func (c *CLI) cmdSave() error {
	if err := c.Chain.Save(c.DataFile); err != nil {
		return err
	}
	c.printf("Chain saved to %s\n", c.DataFile)
	return nil
}

// RunREPL reads whitespace-separated commands from in, one per line, until
// EOF or an "exit"/"quit" command. Each line's output is auto-saved to disk
// so state survives between runs (FR-8), unless DataFile is empty.
func (c *CLI) RunREPL(in io.Reader) {
	scanner := bufio.NewScanner(in)
	c.printf("Toy Blockchain CLI. Type 'help' for commands, 'exit' to quit.\n")
	for {
		c.printf("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}
		args := strings.Fields(line)
		if err := c.Run(args); err != nil {
			c.printf("Error: %v\n", err)
			continue
		}
		if c.DataFile != "" {
			if err := c.Chain.Save(c.DataFile); err != nil {
				c.printf("Warning: could not save chain: %v\n", err)
			}
		}
	}
	c.printf("Goodbye.\n")
}
