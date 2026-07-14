// Package cli implements the Toy Blockchain command-line interface.
package cli

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"toyblockchain/block"
	"toyblockchain/chain"
)

// Wallet stores one toy Ed25519 wallet.
//
// This is suitable for the assessment demonstration only. A production
// wallet would encrypt private keys and use secure key management.
type Wallet struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// WalletStore stores wallets by user-friendly names.
type WalletStore struct {
	Wallets map[string]Wallet `json:"wallets"`
}

func loadWalletStore(path string) (*WalletStore, error) {
	data, err := os.ReadFile(path)

	if os.IsNotExist(err) {
		return &WalletStore{
			Wallets: make(map[string]Wallet),
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading wallet file: %w", err)
	}

	var store WalletStore

	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parsing wallet file: %w", err)
	}

	if store.Wallets == nil {
		store.Wallets = make(map[string]Wallet)
	}

	return &store, nil
}

func (s *WalletStore) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating wallet directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling wallets: %w", err)
	}

	temporaryPath := path + ".tmp"

	if err := os.WriteFile(temporaryPath, data, 0600); err != nil {
		return fmt.Errorf("writing wallet file: %w", err)
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replacing wallet file: %w", err)
	}

	return nil
}

// CLI holds application configuration and state.
type CLI struct {
	Chain       *chain.Chain
	DataFile    string
	WalletFile  string
	Out         io.Writer
	MineTimeout time.Duration
	MineLimits  block.MiningLimits
}

// New creates the command-line application.
func New(
	c *chain.Chain,
	dataFile string,
	walletFile string,
	out io.Writer,
	mineTimeout time.Duration,
	mineLimits block.MiningLimits,
) *CLI {
	return &CLI{
		Chain:       c,
		DataFile:    dataFile,
		WalletFile:  walletFile,
		Out:         out,
		MineTimeout: mineTimeout,
		MineLimits:  mineLimits,
	}
}

func (c *CLI) printf(format string, args ...interface{}) {
	fmt.Fprintf(c.Out, format, args...)
}

// Run executes one CLI command.
func (c *CLI) Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command given; try 'help'")
	}

	command := args[0]
	remainingArgs := args[1:]

	switch command {
	case "help":
		c.help()
		return nil

	case "wallet-create":
		return c.cmdWalletCreate(remainingArgs)

	case "wallet-list":
		return c.cmdWalletList()

	case "send", "addtx":
		return c.cmdSend(remainingArgs)

	case "faucet":
		return c.cmdFaucet(remainingArgs)

	case "mine":
		return c.cmdMine()

	case "setdifficulty":
		return c.cmdSetDifficulty(remainingArgs)

	case "policy":
		c.cmdPolicy()
		return nil

	case "retarget":
		c.cmdRetarget()
		return nil

	case "retarget-config":
		return c.cmdRetargetConfig(remainingArgs)

	case "print":
		c.cmdPrint()
		return nil

	case "validate":
		return c.cmdValidate()

	case "balances":
		c.cmdBalances()
		return nil

	case "balance":
		return c.cmdBalance(remainingArgs)

	case "pending":
		c.cmdPending()
		return nil

	case "save":
		return c.cmdSave()

	default:
		return fmt.Errorf(
			"unknown command %q; try 'help'",
			command,
		)
	}
}

func (c *CLI) help() {
	c.printf(`Available commands:
  wallet-create <name>                 Create an Ed25519 wallet
  wallet-list                          List wallet names and addresses
  send <wallet-name> <recipient> <amount>
                                        Sign and queue a transaction
  addtx <wallet-name> <recipient> <amount>
                                        Alias for send
  faucet <recipient-address> <amount>  Create test funds
  mine                                 Mine pending transactions
  setdifficulty <n>                    Apply n from the next block onward
  policy                               Show the difficulty schedule
  retarget                             Show automatic retarget settings
  retarget-config <on|off> <interval> <target-seconds> <min> <max>
                                        Configure retargeting before block 1
  print                                Print the full blockchain
  validate                             Validate the complete blockchain
  balances                             Show all account balances
  balance <address>                    Show one account balance
  pending                              Show pending transactions
  save                                 Save the blockchain immediately
  help                                 Show this message
`)
}

func (c *CLI) cmdWalletCreate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: wallet-create <name>")
	}

	name := strings.TrimSpace(args[0])

	if name == "" {
		return fmt.Errorf("wallet name cannot be empty")
	}

	store, err := loadWalletStore(c.WalletFile)
	if err != nil {
		return err
	}

	if _, exists := store.Wallets[name]; exists {
		return fmt.Errorf("wallet %q already exists", name)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating wallet key: %w", err)
	}

	wallet := Wallet{
		Address:    block.AddressFromPublicKey(publicKey),
		PublicKey:  hex.EncodeToString(publicKey),
		PrivateKey: hex.EncodeToString(privateKey),
	}

	store.Wallets[name] = wallet

	if err := store.save(c.WalletFile); err != nil {
		return err
	}

	c.printf(
		"Created wallet %s\n  Address: %s\n",
		name,
		wallet.Address,
	)

	return nil
}

func (c *CLI) cmdWalletList() error {
	store, err := loadWalletStore(c.WalletFile)
	if err != nil {
		return err
	}

	if len(store.Wallets) == 0 {
		c.printf("(no wallets)\n")
		return nil
	}

	names := make([]string, 0, len(store.Wallets))

	for name := range store.Wallets {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		c.printf(
			"  %-16s %s\n",
			name,
			store.Wallets[name].Address,
		)
	}

	return nil
}

func (c *CLI) cmdSend(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf(
			"usage: send <wallet-name> <recipient-address> <amount>",
		)
	}

	amount, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}

	store, err := loadWalletStore(c.WalletFile)
	if err != nil {
		return err
	}

	wallet, exists := store.Wallets[args[0]]
	if !exists {
		return fmt.Errorf(
			"wallet %q not found",
			args[0],
		)
	}

	privateKeyBytes, err := hex.DecodeString(wallet.PrivateKey)
	if err != nil {
		return fmt.Errorf("invalid stored private key")
	}

	currentLedger, err := c.Chain.Ledger()
	if err != nil {
		return err
	}

	// Include pending transactions before choosing the next nonce.
	for _, pendingTransaction := range c.Chain.Pending {
		if err := currentLedger.Apply(pendingTransaction); err != nil {
			return fmt.Errorf(
				"invalid pending transaction: %w",
				err,
			)
		}
	}

	transaction := block.Transaction{
		Recipient: args[1],
		Amount:    amount,
		Nonce: currentLedger.NextNonce(
			wallet.Address,
		),
	}

	if err := transaction.Sign(
		ed25519.PrivateKey(privateKeyBytes),
	); err != nil {
		return err
	}

	if err := c.Chain.AddTransaction(transaction); err != nil {
		return fmt.Errorf(
			"transaction rejected: %w",
			err,
		)
	}

	c.printf(
		"Queued signed transaction: %s -> %s : %d (nonce %d)\n",
		transaction.Sender,
		transaction.Recipient,
		transaction.Amount,
		transaction.Nonce,
	)

	return nil
}

func (c *CLI) cmdFaucet(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf(
			"usage: faucet <recipient-address> <amount>",
		)
	}

	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}

	transaction := block.Transaction{
		Sender:    block.FaucetAccount,
		Recipient: args[0],
		Amount:    amount,
		Nonce:     0,
	}

	if err := c.Chain.AddTransaction(transaction); err != nil {
		return fmt.Errorf(
			"faucet transaction rejected: %w",
			err,
		)
	}

	c.printf(
		"Queued faucet grant: %s receives %d\n",
		transaction.Recipient,
		transaction.Amount,
	)

	return nil
}

func (c *CLI) cmdMine() error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		c.MineTimeout,
	)
	defer cancel()

	newBlock, result, err := c.Chain.MineBlock(
		ctx,
		c.MineLimits,
	)
	if err != nil {
		return err
	}

	c.printf("Mined block #%d\n", newBlock.Height)
	c.printf("  Hash:       %s\n", result.Hash)
	c.printf("  Nonce:      %d\n", result.Nonce)
	c.printf("  Attempts:   %d\n", result.Attempts)
	c.printf("  Difficulty: %d\n", result.Difficulty)
	c.printf("  Workers:    %d\n", result.Workers)
	c.printf("  Time:       %s\n", result.Elapsed)

	return nil
}

func (c *CLI) cmdSetDifficulty(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: setdifficulty <n>")
	}

	difficulty, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid difficulty: %w", err)
	}

	if err := c.Chain.SetDifficulty(difficulty); err != nil {
		return err
	}

	c.printf(
		"Difficulty %d will apply from block height %d\n",
		difficulty,
		c.Chain.Latest().Height+1,
	)

	return nil
}

func (c *CLI) cmdPolicy() {
	for _, rule := range c.Chain.DifficultySchedule {
		c.printf(
			"  from height %d: difficulty %d\n",
			rule.StartHeight,
			rule.Difficulty,
		)
	}
}

func (c *CLI) cmdRetarget() {
	config := c.Chain.Retarget

	c.printf("Automatic difficulty retargeting:\n")
	c.printf("  Enabled:              %t\n", config.Enabled)
	c.printf("  Interval:             %d blocks\n", config.Interval)
	c.printf("  Target block time:    %d seconds\n", config.TargetBlockSeconds)
	c.printf("  Minimum difficulty:   %d\n", config.MinDifficulty)
	c.printf("  Maximum difficulty:   %d\n", config.MaxDifficulty)

	nextHeight := c.Chain.NextRetargetHeight()
	if nextHeight < 0 {
		c.printf("  Next retarget height: disabled\n")
	} else {
		c.printf("  Next retarget height: %d\n", nextHeight)
	}

	nextBlockHeight := c.Chain.Latest().Height + 1
	c.printf(
		"  Next block difficulty: %d\n",
		c.Chain.ExpectedDifficulty(nextBlockHeight),
	)
}

func (c *CLI) cmdRetargetConfig(args []string) error {
	if len(args) != 5 {
		return fmt.Errorf(
			"usage: retarget-config <on|off> <interval> <target-seconds> <min> <max>",
		)
	}

	var enabled bool
	switch strings.ToLower(args[0]) {
	case "on", "true", "enabled":
		enabled = true
	case "off", "false", "disabled":
		enabled = false
	default:
		return fmt.Errorf("first value must be on or off")
	}

	interval, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid retarget interval: %w", err)
	}

	targetSeconds, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid target block seconds: %w", err)
	}

	minimum, err := strconv.Atoi(args[3])
	if err != nil {
		return fmt.Errorf("invalid minimum difficulty: %w", err)
	}

	maximum, err := strconv.Atoi(args[4])
	if err != nil {
		return fmt.Errorf("invalid maximum difficulty: %w", err)
	}

	config := chain.RetargetConfig{
		Enabled:            enabled,
		Interval:           interval,
		TargetBlockSeconds: targetSeconds,
		MinDifficulty:      minimum,
		MaxDifficulty:      maximum,
	}

	if err := c.Chain.ConfigureRetarget(config); err != nil {
		return err
	}

	c.printf("Retarget configuration updated.\n")
	c.cmdRetarget()

	return nil
}

func (c *CLI) cmdPrint() {
	for _, currentBlock := range c.Chain.Blocks {
		c.printf("%s\n", currentBlock.String())
	}
}

func (c *CLI) cmdValidate() error {
	if err := c.Chain.Validate(); err != nil {
		c.printf("Chain is INVALID: %v\n", err)
		return nil
	}

	c.printf(
		"Chain is VALID (%d blocks)\n",
		len(c.Chain.Blocks),
	)

	return nil
}

func (c *CLI) cmdBalances() {
	currentLedger, err := c.Chain.Ledger()
	if err != nil {
		c.printf(
			"Could not compute balances: %v\n",
			err,
		)
		return
	}

	balances := currentLedger.Balances()

	if len(balances) == 0 {
		c.printf("(no accounts yet)\n")
		return
	}

	for account, balance := range balances {
		c.printf(
			"  %-42s %d\n",
			account,
			balance,
		)
	}
}

func (c *CLI) cmdBalance(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: balance <address>")
	}

	currentLedger, err := c.Chain.Ledger()
	if err != nil {
		return err
	}

	c.printf(
		"%s: %d\n",
		args[0],
		currentLedger.Balance(args[0]),
	)

	return nil
}

func (c *CLI) cmdPending() {
	if len(c.Chain.Pending) == 0 {
		c.printf("(no pending transactions)\n")
		return
	}

	for _, tx := range c.Chain.Pending {
		c.printf(
			"  %s -> %s : %d (nonce %d)\n",
			tx.Sender,
			tx.Recipient,
			tx.Amount,
			tx.Nonce,
		)
	}
}

func (c *CLI) cmdSave() error {
	if err := c.Chain.Save(c.DataFile); err != nil {
		return err
	}

	c.printf("Chain saved to %s\n", c.DataFile)

	return nil
}

// RunREPL starts interactive mode.
func (c *CLI) RunREPL(in io.Reader) {
	scanner := bufio.NewScanner(in)

	c.printf(
		"Toy Blockchain CLI. Type 'help' or 'exit'.\n",
	)

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

		if err := c.Run(strings.Fields(line)); err != nil {
			c.printf("Error: %v\n", err)
			continue
		}

		if c.DataFile != "" {
			if err := c.Chain.Save(c.DataFile); err != nil {
				c.printf(
					"Warning: could not save chain: %v\n",
					err,
				)
			}
		}
	}

	c.printf("Goodbye.\n")
}
