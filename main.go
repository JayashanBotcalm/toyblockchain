// Command toyblockchain is a minimal, single-process, proof-of-work
// blockchain and ledger simulator. See README.md for usage.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"toyblockchain/chain"
	"toyblockchain/cli"
)

func main() {
	difficulty := flag.Int("difficulty", chain.DefaultDifficulty, "number of required leading hex zeros for proof-of-work")
	maxTx := flag.Int("maxtx", chain.DefaultMaxTxPerBlock, "maximum transactions per mined block")
	dataFile := flag.String("data", "data/chain.json", "path to the chain's persistence file")
	flag.Parse()

	c, err := chain.Load(*dataFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("No existing chain found at %s; creating a new one (difficulty=%d).\n", *dataFile, *difficulty)
			c = chain.New(*difficulty, *maxTx)
			if err := c.Save(*dataFile); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not save initial chain: %v\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "failed to load chain from %s: %v\n", *dataFile, err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Loaded existing chain from %s (%d blocks).\n", *dataFile, len(c.Blocks))
	}

	app := cli.New(c, *dataFile, os.Stdout)

	// One-shot mode: `toyblockchain <command> [args...]`
	if flag.NArg() > 0 {
		if err := app.Run(flag.Args()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := c.Save(*dataFile); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save chain: %v\n", err)
		}
		return
	}

	// Interactive mode.
	app.RunREPL(os.Stdin)
}
