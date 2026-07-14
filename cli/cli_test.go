package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"toyblockchain/block"
	"toyblockchain/chain"
)

// newTestCLI builds a CLI wired to a fresh chain and a wallet file under a
// temporary directory, with mining limits generous enough to always
// succeed quickly at the low difficulties used in these tests, and a
// buffer that captures everything the CLI prints so tests can assert on
// exact output.
func newTestCLI(t *testing.T, difficulty int) (*CLI, *bytes.Buffer) {
	t.Helper()

	limits := block.MiningLimits{
		MaxAttempts: 5_000_000,
		MaxNonce:    10_000_000,
		Workers:     1, // deterministic attempt counts in assertions
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testChain, err := chain.New(ctx, difficulty, 10, limits)
	if err != nil {
		t.Fatalf("creating test chain: %v", err)
	}

	dir := t.TempDir()
	out := &bytes.Buffer{}

	cliApp := New(
		testChain,
		filepath.Join(dir, "chain.json"),
		filepath.Join(dir, "wallets.json"),
		out,
		10*time.Second,
		limits,
	)

	return cliApp, out
}

func mustRun(t *testing.T, c *CLI, args ...string) {
	t.Helper()

	if err := c.Run(args); err != nil {
		t.Fatalf("Run(%v) returned unexpected error: %v", args, err)
	}
}

// TestHelpPrintsCommandList covers the "help" command end to end.
func TestHelpPrintsCommandList(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "help")

	if !strings.Contains(out.String(), "wallet-create <name>") {
		t.Errorf("expected help output to list wallet-create, got: %s", out.String())
	}
}

// TestUnknownCommandRejected checks Run's default case.
func TestUnknownCommandRejected(t *testing.T) {
	c, _ := newTestCLI(t, 1)

	if err := c.Run([]string{"not-a-real-command"}); err == nil {
		t.Fatalf("expected an error for an unknown command")
	}

	if err := c.Run(nil); err == nil {
		t.Fatalf("expected an error when no command is given")
	}
}

// TestWalletCreateAndList covers wallet creation, duplicate rejection and
// listing, exercising the on-disk wallet store round trip.
func TestWalletCreateAndList(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "wallet-create", "alice")

	if !strings.Contains(out.String(), "Created wallet alice") {
		t.Fatalf("expected wallet creation confirmation, got: %s", out.String())
	}

	if err := c.Run([]string{"wallet-create", "alice"}); err == nil {
		t.Fatalf("expected creating a duplicate wallet name to fail")
	}

	if err := c.Run([]string{"wallet-create"}); err == nil {
		t.Fatalf("expected wallet-create with no name to fail")
	}

	out.Reset()
	mustRun(t, c, "wallet-create", "bob")
	mustRun(t, c, "wallet-list")

	listing := out.String()
	if !strings.Contains(listing, "alice") || !strings.Contains(listing, "bob") {
		t.Fatalf("expected wallet-list to show both wallets, got: %s", listing)
	}
}

// TestFaucetMineAndBalances exercises the most common happy path: fund an
// account, mine it into a block, and confirm the balance reflects it.
func TestFaucetMineAndBalances(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "wallet-create", "alice")
	out.Reset()

	// Find alice's address via wallet-list output rather than reaching
	// into the wallet store directly, so this test exercises the same
	// path a real user would.
	mustRun(t, c, "wallet-list")
	address := addressFromWalletList(t, out.String(), "alice")
	out.Reset()

	mustRun(t, c, "faucet", address, "500")
	if !strings.Contains(out.String(), "Queued faucet grant") {
		t.Fatalf("expected faucet queue confirmation, got: %s", out.String())
	}

	out.Reset()
	mustRun(t, c, "mine")
	if !strings.Contains(out.String(), "Mined block #1") {
		t.Fatalf("expected block #1 to be mined, got: %s", out.String())
	}

	out.Reset()
	mustRun(t, c, "balances")
	if !strings.Contains(out.String(), address) || !strings.Contains(out.String(), "500") {
		t.Fatalf("expected balances to show %s with 500, got: %s", address, out.String())
	}

	out.Reset()
	mustRun(t, c, "balance", address)
	if strings.TrimSpace(out.String()) != address+": 500" {
		t.Fatalf("unexpected balance output: %q", out.String())
	}
}

// TestSendSignsAndQueuesTransaction confirms a real wallet-signed transfer
// is accepted and reduces the sender's balance once mined.
func TestSendSignsAndQueuesTransaction(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "wallet-create", "alice")
	mustRun(t, c, "wallet-create", "bob")
	out.Reset()

	mustRun(t, c, "wallet-list")
	aliceAddress := addressFromWalletList(t, out.String(), "alice")
	bobAddress := addressFromWalletList(t, out.String(), "bob")
	out.Reset()

	mustRun(t, c, "faucet", aliceAddress, "100")
	mustRun(t, c, "mine")
	out.Reset()

	mustRun(t, c, "send", "alice", bobAddress, "40")
	if !strings.Contains(out.String(), "Queued signed transaction") {
		t.Fatalf("expected signed transaction confirmation, got: %s", out.String())
	}

	mustRun(t, c, "mine")

	out.Reset()
	mustRun(t, c, "balance", aliceAddress)
	if strings.TrimSpace(out.String()) != aliceAddress+": 60" {
		t.Fatalf("expected alice's balance to be 60, got: %s", out.String())
	}
}

// TestSendUnknownWalletRejected ensures a helpful error is returned
// instead of a panic when the named wallet does not exist.
func TestSendUnknownWalletRejected(t *testing.T) {
	c, _ := newTestCLI(t, 1)

	err := c.Run([]string{"send", "ghost", "someaddress", "10"})
	if err == nil {
		t.Fatalf("expected sending from a nonexistent wallet to fail")
	}
}

// TestSetDifficultyAndPolicy checks that a scheduled difficulty change is
// reported by "policy" and actually takes effect on the next mined block.
func TestSetDifficultyAndPolicy(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "setdifficulty", "2")

	out.Reset()
	mustRun(t, c, "policy")
	if !strings.Contains(out.String(), "from height 1: difficulty 2") {
		t.Fatalf("expected policy to show the scheduled change, got: %s", out.String())
	}

	mustRun(t, c, "faucet", "someaddress", "1")

	out.Reset()
	mustRun(t, c, "mine")
	if !strings.Contains(out.String(), "Difficulty: 2") {
		t.Fatalf("expected block #1 to be mined at the newly scheduled difficulty, got: %s", out.String())
	}
}

// TestRetargetConfigOnlyBeforeMining confirms retarget-config is accepted
// before any normal block is mined and rejected afterward, matching
// Chain.ConfigureRetarget's own rule.
func TestRetargetConfigOnlyBeforeMining(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "retarget-config", "on", "5", "5", "1", "8")
	if !strings.Contains(out.String(), "Retarget configuration updated.") {
		t.Fatalf("expected retarget-config to succeed before mining, got: %s", out.String())
	}

	out.Reset()
	mustRun(t, c, "retarget")
	if !strings.Contains(out.String(), "Enabled:              true") {
		t.Fatalf("expected retarget to report the updated config, got: %s", out.String())
	}

	mustRun(t, c, "faucet", "someaddress", "1")
	mustRun(t, c, "mine")

	if err := c.Run([]string{"retarget-config", "off", "5", "5", "1", "8"}); err == nil {
		t.Fatalf("expected retarget-config to be rejected after the first block was mined")
	}
}

// TestValidateReportsTamperedChain confirms the CLI surfaces a tampered
// chain as INVALID with a reason, rather than erroring out or panicking.
func TestValidateReportsTamperedChain(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "faucet", "someaddress", "10")
	mustRun(t, c, "mine")

	out.Reset()
	mustRun(t, c, "validate")
	if !strings.Contains(out.String(), "Chain is VALID") {
		t.Fatalf("expected an honest chain to validate, got: %s", out.String())
	}

	// Tamper directly, the same way an attacker editing the JSON file
	// would.
	c.Chain.Blocks[1].Transactions[0].Amount = 999999

	out.Reset()
	if err := c.Run([]string{"validate"}); err != nil {
		t.Fatalf("validate should report failure through its own output, not a Go error: %v", err)
	}
	if !strings.Contains(out.String(), "Chain is INVALID") {
		t.Fatalf("expected tampered chain to be reported invalid, got: %s", out.String())
	}
}

// TestPendingAndSave covers the remaining simple read commands and that
// save writes a file that can be loaded back.
func TestPendingAndSave(t *testing.T) {
	c, out := newTestCLI(t, 1)

	mustRun(t, c, "pending")
	if !strings.Contains(out.String(), "no pending transactions") {
		t.Fatalf("expected empty pending pool message, got: %s", out.String())
	}

	out.Reset()
	mustRun(t, c, "faucet", "someaddress", "5")
	mustRun(t, c, "pending")
	if !strings.Contains(out.String(), "someaddress") {
		t.Fatalf("expected pending pool to show the queued faucet grant, got: %s", out.String())
	}

	out.Reset()
	mustRun(t, c, "save")
	if !strings.Contains(out.String(), "Chain saved to") {
		t.Fatalf("expected save confirmation, got: %s", out.String())
	}

	if _, err := chain.Load(c.DataFile); err != nil {
		t.Fatalf("expected the saved chain file to load back cleanly: %v", err)
	}
}

// TestWorkAndResolveFork drives two independent chains through the CLI
// exactly as a user would from two terminals, then resolves the weaker
// one against the stronger one via the "resolvefork" command.
func TestWorkAndResolveFork(t *testing.T) {
	weak, weakOut := newTestCLI(t, 1)
	mustRun(t, weak, "faucet", "weak-recipient", "1")
	mustRun(t, weak, "mine")
	mustRun(t, weak, "save")

	weakOut.Reset()
	mustRun(t, weak, "work")
	if strings.TrimSpace(weakOut.String()) != "Accumulated work: 32" {
		t.Fatalf("expected weak chain's accumulated work to be 32, got: %s", weakOut.String())
	}

	strong, _ := newTestCLI(t, 1)
	mustRun(t, strong, "faucet", "strong-recipient-1", "1")
	mustRun(t, strong, "mine")
	mustRun(t, strong, "faucet", "strong-recipient-2", "1")
	mustRun(t, strong, "mine")
	mustRun(t, strong, "save")

	weakOut.Reset()
	mustRun(t, weak, "resolvefork", strong.DataFile)

	if !strings.Contains(weakOut.String(), "Fork resolved. Stronger chain adopted.") {
		t.Fatalf("expected fork resolution to succeed, got: %s", weakOut.String())
	}

	weakOut.Reset()
	mustRun(t, weak, "work")
	if strings.TrimSpace(weakOut.String()) != "Accumulated work: 48" {
		t.Fatalf("expected weak chain to now report the strong chain's work (48), got: %s", weakOut.String())
	}
}

// addressFromWalletList extracts the address printed for a given wallet
// name from "wallet-list" output, which is formatted as:
//
//	  <name>            <address>
func addressFromWalletList(t *testing.T, listing, name string) string {
	t.Helper()

	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == name {
			return fields[1]
		}
	}

	t.Fatalf("wallet %q not found in wallet-list output: %s", name, listing)
	return ""
}
