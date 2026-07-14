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

	miningWorkers := flag.Int(
		"mining-workers",
		0,
		"number of concurrent mining workers; 0 uses CPU count",
	)

	lockTimeout := flag.Duration(
		"lock-timeout",
		5*time.Second,
		"time to wait for the chain file lock",
	)

	retargetEnabled := flag.Bool(
		"retarget",
		chain.DefaultRetargetConfig.Enabled,
		"enable automatic difficulty retargeting for a new chain",
	)

	retargetInterval := flag.Int(
		"retarget-interval",
		chain.DefaultRetargetConfig.Interval,
		"number of normal blocks in each retarget window",
	)

	targetBlockTime := flag.Duration(
		"target-block-time",
		time.Duration(chain.DefaultRetargetConfig.TargetBlockSeconds)*time.Second,
		"target time for one block",
	)

	minDifficulty := flag.Int(
		"min-difficulty",
		chain.DefaultRetargetConfig.MinDifficulty,
		"minimum automatic difficulty",
	)

	maxDifficulty := flag.Int(
		"max-difficulty",
		chain.DefaultRetargetConfig.MaxDifficulty,
		"maximum automatic difficulty",
	)

	flag.Parse()

	if err := os.MkdirAll(
		filepath.Dir(*dataFile),
		0755,
	); err != nil {
		fatal(err)
	}

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
		Workers:     *miningWorkers,
	}

	if *targetBlockTime < time.Second {
		fatal(fmt.Errorf("target block time must be at least 1 second"))
	}

	retargetConfig := chain.RetargetConfig{
		Enabled:            *retargetEnabled,
		Interval:           *retargetInterval,
		TargetBlockSeconds: int64((*targetBlockTime) / time.Second),
		MinDifficulty:      *minDifficulty,
		MaxDifficulty:      *maxDifficulty,
	}

	if err := retargetConfig.Validate(); err != nil {
		fatal(fmt.Errorf("invalid retarget configuration: %w", err))
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

		currentChain, err = chain.NewWithRetarget(
			ctx,
			*difficulty,
			*maxTransactions,
			miningLimits,
			retargetConfig,
		)

		cancel()

		if err != nil {
			fatal(err)
		}

		if err := currentChain.Save(
			*dataFile,
		); err != nil {
			fatal(err)
		}

		fmt.Printf(
			"Created new chain at %s (difficulty=%d, workers=%d, retarget=%t).\n",
			*dataFile,
			*difficulty,
			*miningWorkers,
			retargetConfig.Enabled,
		)
	} else {
		fmt.Printf(
			"Loaded chain from %s (%d blocks).\n",
			*dataFile,
			len(currentChain.Blocks),
		)

		nextExpected :=
			currentChain.ExpectedDifficulty(
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

	if flag.NArg() > 0 {
		if err := application.Run(
			flag.Args(),
		); err != nil {
			fatal(err)
		}

		if err := currentChain.Save(
			*dataFile,
		); err != nil {
			fatal(err)
		}

		return
	}

	application.RunREPL(os.Stdin)
}

func fatal(err error) {
	fmt.Fprintln(
		os.Stderr,
		"Error:",
		err,
	)

	os.Exit(1)
}
