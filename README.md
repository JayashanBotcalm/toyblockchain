# Toy Blockchain and Ledger Simulator

A minimal, single-process, proof-of-work blockchain and ledger written in
pure Go (standard library only, plus `crypto/ed25519` for signatures).
Built for the Backend Engineering Internship take-home assessment.

It supports signed transactions, a mineable append-only chain with a
configurable difficulty schedule, full-chain validation with tamper
detection, JSON persistence with atomic writes, and a process-level file
lock so two copies of the program can't corrupt the same chain file.

## Requirements

- Go 1.22 or newer

## Build and run

```bash
# From the project root
go build -o toyblockchain .

# Interactive mode (REPL)
./toyblockchain -difficulty=3 -mining-timeout=15s -max-attempts=5000000 -max-nonce=10000000 \
    -data=data/chain.json -wallets=data/wallets.json

# One-shot mode (each invocation loads the chain, runs one command, saves, and exits)
./toyblockchain -data=data/chain.json wallet-create alice
./toyblockchain -data=data/chain.json faucet <alice-address> 500
./toyblockchain -data=data/chain.json mine
./toyblockchain -data=data/chain.json print
./toyblockchain -data=data/chain.json balances
./toyblockchain -data=data/chain.json validate
```

`-difficulty` only matters the first time a chain is created at the given
`-data` path; on later runs the saved chain, and the difficulty schedule it
already has, is loaded automatically. Use `setdifficulty` to change the
difficulty for future blocks on an existing chain (see below). There is no
compiled binary committed to this repository; a fresh clone builds with the
single `go build` command above.

### Flags

| Flag | Default | Meaning |
|---|---|---|
| `-difficulty` | `4` | Difficulty used only when a *new* chain is created. |
| `-maxtx` | `10` | Maximum pending transactions swept into one block by `mine`. |
| `-data` | `data/chain.json` | Path to the persisted chain JSON file. |
| `-wallets` | `data/wallets.json` | Path to the persisted wallet store. |
| `-mining-timeout` | `15s` | Wall-clock budget for one mining attempt (via `context.WithTimeout`). |
| `-max-attempts` | `5000000` | Hard cap on hash attempts per mining call; `0` disables the cap. |
| `-max-nonce` | `10000000` | Hard cap on the nonce value tried; `0` disables the cap. |
| `-lock-timeout` | `5s` | How long to wait for the chain file lock before giving up. |

## Running the tests

```bash
go test ./...          # run everything
go test ./... -v       # verbose, see every test case by name
go vet ./...           # static analysis
gofmt -l .              # should print nothing if the code is gofmt-clean
```

A `Makefile` is included with `build`, `test`, `vet`, `fmt`, `check`
(runs `fmt`, `vet`, then `test`), `run`, and `clean` targets.

## Commands (CLI reference)

| Command | Usage | Description |
|---|---|---|
| `wallet-create` | `wallet-create <name>` | Generate an Ed25519 keypair and store it under a friendly name. |
| `wallet-list` | `wallet-list` | List every stored wallet name and its address. |
| `send` / `addtx` | `send <wallet-name> <recipient-address> <amount>` | Sign a transaction with the named wallet's private key and queue it. |
| `faucet` | `faucet <recipient-address> <amount>` | Mint new funds into an account (stands in for a coinbase transaction, used to bootstrap balances). The only unsigned transaction type. |
| `mine` | `mine` | Mine a new block from pending transactions (bounded proof-of-work). |
| `setdifficulty` | `setdifficulty <n>` | Schedule difficulty `n` starting from the next block height onward. Already-mined blocks keep their original difficulty. |
| `policy` | `policy` | Print the full difficulty schedule (which height each difficulty rule starts at). |
| `print` | `print` | Print every block in the chain, human-readable. |
| `validate` | `validate` | Validate the whole chain; reports pass/fail and the first offending block. |
| `balances` | `balances` | List every known account and its balance. |
| `balance` | `balance <address>` | Show a single account's balance. |
| `pending` | `pending` | List transactions waiting to be mined. |
| `save` | `save` | Persist the chain to disk immediately (this also happens automatically after every command). |
| `help` | `help` | Show the command list. |

In interactive mode, type `exit` or `quit` to leave the REPL.

### Example session

```
> wallet-create alice
Created wallet alice
  Address: 3a47f5505930e7b01d869bb5664ad38adbe91983
> wallet-create bob
Created wallet bob
  Address: fd961f486a53e9e283d03207c8270c539ab85357
> faucet 3a47f5505930e7b01d869bb5664ad38adbe91983 500
Queued faucet grant: 3a47f5505930e7b01d869bb5664ad38adbe91983 receives 500
> mine
Mined block #1
  Hash:       0002877db03f23492e83f46d977c01f2fde46ecc07574d60d92442950ebb1808
  Nonce:      2111
  Attempts:   2112
  Difficulty: 3
  Time:       2.8549ms
> send alice fd961f486a53e9e283d03207c8270c539ab85357 100
Queued signed transaction: 3a47f5505930e7b01d869bb5664ad38adbe91983 -> fd961f486a53e9e283d03207c8270c539ab85357 : 100 (nonce 1)
> mine
Mined block #2
  Hash:       000684e355d22552e61e0e8001a619f974b8b35cfaf08abb6ae3654184e39519
  Nonce:      597
  Attempts:   598
  Difficulty: 3
  Time:       1.1182ms
> balances
  3a47f5505930e7b01d869bb5664ad38adbe91983   400
  fd961f486a53e9e283d03207c8270c539ab85357   100
> validate
Chain is VALID (3 blocks)
```

(Real output captured while exercising this build; see `REPORT.pdf` for
the full walkthrough and additional experiments, including what happens
when the chain file is tampered with directly.)

## Project layout

```
toyblockchain/
├── main.go              # flag parsing, chain load/create, file locking, REPL vs one-shot dispatch
├── block/                # Block, Transaction, signatures, deterministic hashing, bounded mining
│   ├── block.go
│   └── block_test.go
├── chain/                # Chain, difficulty schedule, validation, atomic persistence, file locking
│   ├── chain.go
│   └── chain_test.go
├── ledger/               # Account balances and nonces derived from chain history
│   ├── ledger.go
│   └── ledger_test.go
├── cli/                  # Wallet store, command dispatch, REPL loop
│   └── cli.go
├── data/                 # Default location for the persisted chain.json / wallets.json
├── REPORT.pdf            # Research report (Section 7 of the assessment brief)
└── README.md
```

Each package has one clear responsibility, matching the brief's suggested
`block` / `chain` / `ledger` / `cli` split, and `main.go` is a thin wiring
layer with no business logic of its own.

## Design decisions

**Hashing.** A block's hash is SHA-256 over the JSON encoding of an
explicit `hashPayload` struct containing, in this fixed order: height,
timestamp, the full transaction list (each transaction serialised as
sender, recipient, amount, nonce, public key, signature), previous hash,
nonce, and difficulty. The block's own `Hash` field is excluded, since it
cannot be an input to itself. `encoding/json` always serialises struct
fields in declaration order (not map order), so this is fully deterministic
across runs and machines. This is unit-tested directly
(`block_test.go`, `TestHashDeterministic`, `TestHashChangesWithContent`).

**Digital signatures.** Every non-faucet transaction is signed. A wallet is
an Ed25519 keypair (`wallet-create`); an account address is
`SHA-256(publicKey)[:20 bytes]`, hex-encoded. `Transaction.Sign` signs a
canonical JSON encoding of exactly `{sender, recipient, amount, nonce}` —
deliberately excluding the public key and signature fields themselves, to
avoid a circular signature. `VerifySignature` checks that (a) the supplied
public key really does hash to the claimed sender address, and (b) the
signature verifies against that same canonical payload, so any change to
the amount, recipient, or nonce after signing invalidates the signature.
The faucet is the sole exception and is required to carry *no* public key
or signature (`TestFaucetTransactionValidate`).

**Replay and nonce protection.** Each sender's transactions carry a
strictly increasing nonce. `Ledger.NextNonce` returns "last accepted
nonce + 1", and both the pending pool (`Chain.AddTransaction`) and block
replay (`Ledger.ApplyBlock`) reject a transaction whose nonce doesn't match
exactly. This stops a captured, validly-signed transaction from being
resubmitted later to drain an account a second time
(`TestReplayTransactionRejected`, `TestWrongNonceRejected`).

**Amounts as `int64`.** Transaction amounts are whole numbers (`int64`)
rather than floats, to avoid floating-point rounding creeping into balance
arithmetic. A production system would use fixed-point integers (for
example the smallest indivisible unit of a currency) for the same reason;
this toy just skips the "convert to smallest unit" step since there's no
defined currency.

**Faucet instead of a full coinbase.** Rather than modelling miner
rewards, a special `FAUCET` sender is exempt from the balance check, so new
funds can be minted to bootstrap an account. This satisfies FR-4's
suggestion of "a special coinbase or faucet mechanism" with the fewest
moving parts, while still going through the same validation pipeline as
every other transaction (structural checks, and the "no signature allowed"
rule above).

**Ledger is always derived, never stored as ground truth.**
`Ledger.Rebuild` replays every transaction in every block, in order, from
an empty state. Balances are never persisted directly — this guarantees
that whatever the chain says the balances are is what the ledger will
always compute, with no risk of the two drifting apart. The cost is
O(transactions) recomputation on every `balances` / `send` call, which is
fine at this scale (see Known Limitations).

**Difficulty is a schedule, not a single number.** `DifficultySchedule` is
a list of `{start_height, difficulty}` rules. `ExpectedDifficulty(height)`
picks the highest rule whose start height is `<= height`. `setdifficulty`
appends (or edits) a rule that takes effect from the *next* block onward,
so already-mined blocks keep the difficulty they were actually mined at.
Crucially, `Chain.Validate` does not trust a block's own recorded
`Difficulty` field at face value — it independently recomputes the
expected difficulty for that block's height from the schedule and rejects
the block if the two disagree, *before* even looking at the hash. This
closes an obvious cheat where an attacker lowers a block's recorded
difficulty and re-mines it cheaply (`TestTamperedDifficultyRejected`; see
`REPORT.pdf` for a worked example).

**Bounded mining.** `Block.Mine` is not allowed to run forever. It is
wrapped in a `context.Context` (a wall-clock `-mining-timeout` in `main`),
and additionally takes a `MiningLimits{MaxAttempts, MaxNonce}` pair as
independent hard stops, plus an explicit guard against the nonce wrapping
past `math.MaxUint64` back to zero. Hitting any limit returns a typed
sentinel error (`ErrMiningTimeout`, `ErrMaxAttempts`, `ErrMaxNonce`,
`ErrMiningCancelled`) instead of hanging the CLI. `NewBlock` fixes a
block's difficulty permanently when the block is created, and `Mine` reads
only `b.Difficulty` — so difficulty can never be silently substituted by
whatever argument happens to be passed to `Mine` later.

**Mining failure does not lose or partially commit anything.** If mining
fails (timeout, attempt cap, cancellation), `Chain.MineBlock` returns the
error *without* removing the pending transactions and *without* appending
the half-mined block to the chain, so a retry with a larger budget picks up
exactly where it left off (`TestMiningFailureKeepsPendingTransactions`, and
reproduced live — see `REPORT.pdf`).

**Pending-pool double-spend protection.** `Chain.AddTransaction` rebuilds
the ledger and then re-applies every *already-pending* transaction before
validating the new one. This stops a sender from queuing two transactions
that would individually be fine but together overspend, without waiting
for a block to be mined (`TestPendingDoubleSpendRejected`).

**Validation order and "first offending block".** `Chain.Validate` walks
blocks from genesis to tip and, for each block, checks: recorded difficulty
against the schedule, hash recomputation, the proof-of-work target itself,
then either genesis-specific checks (height 0, fixed previous hash) or
height sequencing / timestamp monotonicity / previous-hash linkage for
every later block, and finally replays that block's transactions through a
running ledger (which covers signatures, ownership, nonces, and balances).
It returns on the *first* failure with a `ValidationError{BlockHeight,
Reason}`, satisfying the requirement to identify the first offending
block. See `REPORT.pdf` for a worked example of exactly which check
catches a real tamper attempt, on a real chain file.

**Atomic, locked persistence.** `Chain.Save` writes to `<path>.tmp`,
`fsync`s it, and only then `os.Rename`s it over the real path — an atomic
replace on all platforms this targets, so a crash mid-write can never leave
a half-written, corrupt `chain.json` behind
(`TestAtomicSaveLeavesNoTemporaryFile`). Separately, `main` acquires an
exclusive `<path>.lock` file (created with `O_EXCL`, so creation itself is
the race-free check) for the whole lifetime of the process, so a second
terminal running the CLI against the same data file cannot load stale
state and clobber the first process's writes
(`TestFileLockRejectsSecondProcess`, and reproduced live — see
`REPORT.pdf`).

## Known limitations

- **No networking or consensus.** Explicitly out of scope for this
  assessment. There is exactly one copy of the chain, held by one process
  at a time (enforced by the file lock above).
- **No Merkle tree.** A block's transactions are hashed as a flat, directly
  embedded list rather than summarised by a Merkle root, so there is no way
  for a light client to verify a single transaction's inclusion without
  the whole block. See `REPORT.pdf`'s discussion section for a sketch of
  how this would be added.
- **Wallet private keys are stored in plaintext JSON.** Adequate for a
  local CLI demo; a real wallet would encrypt keys at rest (for example
  with a passphrase-derived key) or use an OS keychain / HSM.
- **Ledger rebuild is O(n) on every query.** Fine for a toy chain of
  hundreds of blocks; a production system would maintain balances
  incrementally (update a balance map as each block is *appended*, rather
  than replaying full history on every read) and only fall back to a full
  replay when validating from scratch.
- **JSON persistence, not a real database.** No indexing, no partial-file
  corruption recovery beyond the atomic-rename guarantee above. Acceptable
  for a single-process CLI tool; not something to build a real service on.
- **Timestamp check is monotonic-only.** `Validate` requires timestamps to
  be non-decreasing but doesn't try to detect "impossible" clock jumps — a
  deliberately weak check appropriate for a toy without networked peers to
  cross-check time against.
- **Difficulty retargeting is manual, not automatic.** `setdifficulty`
  lets an operator schedule a new difficulty, but nothing in this project
  measures recent block times and adjusts difficulty on its own to keep
  block times roughly constant (the "Difficulty retargeting" stretch
  goal).

## Stretch goals attempted

- **Digital signatures** — implemented in full: Ed25519 wallets, signed
  transactions, and signature verification as part of chain validation
  (see Design decisions above).
- Beyond the listed stretch goals, this submission also went further than
  the minimum on persistence and mining robustness, since these felt like
  the areas most likely to bite a real user of a CLI tool: atomic file
  saves, a process-level file lock, and bounded/cancellable mining with
  typed errors instead of an unboundable loop.
- **Not attempted:** Merkle root, concurrent mining, automatic difficulty
  retargeting, fork resolution. `REPORT.pdf`'s discussion section sketches
  how a Merkle root could be added on top of the current design.
