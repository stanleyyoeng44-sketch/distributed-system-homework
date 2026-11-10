# Paxos KV

A key-value store where nodes agree on an ordered log of operations. Each slot (position in the log) runs its own Paxos round. Both Put and Get take a slot.

## Implementation

[main.go](main.go) sets up the nodes and restores saved state. [protocol.go](protocol.go) handles proposals, voting, and execution. Every node can propose operations, accept proposals, and learn decisions; there is no elected leader.

A proposer sends Prepare and waits for a majority (3 out of 5 nodes). It reuses the operation with the highest accepted ballot (proposal number) in those replies, or its own operation if none was accepted. It then sends Accept, waits for a majority, and announces the decision. If another operation wins the slot, it tries its own operation in the next slot. Put/Get return successfully only after their operation has been executed locally in log order.

Promises, accepted operations, decisions, and execution state are saved to disk. A recovered node uses later Prepare messages to discover missing decisions and recover them through Paxos. It can recover several slots at once, but still executes them in order. 


## Inputs

- `Application.Put(key, value, pauseMs)`: propose a write. Optional `pauseMs` (0–1000) pauses after choosing the slot, making competing writes easier to demonstrate.
- `Application.Get(key)`: read through the log, after earlier operations.
- `Application.Read()`: inspect local state without proposing an operation.
- `Application.Amnesia(enabled)`: turn saving off or on for this node. Set it before any slots are recorded; disabling saving is only for the unsafe experiment.

## State fields

- `role`: `proposer-acceptor-learner`.
- `phase`: latest local activity: idle, prepare, accept, completed, or failed.
- `log`: each slot's promised ballot, accepted ballot, operation, and `decided` flag (whether this node knows the decision).
- `applied`: highest slot executed, with no earlier slot missing.
- `values`: current key-value contents after execution.
- `lastOperation`: latest application result, including its ID, slot, returned value, whether the key was found, and any error.
- `conflicts`: slots where another operation won instead of the local request.
- `messagesSent`: requests sent to other nodes, excluding replies.
- `amnesia`: whether this process is skipping saves.
- `storageError`: a save failure that stops further successful promises and votes.

## Experiments

With Docker running, start the platform with `npm start` from the repository root. In another terminal, also from the repository root:

```bash
DISTVIS_RUNTIME=docker DISTVIS_EVIDENCE_DIR=./evidence \
  node scripts/scenarios.mjs run paxos-kv \
  normal competing-writes crash-catchup partition variant-amnesia persistence-control
```

[scenarios.json](scenarios.json) contains the inputs, faults, and checks. The [report](report/report.md) describes normal operation, competing writes, crash recovery, a partition, and the amnesia variant. `persistence-control` repeats the variant with saving enabled to check that slot 1 keeps its original operation.

## Discussion questions

- [Why does Get occupy a space in the log?](report/report.md#1-why-does-get-occupy-a-space-in-log)
- [What is the main trick of Paxos?](report/report.md#2-what-is-the-main-trick-of-paxos)
- [What happens after Paxos?](report/report.md#3-what-happens-after-paxos)
- [Why is Prepare needed, and why promise not to accept lower ballots?](report/report.md#4-why-is-prepare-needed-why-do-we-need-them-to-promise-not-to-accept-proposals-with-a-lower-slot)
- [Must older Prepare requests be rejected, and must value selection use only positive replies?](report/report.md#5-must-older-prepare-requests-be-rejected-and-must-value-selection-use-only-positive-replies)
- [Why do we check in the Accept stage whether the slot has been decided?](report/report.md#6-why-do-we-check-in-accept-stage-whether-the-slot-has-been-decided)
