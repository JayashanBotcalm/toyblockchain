# Toy Blockchain and Ledger Simulator

A minimal, single-process, proof-of-work blockchain and ledger written in
pure Go (standard library only). Built for the Backend Engineering
Internship take-home assessment.

## Requirements

- Go 1.22 or newer

## Build and run

```bash
# From the project root
go build -o toyblockchain .

# Interactive mode (REPL)
./toyblockchain -difficulty=3 -data=data/chain.json

# One-shot mode (each command loads the chain, runs, saves, and exits)
./toyblockchain -difficulty=3 -data=data/chain.json faucet alice 100
./toyblockchain -data=data/chain.json addtx alice bob 30
./toyblockchain -data=data/chain.json mine
./toyblockchain -data=data/chain.json print
./toyblockchain -data=data/chain.json balances
./toyblockchain -data=data/chain.json validate
```

`-difficulty` and `-data` only need to be passed the first time a chain is
created; on later runs the saved chain (and the difficulty it was created
with) is loaded from `-data` automatically. `-maxtx` (default 10) caps how
many pending transactions are swept into a block when mining.

There is no compiled binary committed to this repository; a fresh clone
builds with the single `go build` command above.

## Running the tests

```bash
go test ./...          # run everything
go test ./... -v       # verbose, see every test case by name
go vet ./...           # static analysis
gofmt -l .              # should print nothing (code is gofmt-clean)
```

## Commands (CLI reference)

| Command | Usage | Description |
|---|---|---|
| `addtx` | `addtx <sender> <recipient> <amount>` | Queue a transaction in the pending pool. Rejected immediately if malformed or overspending. |
| `faucet` | `faucet <recipient> <amount>` | Mint new funds into an account (stands in for a coinbase transaction, used to bootstrap balances). |
| `mine` | `mine` | Mine a new block from pending transactions (proof-of-work). |
| `print` | `print` | Print every block in the chain, human-readable. |
| `validate` | `validate` | Validate the whole chain; reports pass/fail and the first bad block. |
| `balances` | `balances` | List every known account and its balance. |
| `balance` | `balance <account>` | Show a single account's balance. |
| `pending` | `pending` | List transactions waiting to be mined. |
| `save` | `save` | Persist the chain to disk immediately (also happens automatically after every command). |
| `help` | `help` | Show the command list. |

In interactive mode, type `exit` or `quit` to leave the REPL.

## Project layout

```
toyblockchain/
├── main.go              # flag parsing, chain load/create, REPL vs one-shot dispatch
├── block/                # Block, Transaction, hashing, mining
│   ├── block.go
│   └── block_test.go
├── chain/                # Chain, validation, persistence
│   ├── chain.go
│   └── chain_test.go
├── ledger/               # Account balances derived from chain history
│   ├── ledger.go
│   └── ledger_test.go
├── cli/                  # Command dispatch and REPL loop
│   └── cli.go
├── data/                 # Default location for the persisted chain.json
├── REPORT.md             # Research report (Section 7 of the brief)
└── README.md
```

Each package has one clear responsibility, matching the brief's suggested
`block` / `chain` / `ledger` / `cli` split, and `main.go` is a thin wiring
layer with no business logic of its own.

## Design decisions

**Hashing.** A block's hash is SHA-256 over the JSON encoding of an
explicit `hashPayload` struct containing, in this fixed order: height,
timestamp, transactions (each: sender, recipient, amount), previous hash,
nonce, and difficulty. The block's own `Hash` field is excluded, since it
cannot be an input to itself. `encoding/json` always serialises struct
fields in declaration order (not map order), so this is fully deterministic
across runs and machines — this is unit-tested directly (see
`block_test.go`, `TestHashDeterministic`).

**Amounts as `int64`.** Transaction amounts are whole numbers (`int64`)
rather than floats, to avoid floating-point rounding creeping into balance
arithmetic. A production system would use fixed-point integers (e.g. cents
or the smallest indivisible unit) for the same reason; this toy just skips
the "convert to smallest unit" step since there's no defined currency.

**Faucet instead of a full coinbase.** Rather than modelling miner rewards,
a special `FAUCET` sender is exempt from the balance check, so new funds can
be minted to bootstrap an account. This satisfies FR-4's suggestion of "a
special coinbase or faucet mechanism" with the least moving parts.

**Ledger is always derived, never stored as ground truth.** `Ledger.Rebuild`
replays every transaction in every block, in order, from an empty state.
Balances are never persisted directly — this guarantees that whatever the
chain says the balances are, is what the ledger will always compute, with
no risk of the two drifting apart. The cost is O(transactions) balance
recomputation on every `balances`/`addtx` call, which is fine at this scale
(see Known Limitations).

**Validation order and "first offending block".** `Chain.Validate` walks
blocks from genesis to tip, and for each block checks, in this order:
height sequencing, timestamp monotonicity, the previous-hash link, hash
recomputation, the proof-of-work target, and finally replays its
transactions through the ledger. It returns on the *first* failure,
satisfying the requirement to identify the first offending block. See
`REPORT.md` for a worked example of exactly which check catches a real
tamper attempt.

**Pending pool double-spend protection.** `AddTransaction` rebuilds the
ledger and then re-applies every *already-pending* transaction before
checking the new one. This stops a sender from queuing two transactions
that would individually be fine but together overspend, without waiting
for a block to be mined.

**Persistence.** The whole `Chain` (blocks + pending pool + config) is
serialised to a single JSON file via `encoding/json`. It's simple, readable
for debugging, and sufficient for a single-process toy. In one-shot CLI
mode the file is saved after every command; in REPL mode it's saved after
every line.

## Known limitations

- **No networking or consensus.** This is explicitly out of scope. There is
  exactly one copy of the chain, held by one process.
- **No signatures.** Anyone can construct a transaction claiming to be any
  sender; there's no cryptographic proof of authorization. See the stretch
  goals section of `REPORT.md` for how this would be added.
- **Ledger rebuild is O(n) on every query.** Fine for a toy chain of
  hundreds of blocks; a production system would maintain balances
  incrementally (e.g. update a balance map as each block is *appended*,
  rather than replaying history every time) and only fall back to a full
  replay when validating from scratch.
- **JSON persistence, not a real database.** No concurrent-writer safety,
  no partial-write protection beyond what `os.WriteFile` gives for free.
  Acceptable for a single-process CLI tool; not something to build a real
  service on.
- **Timestamp check is monotonic-only.** `Validate` requires timestamps to
  be non-decreasing but doesn't try to detect "impossible" clock jumps —
  a deliberately weak check appropriate for a toy without networked peers
  to cross-check time against.

## Stretch goals attempted

None of the optional stretch goals (digital signatures, Merkle root,
concurrent mining, difficulty retargeting, fork resolution) were
implemented in code for this submission, in favour of making the required
core (FR-1 through FR-9) as solid and well-tested as possible within the
timebox. `REPORT.md`'s discussion section sketches how signatures could be
added on top of the current design.
