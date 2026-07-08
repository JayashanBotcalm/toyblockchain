# Research Report: Toy Blockchain and Ledger Simulator

This report documents experiments run against my own implementation
(`toyblockchain`), plus a short design write-up and discussion of how this
toy relates to real blockchains. All numbers below were produced by
actually running the code in this repository, not estimated.

## 1. Tamper-evidence experiment

**Setup.** I built a small honest chain of 3 blocks using the CLI:

```
./toyblockchain -difficulty=3 -data=data/chain.json faucet alice 100
./toyblockchain -data=data/chain.json addtx alice bob 30
./toyblockchain -data=data/chain.json mine      # block 1
./toyblockchain -data=data/chain.json addtx bob carol 10
./toyblockchain -data=data/chain.json mine      # block 2
```

**Before tampering**, block 1's first transaction is the faucet grant of
100 to alice, and validation passes:

```
Block 1 tx0 amount: 100
$ ./toyblockchain -data=data/chain.json validate
Loaded existing chain from data/chain.json (3 blocks).
Chain is VALID (3 blocks)
```

**Tamper.** I edited `data/chain.json` directly on disk (simulating an
attacker with raw file access) and changed that one field, from 100 to
100000, *without* touching the block's stored `hash` field or re-mining:

```python
d['blocks'][1]['transactions'][0]['amount'] = 100000
```

**After tampering**, validation fails immediately:

```
Block 1 tx0 amount: 100000
$ ./toyblockchain -data=data/chain.json validate
Loaded existing chain from data/chain.json (3 blocks).
Chain is INVALID: block 1 invalid: stored hash
000d5dce4b6a30654baa653c10c6f01c763d61c726631331c2ebb5a37db40304 does not
match recomputed hash
9e238a233234527b64d4522332ebf8faacb6c2d30103d3fa6139e2ae5d5614ba
```

**Why this happens, precisely.** `Chain.Validate` recomputes each block's
hash from its current on-disk fields (height, timestamp, transactions,
prev-hash, nonce, difficulty) and compares it against the `hash` field
stored in that same block (see `chain.go`, the `recomputed := cur.ComputeHash()`
check). Changing the transaction amount changes the JSON payload that feeds
SHA-256, which — by the avalanche property of a cryptographic hash function
— produces a completely different digest. Since the attacker didn't also
rewrite the stored `hash` field to match, the **hash-recomputation check**
is what catches it, and it is caught at block 1, the exact block that was
altered.

**A more careful attacker.** I also tested what happens if the attacker
goes one step further and patches the block's stored hash to match the new
content (`c.Blocks[2].Hash = c.Blocks[2].ComputeHash()`, see
`TestTamperDetectionOnPrevHash` in `chain/chain_test.go`). This still fails
validation, but for a different reason: the **proof-of-work check** catches
it, because recomputing a hash for new content does not, in general,
produce a hash that still satisfies the difficulty target — the attacker
would need to re-run mining (search for a new nonce) to fix that up, and
doing so for one block breaks the `prev_hash` link to the next block,
requiring every subsequent block to be re-mined too. This is the same
"re-mine the tampered block and everything after it" cost that makes real
blockchains tamper-resistant, just without the added ingredient of a live
network of peers who would reject the attacker's rewritten history in
favour of the longest honest chain (see Section 3 for that distinction).

## 2. Difficulty vs. effort experiment

**Setup.** I wrote a small standalone driver (not part of the shipped CLI)
that mines fresh, throwaway blocks directly via the `block` package's
`Mine` method, at difficulty 1 through 6, five trials per difficulty
level, and records the number of hash attempts and wall-clock time to
find a valid nonce.

**Results** (averaged over 5 trials per difficulty):

| Difficulty (leading hex zeros) | Avg. attempts | Avg. time (ms) |
|---:|---:|---:|
| 1 | 11 | 0.047 |
| 2 | 231 | 0.353 |
| 3 | 2,402 | 2.413 |
| 4 | 56,603 | 49.701 |
| 5 | 619,161 | 543.577 |
| 6 | 14,668,670 | 12,867.569 |

**Trend.** This is clearly **not linear — it grows roughly exponentially**,
by a factor of about 16x in expected attempts for each additional required
hex digit. That factor is exactly what theory predicts: each hex character
of a SHA-256 digest is effectively uniformly random over 16 possible
values (0–f), so requiring one more leading zero digit divides the
probability of any single attempt succeeding by 16, and therefore
multiplies the *expected* number of attempts needed by 16. Concretely,
requiring N leading hex zeros means an attempt succeeds with probability
1/16^N, so the expected number of attempts is 16^N — an exponential
function of the difficulty, not a polynomial or linear one. The measured
ratios between consecutive rows (21x, 10x, 24x, 11x, 24x) bounce around
that theoretical 16x because each row is only 5 trials of an inherently
random process (the geometric distribution has high variance), but the
overall shape across all 6 rows is unmistakably exponential rather than
linear — going from difficulty 1 to difficulty 6 multiplies the work by
roughly 1.3 million, not by 6.

This is the whole point of proof-of-work as a rate-limiting mechanism: a
small, linear increase in the *difficulty parameter* the network agrees on
translates into an exponential increase in the *computational cost* of
finding a valid block, which is what makes it expensive to rewrite history
or to out-mine the honest network.

## 3. Design write-up

**Hashing scheme.** Each block's hash is `SHA-256(JSON(payload))`, where
`payload` is an explicitly-ordered struct with six fields, in this exact
order: `height`, `timestamp`, `transactions` (each serialised as `sender`,
`recipient`, `amount` in that order), `prev_hash`, `nonce`, `difficulty`.
The block's own `hash` field is deliberately excluded — it is the *output*
of this computation, so including it would be circular. I chose
`encoding/json` over a hand-rolled byte concatenation because (a) it's
standard-library, (b) Go's JSON encoder serialises struct fields in
declaration order (never map order), which makes the output deterministic
without extra bookkeeping, and (c) it's easy to eyeball/debug by printing
the payload. The trade-off is that JSON is more verbose than a compact
binary encoding, so hashing is marginally slower than it could be — a
non-issue at this scale, and not something I'd optimise without a reason.

**How validation guarantees integrity across the whole chain.**
`Chain.Validate` walks blocks from genesis to tip and, for every block,
checks five things before moving on: (1) height is exactly one more than
the previous block's height, (2) timestamp is not earlier than the
previous block's, (3) `prev_hash` matches the previous block's *stored*
hash exactly, (4) recomputing the hash from the block's current fields
matches its *stored* hash, and (5) that stored hash satisfies the
difficulty target recorded for that block. It also replays every
transaction through a `ledger.Ledger` as it goes, so a chain that is
structurally perfect but contains an overspend is still rejected. Any
single failure returns immediately with the offending block's height and a
human-readable reason (`ValidationError`), satisfying the "identify the
first offending block" requirement.

The integrity guarantee comes from chaining checks (3) and (4) together:
because each block's hash is computed *over* its `prev_hash` field, and the
next block stores that hash as its own `prev_hash`, every block is
cryptographically bound to the one before it. Changing anything in block
*k* changes block *k*'s hash (check 4 catches this immediately), and even
if an attacker patches block *k*'s stored hash to hide that, block *k+1*'s
`prev_hash` now points to a hash that no longer exists on the (patched)
chain (check 3 would catch this) — unless the attacker also re-mines block
*k* to satisfy its difficulty target with the new content (check 5), and
then updates and re-mines every block after it too, since each one's
`prev_hash` needs to change to match. In other words, tampering with block
*k* is only "free" if you're willing to redo the proof-of-work for blocks
*k* through the tip, which is exactly the cost Section 2 measured.

## 4. Discussion questions

**How does the previous-hash link make tampering with an old block
impractical in a real chain, even though it is trivial in your local toy?**
In my local toy, "trivial" only in the sense that nothing stops me from
editing the JSON file directly — but Section 1's second experiment shows
that even locally, a tamper that survives the hash-recomputation check
still gets caught by the proof-of-work check, because faking a valid hash
for new content requires re-mining. The real difference in a production
chain isn't the difficulty of re-mining one block — it's that re-mining
one block isn't enough. In a live network, every peer independently holds
a copy of the chain and only accepts the *longest valid chain* it sees. To
rewrite an old block, an attacker has to re-mine that block *and every
block after it* fast enough to produce an alternative chain that is
longer/heavier than the one the rest of the honest network is still
extending in real time — a race against the combined hashing power of
every other participant, not just a one-off computation on a laptop. My
toy has no peers and no "longest chain wins" rule, so there's nothing
stopping a local edit from being accepted as-is by validation once it's
internally consistent; a real network's tamper-resistance comes from
needing to out-mine everyone else simultaneously, not merely from the hash
link itself.

**Proof-of-work is one way to decide who may add the next block. Name at
least one alternative, such as proof-of-stake or proof-of-authority, and
give one advantage and one drawback versus proof-of-work in your own
words.**
Proof-of-stake (used by, e.g., Ethereum since its 2022 "Merge") selects
the next block proposer based on how much cryptocurrency they've locked up
as stake, rather than how much computation they've burned. *Advantage over
proof-of-work:* it doesn't require racing enormous amounts of real-world
energy to search for nonces, so it's far cheaper to run and doesn't scale
its environmental footprint with network security. *Drawback versus
proof-of-work:* the cost of attacking the network is denominated in the
network's own token rather than in an external, physical resource
(electricity and hardware), which raises subtler questions — e.g. "nothing
at stake" style attacks, or the concern that wealth concentration in the
token itself can concentrate block-producing power, whereas mining
hardware and electricity are at least an independent, external cost that
doesn't circularly depend on the value of the chain being secured.

**List three concrete ways your toy differs from a production blockchain
(think about consensus among peers, transaction signatures, Merkle trees,
and finality). Pick one and sketch how you would add it.**
1. *No consensus among peers* — there is exactly one process, one copy of
   the chain, and no network protocol for peers to gossip blocks or agree
   on which chain is canonical.
2. *No transaction signatures* — anyone can submit a transaction claiming
   to be any sender; there's no cryptographic proof that the named sender
   authorized it.
3. *No Merkle tree* — a block's transactions are hashed as a flat,
   directly-embedded list, rather than summarised by a Merkle root that
   would let a light client verify a single transaction's inclusion
   without downloading the whole block.

**Sketch: adding digital signatures.** I'd extend `Transaction` with a
`PublicKey` field (identifying the sender) and a `Signature` field. When a
transaction is created, the caller would sign a canonical encoding of
`{sender, recipient, amount}` using Go's `crypto/ed25519` (chosen over
RSA/ECDSA for its simplicity and speed — no parameter choices to get
wrong, and it's in the standard library's `crypto` family via
`golang.org/x/crypto` or `crypto/ed25519` directly in modern Go). The
`Sender` field would then be derived from — or checked against — the
public key, so "sender" becomes "whoever holds this keypair" rather than a
free-text string. `Ledger.ValidateTransaction` would gain a new check
before the balance check: verify the signature against the public key and
the transaction's canonical bytes, rejecting the transaction outright if
it doesn't verify. This closes the current gap where the CLI happily lets
you type `addtx alice bob 30` on anyone's behalf — after this change, only
whoever controls alice's private key could produce a transaction alice's
balance would actually be debited for.

## Sources

General blockchain concepts referenced above (proof-of-work as a
difficulty-scaled search problem, the longest-valid-chain rule, Merkle
trees for compact transaction summaries) follow the design first described
in Satoshi Nakamoto's Bitcoin whitepaper, "Bitcoin: A Peer-to-Peer
Electronic Cash System" (2008). The description of Ethereum's move to
proof-of-stake follows publicly documented information about "The Merge"
from the Ethereum Foundation's own documentation and blog. No text from
either source is reproduced here; all explanations above are written in my
own words based on general, publicly available knowledge of how these
systems work.
