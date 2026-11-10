# Lamport and Vector Clocks

A demonstration where nodes timestamp the same local, send, and receive events with Lamport clocks [single counters], vector clocks [one counter per node], and total-order timestamps [numbers that break Lamport ties]. These describe event order rather than physical time.

## Implementation

[main.go](main.go) starts the nodes. [protocol.go](protocol.go) restores saved state and handles events and messages. [clocks.go](clocks.go) implements clock updates and vector comparison. [controls.go](controls.go) handles automatic events, comparisons, and the design variant.

A local event or send increments the Lamport counter and the node's own vector entry. A receive first merges the incoming clocks [takes the larger Lamport counter and the larger value at each vector entry], then increments both. The total-order timestamp is `N * L + i`, where `N` is the number of nodes, `L` is the Lamport counter, and `i` is the node's position in `members`, starting from zero.

Clocks and recent events are saved to disk before sending a message or confirming a receive. A recovered node restores its counters instead of starting from zero. Reads, comparisons, and delivery confirmations do not advance the clocks.

## Inputs

- `Application.Local(label)`: record one local event.
- `Application.Send(to, label, receiveLabel, payload)`: record a send and attempt delivery. The two labels name the send and receive events; `payload` is optional message text. Check `lastDelivery.confirmed` for delivery status.
- `Application.Read()`: inspect local state without advancing the clocks.
- `Application.Compare(left, right)`: compare two vector arrays in `members` order. Returns `happens-before`, `happens-after`, `concurrent`, or `equal`.
- `Application.CompareEvents(left, right)`: compare two timestamps, each containing `lamport`, `vector`, and `total`. Saves the result in `lastCompare`.
- `Application.Auto(count)`: generate random local events and sends. `count` is 1–20; omitted or zero means 8.
- `Application.Variant(omitReceiveMerge)`: turn the incorrect receive rule [ignore incoming clocks] off or on for this node. Set it before any events are recorded.

## State fields

- `role`: `clock`.
- `node`: this node's ID.
- `members`: node IDs in vector-entry order.
- `clock`: current `lamport`, `vector`, and `total` values.
- `events`: the most recent 64 events, including their IDs, labels, kinds, and timestamps. Matching send and receive events share a `messageID`.
- `messagesSent`: send attempts since this process started, not confirmed deliveries.
- `lastDelivery`: latest completed send attempt's message ID, destination, confirmation status, and any error.
- `autoRunning`: whether automatic event generation is running.
- `lastCompare`: latest vector relation and numeric Lamport and total orders. `agrees` checks that a strict vector order has the same Lamport order; concurrent or equal vectors also return true.
- `storageError`: a save failure that stops further event recording.

## Experiments

With Docker running, start the platform with `npm start` from the repository root. In another terminal, also from the repository root:

```bash
DISTVIS_RUNTIME=docker DISTVIS_EVIDENCE_DIR=./evidence \
  node scripts/scenarios.mjs run logical-clocks \
  normal-six-events equal-concurrent auto-invariant crash-recovery partition variant-no-merge
```

[scenarios.json](scenarios.json) contains the inputs, faults, and checks. The [report](report/report.md) describes the six-event example, concurrent events [neither causally precedes the other] with equal Lamport timestamps, random events, crash recovery, and a blocked connection. `variant-no-merge` disables the receive merge to demonstrate incorrect timestamps.

## Discussion questions

- [What does a successful Send mean if delivery is not confirmed?](report/report.md#1-what-does-a-successful-send-mean-if-delivery-is-not-confirmed)
- [Does the index order in total-ordered timestamp have any meaning?](report/report.md#2-does-the-index-order-in-total-ordered-timestamp-have-any-meaning)
