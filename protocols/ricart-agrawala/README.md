# Ricart–Agrawala

A mutual exclusion protocol (at most one node executes a critical section at a time). A critical section is work that must not overlap with another node's. Each node needs permission from every other node before entering, not just a majority.

## Implementation

[main.go](main.go) sets up the nodes and their logical clocks (counters used to order events). [protocol.go](protocol.go) handles requests, permissions, and entry into the critical section. Each request has a fixed priority `(timestamp, node ID)`; earlier timestamps win, with numeric node IDs breaking ties.

A node sends one Request to every other node. Each reply is the permission itself, not an acknowledgment that the request arrived. A node delays its reply while it is in the critical section, or while its own waiting request has earlier priority. On leaving, it lets those delayed replies return. There are no separate Ack or Release calls: one successful acquisition uses `2(n−1)` messages across n nodes, counting requests and replies.

Membership is fixed, and state is kept in memory, not restored after a restart. A failed call never counts as permission. There is no retry or cancellation of a waiting acquisition, so a crash, blocked link, or reply exceeding the platform's ten-second call deadline can leave a node waiting indefinitely. Healing a link does not resend a failed request.

## Inputs

- `Application.Acquire(holdMs)`: start one acquisition, hold the critical section, then leave automatically. Optional `holdMs` (1–120000) defaults to 2000 when omitted or zero.
- `Application.Auto(count)`: run repeated acquisitions, holding each for one second with a random pause between them. Optional `count` (1–100) defaults to 5 when omitted or zero.
- `Application.Read()`: inspect local state without requesting permission from other nodes.

Acquire and Auto return after starting work in the background, not after entering the critical section. Watch `role`, `entries`, and `completed` for progress. A second local acquisition is rejected while an acquisition or Auto sequence is running.

## State fields

- `role`: `idle`, `waiting`, or `critical`.
- `clock`: current logical clock; receiving a message advances it past both the local and received counters.
- `myTS`: fixed timestamp of the current acquisition, cleared on completion.
- `acksHave`, `acksNeed`: permissions received and required; `acksNeed` is the number of other nodes.
- `missingAcks`: nodes whose permission is still missing while waiting.
- `deferred`: nodes whose Request replies this node is delaying.
- `entries`, `completed`: number of critical-section entries and finished acquisitions.
- `lastWaitMs`: time spent waiting before the latest entry, in milliseconds.
- `messagesSent`: outgoing requests and permission replies prepared for sending, not proof of delivery.
- `sendErrors`, `lastError`: failed permission calls and the most recent operation error.
- `operation`: latest local activity: none, waiting, critical, completed, or stopped.
- `operationRunning`, `autoRunning`: whether an acquisition or an Auto sequence is still running.

## Experiments

With Docker running, start the platform with `npm start` from the repository root. In another terminal, also from the repository root:

```bash
DISTVIS_RUNTIME=docker DISTVIS_EVIDENCE_DIR=./evidence \
  node scripts/scenarios.mjs run ricart-agrawala \
  normal-contention crash partition jitter-fifo jitter-unordered \
  long-hold-timeout variant-message-count
```

[scenarios.json](scenarios.json) contains the inputs, faults, and checks. The [report](report/report.md) covers simultaneous requests, a crash, reordered messages, a blocked link, and a comparison with Lamport mutual exclusion. `long-hold-timeout` checks that a late permission cannot unblock a timed-out call. `variant-message-count` checks the Ricart–Agrawala message count; it does not run a Lamport implementation.

## Discussion questions

- [Why is mutual exclusion guaranteed for out-of-order messages?](report/report.md#why-is-mutual-exclusion-guaranteed-for-out-of-order-messages)
