# Toy Blockchain and Ledger Simulator

A minimal, single-process, proof-of-work blockchain and ledger written in
pure Go (standard library only, plus `crypto/ed25519` for signatures).
Built for the Backend Engineering Internship take-home assessment.

Beyond the required functionality, this build implements all five optional
stretch goals from the assessment brief: digital signatures, a Merkle root,
concurrent mining, automatic difficulty retargeting, and fork resolution.
It also goes one step past the listed stretch goals with **Merkle inclusion
proofs** (light-client-style verification that a single transaction belongs
to a block, without the block's full transaction list) — on top of a
difficulty schedule, bounded/cancellable mining, atomic saves, and a
process-level file lock that were added to close specific correctness and
robustness issues found during review. Every package, including the `cli`
package, has its own test file; the suite currently stands at 66 tests.

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

`-difficulty` (and the retarget flags below) only matter the first time a
chain is created at the given `-data` path; on later runs the saved chain
— and the difficulty schedule / retarget policy it already has — is loaded
automatically. Use `setdifficulty` to change the difficulty for future
blocks on an existing chain. There is no compiled binary committed to this
repository; a fresh clone builds with the single `go build` command above.

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
| `-mining-workers` | `0` | Number of concurrent mining goroutines; `0` uses `runtime.NumCPU()`. |
| `-lock-timeout` | `5s` | How long to wait for the chain file lock before giving up. |
| `-retarget` | `false` | Enable automatic difficulty retargeting on a *new* chain. |
| `-retarget-interval` | `5` | Number of blocks per retarget window. |
| `-target-block-time` | `5s` | Target time per block; must be at least 1 second. |
| `-min-difficulty` | `1` | Lower bound automatic retargeting will not go below. |
| `-max-difficulty` | `8` | Upper bound automatic retargeting will not exceed. |

## Running the tests

```bash
go test ./...          # run everything
go test ./... -v       # verbose, see every test case by name
go test ./... -race    # confirm concurrent mining has no data races
go vet ./...           # static analysis
gofmt -l .              # should print nothing if the code is gofmt-clean
```

A `Makefile` is included with `build`, `test`, `vet`, `fmt`, `check`
(runs `fmt`, `vet`, then `test`), `run`, and `clean` targets.

The full suite is 66 tests across the `block`, `chain`, `ledger`, and
`cli` packages. Note that `-race` requires a working 64-bit cgo C
toolchain; on Windows with an older 32-bit-only MinGW gcc it fails to
*build* (`cc1.exe: sorry, unimplemented: 64-bit mode not compiled in`)
rather than reporting a race — run it on Linux/macOS or install a 64-bit
gcc (e.g. mingw-w64) in that case.

## Commands (CLI reference)

| Command | Usage | Description |
|---|---|---|
| `wallet-create` | `wallet-create <name>` | Generate an Ed25519 keypair and store it under a friendly name. |
| `wallet-list` | `wallet-list` | List every stored wallet name and its address. |
| `send` / `addtx` | `send <wallet-name> <recipient-address> <amount>` | Sign a transaction with the named wallet's private key and queue it. |
| `faucet` | `faucet <recipient-address> <amount>` | Mint new funds into an account (stands in for a coinbase transaction). The only unsigned transaction type. |
| `mine` | `mine` | Mine a new block from pending transactions using concurrent proof-of-work (bounded by timeout / attempts / nonce). |
| `setdifficulty` | `setdifficulty <n>` | Schedule difficulty `n` starting from the next block height onward. Already-mined blocks keep their original difficulty. |
| `policy` | `policy` | Print the manual difficulty schedule (which height each rule starts at). |
| `retarget` | `retarget` | Show the automatic-retargeting configuration, the next retarget height, and the difficulty the next block will actually use. |
| `retarget-config` | `retarget-config <on\|off> [interval] [target-seconds] [min] [max]` | Enable or disable automatic retargeting (only before the first normal block is mined). |
| `work` | `work` | Show the chain's total accumulated proof-of-work (`sum(16^difficulty)` over all blocks), used to compare competing chains. |
| `resolvefork` | `resolvefork <candidate-chain.json>` | Load another chain file and, if it is fully valid, shares this chain's genesis, and has strictly greater accumulated work, adopt it. |
| `print` | `print` | Print every block in the chain, human-readable, including its Merkle root. |
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
  Address: 458a00afa186b42e2626f28c4a0866733469a9d3
> faucet 458a00afa186b42e2626f28c4a0866733469a9d3 500
Queued faucet grant: 458a00afa186b42e2626f28c4a0866733469a9d3 receives 500
> mine
Mined block #1
  Hash:       000245e416120786046ac95271933405e6bf2aaea18d91ddbdc1620cad5bea3b
  Nonce:      13834
  Attempts:   13304
  Difficulty: 3
  Workers:    4
  Time:       7.7072ms
> print
Block #1
  Timestamp:  2026-07-14T04:38:48Z
  PrevHash:   00073a7051ad7b3366982085f2dca48934130bbb5908b635254d3f7d59ac7235
  MerkleRoot: 136c3a4bfe56a561a5e1b58fcd919ca582aa954a7e69ba281eeda1b9b0011679
  Hash:       000245e416120786046ac95271933405e6bf2aaea18d91ddbdc1620cad5bea3b
  Nonce:      13834
  Difficulty: 3
  Transactions:
    - FAUCET -> 458a00afa186b42e2626f28c4a0866733469a9d3 : 500 (tx nonce 0)
```

(Real output captured while exercising this build; see `REPORT.pdf` for
the full walkthrough and additional experiments, including tamper
detection, difficulty-vs-effort timings, an automatic-retargeting session,
and a two-chain fork-resolution demo.)

## Project layout

```
toyblockchain/
├── main.go              # flag parsing, chain load/create, file locking, REPL vs one-shot dispatch
├── block/                # Block, Transaction, signatures, Merkle root + inclusion proofs, deterministic hashing, concurrent bounded mining
│   ├── block.go
│   ├── block_test.go
│   ├── merkle.go
│   ├── merkle_test.go
│   ├── merkle_proof.go
│   └── merkle_proof_test.go
├── chain/                # Chain, difficulty schedule + retargeting, fork resolution, validation, atomic persistence, file locking
│   ├── chain.go
│   ├── chain_test.go
│   ├── retarget.go
│   ├── retarget_test.go
│   ├── fork.go
│   └── fork_test.go
├── ledger/               # Account balances and nonces derived from chain history
│   ├── ledger.go
│   └── ledger_test.go
├── cli/                  # Wallet store, command dispatch, REPL loop
│   ├── cli.go
│   └── cli_test.go
├── data/                 # Default location for the persisted chain.json / wallets.json
├── REPORT.pdf            # Research report (Section 7 of the assessment brief)
└── README.md
```

Each package has one clear responsibility, matching the brief's suggested
`block` / `chain` / `ledger` / `cli` split, and `main.go` is a thin wiring
layer with no business logic of its own. Difficulty retargeting and fork
resolution live in their own files (`retarget.go`, `fork.go`) inside
`chain` rather than being folded into `chain.go`, since they are each a
self-contained policy on top of the same `Chain` type.

## Design decisions

**Hashing.** A block's hash is SHA-256 over the JSON encoding of an
explicit `hashPayload` struct: height, timestamp, **Merkle root** (not the
raw transaction list — see below), previous hash, nonce, and difficulty,
in that order. `encoding/json` always serialises struct fields in
declaration order, so this is fully deterministic across runs and
machines (`TestHashDeterministic`, `TestHashChangesWithContent`).

**Merkle root.** A block's transactions are summarised by a Merkle root
rather than hashed as a flat, directly embedded list. `CalculateMerkleRoot`
hashes each transaction individually, then repeatedly hashes adjacent
pairs together (duplicating the last hash when a level has an odd count)
until one root remains — the standard binary Merkle tree pattern. An empty
block's root is `SHA-256("")`, matching the well-known empty-tree value.
`Chain.Validate` independently recomputes the Merkle root from a block's
stored transactions and rejects the block if it doesn't match what's
recorded, *before* even checking the block hash — so editing a transaction
without updating the Merkle root is caught immediately, and updating the
Merkle root without redoing the proof-of-work is caught by the hash check
right after (`TestTamperedMerkleRootRejected`,
`TestTransactionTamperingBreaksMerkleRoot`).

**Merkle inclusion proofs.** On top of the root itself,
`block/merkle_proof.go` implements light-client-style inclusion proofs.
`GenerateMerkleProof(transactions, index)` walks the exact same
pairing-and-duplication structure as `CalculateMerkleRoot`, but instead of
discarding intermediate hashes it records, at each level, the sibling hash
on the path from the chosen transaction up to the root, plus whether that
sibling sits on the left or right (needed to hash each pair back together
in the correct order). `VerifyMerkleProof(txHash, proof, root)` then
recomputes the root from just one transaction's hash and its `log2(n)`
proof steps and compares it to the known root — no other transaction in
the block is ever needed. Covered by five dedicated tests: a valid proof
verifies (`TestMerkleProofVerifiesInclusion`), a proof fails for the wrong
leaf (`TestMerkleProofFailsForWrongLeaf`) and against the wrong root
(`TestMerkleProofFailsAgainstWrongRoot`), out-of-range indexes are
rejected (`TestMerkleProofIndexOutOfRange`), and the single-transaction
edge case works (`TestMerkleProofSingleTransaction`). This is currently a
library-level API in the `block` package — there is deliberately no CLI
command for it yet, since this toy has no light client to consume the
proofs (see Known limitations).

**Concurrent mining.** `Block.Mine` now takes a `context.Context` and a
`MiningLimits{MaxAttempts, MaxNonce, Workers}` and searches the nonce space
across `Workers` goroutines (default: `runtime.NumCPU()`), each striding
through a disjoint slice of nonce values (worker `i` of `n` tries
`i, i+n, i+2n, ...`) so no two workers ever hash the same nonce. Each
worker hashes its own local copy of the block (`candidate := *b`) rather
than mutating the shared block directly, which is what keeps this race
-free under `go test -race`. The first worker to find a valid hash sends
it on a buffered result channel and cancels a shared context so every
other worker stops promptly; attempts are counted with `atomic.Uint64`
across workers so the reported attempt count is exact, not estimated. The
one deliberate exception: **the genesis block is always mined with exactly
one worker.** With multiple workers racing, whichever worker happens to
win is essentially random, so the genesis nonce (and therefore its hash)
would not be reproducible from a fixed difficulty alone across different
machines/core counts; pinning genesis mining to a single worker keeps it
deterministic (`TestGenesisDeterministicAcrossWorkerCounts`), while every
block after it is free to use as many workers as the machine has.

**Digital signatures.** Every non-faucet transaction is signed. A wallet is
an Ed25519 keypair (`wallet-create`); an account address is
`SHA-256(publicKey)[:20 bytes]`, hex-encoded. `Transaction.Sign` signs a
canonical JSON encoding of exactly `{sender, recipient, amount, nonce}` —
deliberately excluding the public key and signature fields themselves, to
avoid a circular signature. `VerifySignature` checks that (a) the supplied
public key really does hash to the claimed sender address, and (b) the
signature verifies against that same canonical payload. The faucet is the
sole exception and is required to carry *no* public key or signature
(`TestFaucetTransactionValidate`).

**Replay and nonce protection.** Each sender's transactions carry a
strictly increasing nonce. Checks in both the pending pool
(`Chain.AddTransaction`) and block replay (`Ledger.ApplyBlock`) reject a
transaction whose nonce doesn't match exactly, stopping a captured,
validly-signed transaction from being resubmitted later
(`TestReplayTransactionRejected`, `TestWrongNonceRejected`).

**Difficulty is a schedule, not a single number — and can now retarget
itself.** `DifficultySchedule` is a list of `{start_height, difficulty}`
manual rules; `setdifficulty` appends a rule taking effect from the next
height onward. On top of that, `RetargetConfig` lets a chain (created with
`-retarget=true`) recompute its own difficulty automatically every
`Interval` blocks, by comparing the actual time taken over the last window
to `Interval-1 × TargetBlockSeconds`: if the window took less than half
the target time, difficulty goes up by 1; if it took more than double,
difficulty goes down by 1; either way the result is clamped to
`[MinDifficulty, MaxDifficulty]`. A manual `setdifficulty` rule at the same
height always takes priority over an automatic retarget at that height.
Crucially, `Chain.Validate` does not trust a block's own recorded
difficulty at all — for every block it independently recomputes what the
difficulty *should* be (from the manual schedule and the retarget policy
together) and rejects the block if the two disagree, before it even looks
at the hash. This closes the cheat where an attacker lowers a block's
recorded difficulty to re-mine it cheaply
(`TestTamperedDifficultyRejected`), and it's also what makes the retarget
policy itself tamper-evident. Retarget policy can only be set when a chain
is first created or before its first normal block is mined
(`TestRetargetConfigurationCannotChangeAfterMiningStarts`), since changing
the rules for already-mined blocks after the fact would make history
unverifiable.

**Bounded mining.** `Block.Mine` is not allowed to run forever: a
wall-clock `-mining-timeout` (`context.WithTimeout`), a `MaxAttempts` hard
cap, a `MaxNonce` hard cap, and an explicit guard against nonce overflow
all apply on top of the concurrency described above. Hitting any limit
returns a typed sentinel error (`ErrMiningTimeout`, `ErrMaxAttempts`,
`ErrMaxNonce`, `ErrMiningCancelled`) instead of hanging the CLI
(`TestMineStopsAtMaximumAttempts`, `TestMineCanBeCancelled`). `NewBlock`
fixes a block's difficulty permanently when the block is created, and
`Mine` only ever reads `b.Difficulty` — so a block's difficulty can never
be silently substituted by a different value at mine time
(`TestMineDoesNotReplaceBlockDifficulty`). If mining fails for any reason,
`Chain.MineBlock` returns the error without removing pending transactions
or appending a half-mined block, so a retry with a larger budget picks up
exactly where it left off (`TestMiningFailureKeepsPendingTransactions`).

**Fork resolution.** `Chain.TotalWork` sums `16^difficulty` over every
block — the expected number of hash attempts needed to produce a hash at
that difficulty — rather than just comparing chain length, so a longer
but lower-difficulty chain does not automatically beat a shorter,
higher-difficulty one. `Chain.ResolveFork(candidate)` only adopts a
candidate chain if: the candidate itself passes full validation, both
chains share the same genesis block hash, both chains use the same
difficulty-schedule/retarget/max-tx-per-block policy (so you can't get
tricked into adopting a chain from an incompatible "network"), and the
candidate's total work is *strictly* greater than the current chain's
(`TestResolveForkAdoptsStrongerCandidate`,
`TestResolveForkRejectsEqualOrWeakerCandidate`,
`TestResolveForkRejectsDifferentGenesis`,
`TestResolveForkRejectsInvalidCandidate`). On adoption,
`CommonAncestorHeight` reports where the two chains diverged, and pending
transactions from both the losing and winning chain's pools are merged,
deduplicated, and revalidated against the new chain rather than either
being silently kept or silently dropped
(`TestResolveForkRevalidatesPendingTransactions`). This is a manual
`resolvefork <file>` CLI command, not an automatic network protocol —
there are still no peers or gossip (see Known limitations).

**Amounts as `int64`.** Transaction amounts are whole numbers rather than
floats, to avoid floating-point rounding creeping into balance arithmetic.

**Faucet instead of a full coinbase.** A special `FAUCET` sender is exempt
from the balance check so new funds can be minted to bootstrap an account,
while still going through the same structural validation as every other
transaction.

**Ledger is always derived, never stored as ground truth.**
`Ledger.Rebuild` replays every transaction in every block, in order, from
an empty state, so balances can never drift from what the chain actually
contains.

**Atomic, locked persistence.** `Chain.Save` writes to `<path>.tmp`,
`fsync`s it, and only then `os.Rename`s it over the real path, so a crash
mid-write can never leave a corrupt `chain.json` behind
(`TestAtomicSaveLeavesNoTemporaryFile`). Separately, `main` acquires an
exclusive `<path>.lock` file (created with `O_EXCL`, so creation itself is
the race-free check) for the whole process lifetime, so a second terminal
running the CLI against the same data file is refused outright instead of
loading stale state and clobbering the first process's writes
(`TestFileLockRejectsSecondProcess`).

## Known limitations

- **No networking or consensus protocol.** Fork resolution (above) lets a
  human manually feed one chain file into another and have the stronger
  one win, but there is still no peer discovery, gossip, or automatic
  syncing between processes — `resolvefork` has to be run by hand against
  a file someone else produced.
- **Merkle proofs are a library API only.** `GenerateMerkleProof` /
  `VerifyMerkleProof` exist and are tested, but no CLI command exposes
  them, because this toy has no light client that would consume a proof —
  the CLI always has the full chain locally anyway.
- **Wallet private keys are stored in plaintext JSON.** Adequate for a
  local CLI demo; a real wallet would encrypt keys at rest.
- **Ledger rebuild is O(n) on every query**, and `ExpectedDifficulty`
  recomputes the schedule/retarget history from height 1 on every call
  (so `Validate` is roughly O(n²) in the number of blocks). Both are fine
  at toy scale (hundreds of blocks); a production system would maintain
  balances and difficulty incrementally instead of replaying from scratch.
- **JSON persistence, not a real database.** No indexing, no partial-file
  corruption recovery beyond the atomic-rename guarantee above.
- **Timestamp check is monotonic-only**, and the retarget window's timing
  measurement trusts each block's self-reported timestamp — there's no
  networked peer to cross-check the clock against, which is a deliberately
  weak (but honestly-documented) check appropriate for a toy.
- **The file lock isn't released on a hard kill.** `Ctrl+C` / `exit` /
  a normal crash-with-unwind all release it via `defer`; a `SIGKILL` or
  power loss would leave a stale `.lock` file that needs manual deletion.

## Stretch goals attempted

All five stretch goals listed in the assessment brief are implemented and
tested:

- **Digital signatures** — Ed25519 wallets, signed transactions, nonce
  based replay protection.
- **Merkle root** — transactions are summarised by a Merkle root instead
  of being hashed as a raw list; tamper detection covers both the
  transaction content and the root itself. Going one step further,
  `log2(n)`-sized Merkle *inclusion proofs* can be generated and verified
  for a single transaction (`GenerateMerkleProof` / `VerifyMerkleProof`).
- **Concurrent mining** — the nonce space is searched across
  `runtime.NumCPU()` (or `-mining-workers`) goroutines, verified race-free
  with `go test -race`, with genesis pinned to one worker for determinism.
- **Difficulty retargeting** — automatic difficulty adjustment toward a
  target block time, bounded by configurable min/max difficulty, layered
  underneath the existing manual difficulty schedule.
- **Fork resolution** — accumulated-work (not just height) comparison
  between two chain files, with genesis/policy compatibility checks and
  safe pending-pool merging on adoption.

Beyond the five listed stretch goals, this submission also added:

- **Merkle inclusion proofs** — generation and verification of a single
  transaction's `log2(n)` sibling-hash path against a block's root,
  with five dedicated tests.
- **A test suite for the `cli` package** — 11 tests exercising command
  dispatch end-to-end (wallet creation, faucet/mine/balances, signed
  sends, unknown-command and unknown-wallet rejection, difficulty policy,
  retarget configuration rules, tamper reporting through `validate`,
  pending/save, and work/fork resolution), so every package in the
  repository now has direct coverage.
- **Atomic file saves and a process-level file lock**, since those felt
  like the areas most likely to bite a real user of a CLI tool that can
  be run twice against the same file or interrupted mid-write.

**Not attempted:** an actual peer-to-peer network — fork resolution here
is manual, file-to-file, not an automatic gossip protocol. This remains
the single largest gap versus a real blockchain and is discussed, with a
sketch of how it would be added, in `REPORT.pdf`.
