# Quorum KV

A key-value store where each node keeps a copy of the data. R is the number of replies needed to fetch a value; W is the number of acknowledgments needed to store it. Both default to a majority (3 out of 5 nodes).

## Implementation

[main.go](main.go) sets up the nodes and restores saved data. [protocol.go](protocol.go) handles reads and writes. Each value has a version `(counter, writer ID)`; the writer ID breaks ties between equal counters.

A write fetches R copies, chooses a newer version, then waits for W store acknowledgments. A read fetches R copies, picks the newest version, and writes it back to W nodes before returning. Data and counters are saved to disk. A timeout does not undo updates that already reached other nodes.

This implementation has fixed membership and no transactions across multiple keys. Changing W and R is an experiment control, not a safe way to reconfigure a running system.

## Inputs

- `Application.Put(key, value)`: write a value.
- `Application.Get(key, minCounter)`: read a value. Optional `minCounter` marks the result as stale if its counter is below this value.
- `Application.Mode(w, r)`: change this node's W and R.
- `Application.Read()`: inspect local state without fetching from other nodes.

## State fields

- `role`: always `replica`.
- `n`, `w`, `r`: node count and the local W/R settings.
- `values`: locally stored values and versions.
- `messagesSent`: requests sent to other nodes, excluding replies.
- `lastPut`, `lastGet`: latest results, including the value, counted responders, elapsed time (`waitedMs`), and success status. `lastGet.stale` compares the returned counter with `minCounter`.
- `lastError`: the most recent operation error.

## Experiments

With Docker running, start the platform with `npm start` from the repository root. In another terminal, also from the repository root:

```bash
DISTVIS_RUNTIME=docker DISTVIS_EVIDENCE_DIR=./evidence \
  node scripts/scenarios.mjs run quorum-kv \
  normal slow-replica crash partition variant-nonintersection
```

[scenarios.json](scenarios.json) contains the inputs, faults, and checks. The [report](report/report.md) covers normal operation, a slow node, a crash, a partition, and the W=R=1 variant, with screenshots and observations.

## Discussion questions

- [Why do we need to fetch version first before we write?](report/report.md#why-do-we-need-to-fetch-version-first-before-we-write)
- [Why do we write back what we read after reading?](report/report.md#why-do-we-write-back-what-we-read-after-reading)
- [What kind of consistency does quorum replication guarantee?](report/report.md#what-kind-of-consistency-does-quorum-replication-guarantee)



see the answers in my report