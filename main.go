// Command toyblockchain runs the Toy Blockchain CLI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"toyblockchain/block"
	"toyblockchain/chain"
	"toyblockchain/cli"
)

func main() {
	difficulty := flag.Int(
		"difficulty",
		chain.DefaultDifficulty,
		"initial difficulty for a new chain",
	)

	maxTransactions := flag.Int(
		"maxtx",
		chain.DefaultMaxTxPerBlock,
		"maximum transactions per block",
	)

	dataFile := flag.String(
		"data",
		"data/chain.json",
		"chain JSON path",
	)

	walletFile := flag.String(
		"wallets",
		"data/wallets.json",
		"wallet store path",
	)

	miningTimeout := flag.Duration(
		"mining-timeout",
		15*time.Second,
		"maximum mining time",
	)

	maxAttempts := flag.Uint64(
		"max-attempts",
		5_000_000,
		"maximum hash attempts; 0 disables",
	)

	maxNonce := flag.Uint64(
		"max-nonce",
		10_000_000,
		"maximum nonce; 0 disables",
	)

	lockTimeout := flag.Duration(
		"lock-timeout",
		5*time.Second,
		"time to wait for the chain file lock",
	)

	flag.Parse()

	if err := os.MkdirAll(
		filepath.Dir(*dataFile),
		0755,
	); err != nil {
		fatal(err)
	}

	// Hold the lock for the complete application process.
	// A second terminal cannot load stale state and overwrite this process.
	fileLock, err := chain.AcquireFileLock(
		*dataFile,
		*lockTimeout,
	)
	if err != nil {
		fatal(err)
	}

	defer func() {
		if err := fileLock.Release(); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(
				os.Stderr,
				"warning: could not release lock:",
				err,
			)
		}
	}()

	miningLimits := block.MiningLimits{
		MaxAttempts: *maxAttempts,
		MaxNonce:    *maxNonce,
	}

	currentChain, err := chain.Load(*dataFile)

	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fatal(err)
		}

		ctx, cancel := context.WithTimeout(
			context.Background(),
			*miningTimeout,
		)

		currentChain, err = chain.New(
			ctx,
			*difficulty,
			*maxTransactions,
			miningLimits,
		)

		cancel()

		if err != nil {
			fatal(err)
		}

		if err := currentChain.Save(*dataFile); err != nil {
			fatal(err)
		}

		fmt.Printf(
			"Created new chain at %s (difficulty=%d).\n",
			*dataFile,
			*difficulty,
		)
	} else {
		fmt.Printf(
			"Loaded chain from %s (%d blocks).\n",
			*dataFile,
			len(currentChain.Blocks),
		)

		// The flag must not silently replace an existing chain's policy.
		// Difficulty changes must use setdifficulty.
		nextExpected := currentChain.ExpectedDifficulty(
			currentChain.Latest().Height + 1,
		)

		if *difficulty != chain.DefaultDifficulty &&
			*difficulty != nextExpected {
			fmt.Printf(
				"Note: -difficulty only creates a new chain. "+
					"Use 'setdifficulty %d' for future blocks.\n",
				*difficulty,
			)
		}
	}

	application := cli.New(
		currentChain,
		*dataFile,
		*walletFile,
		os.Stdout,
		*miningTimeout,
		miningLimits,
	)

	// One-shot mode.
	if flag.NArg() > 0 {
		if err := application.Run(flag.Args()); err != nil {
			fatal(err)
		}

		if err := currentChain.Save(*dataFile); err != nil {
			fatal(err)
		}

		return
	}

	// Interactive mode.
	application.RunREPL(os.Stdin)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
