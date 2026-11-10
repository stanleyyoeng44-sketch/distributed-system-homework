# Lamport Mutual Exclusion

A decentralized mutual exclusion protocol. Each node broadcasts a timestamped Request, queues every request in timestamp order, and enters the critical section only when its own request is first and every other node has acknowledged it. A Release removes the request and lets deferred acknowledgments go.

## Implementation

[main.go](main.go) sets up the Lamport clock and services. [protocol.go](protocol.go) implements the request queue, acknowledgments, release, and application inputs. Requests are ordered by `(timestamp, numeric node ID)`, so an earlier timestamp wins and the node ID breaks ties.

`Acquire` increments the local Lamport clock, inserts its own request into the queue, and sends Request to every other node. A receiver merges the incoming timestamp into its clock, inserts the request in `(timestamp, node)` order, and replies Ack immediately unless it is in the critical section or its own waiting request has earlier priority; in those cases the Ack is deferred. The requester enters when it has an Ack from every peer and its request is at the queue head. It holds the section, then removes its request, flushes deferred Acks, and broadcasts Release. A received Request, Ack, or Release advances the clock past the sender's timestamp.

One acquisition uses `3(n−1)` messages in a normal n-node run: `n−1` Requests, `n−1` Acks, and `n−1` Releases. This is one round more than Ricart–Agrawala, which folds the release into the next reply. There is no coordinator, retry, failure detector, or persistence. A crashed node or blocked link can leave a requester waiting forever; healing does not resend the lost request or Ack. A delayed Release removes the first queued request from that node, because Release identifies the sender rather than a specific request.

## Inputs

- `Application.Acquire(holdMs)`: request the critical section, hold it, release, and return. Optional `holdMs` defaults to 2000 ms when omitted or zero; the valid range is (0, 120000].
- `Application.Auto(count)`: run repeated acquisitions in the background, holding each for 1000 ms with a 300 ms pause between cycles. Optional `count` defaults to 5 when omitted or zero.
- `Application.Read()`: inspect local state without requesting the section.

`Acquire` returns only after the section has been released; a hold longer than the platform's ten-second call deadline can make the call time out even though the section finishes on the node. `Auto` returns after starting the background sequence; watch `entries` and `role`. A second local acquisition is rejected while this node is waiting or critical, so a manual `Acquire` and an `Auto` sequence can stop each other when they overlap.

## State fields

- `role`: `idle`, `waiting`, or `critical`.
- `clock`: current Lamport counter.
- `myTS`: timestamp of the current request; zero when idle.
- `acksHave`, `acksNeed`: Acks received for this request and the number required (`n−1`).
- `queue`: requests in `(timestamp, node)` order, including the local request.
- `deferred`: nodes whose Acks this node is withholding.
- `entries`, `lastWaitMs`: critical-section entries and the wait before the latest entry.
- `messagesSent`: outgoing Request, Ack, and Release attempts, including failed calls; replies are excluded.
- `lastError`: reserved; a missing peer shows up as an unfinished wait, not as an error.

## Experiments

With Docker running, start the platform with `npm start` from the repository root, create a three-node Docker experiment, and run the inputs from the node panel. These runs were created manually rather than from a `scenarios.json`. The [report](report/report.md) contains the observations and screenshots.

- Contention: run `Acquire(holdMs=2000)` on `node-2` and `node-3` at the same time. Both requests get timestamp 1, `node-2` wins the tie, `node-3` waits about 2.2 s, and the critical sections do not overlap.
- Crash: crash `node-3`, then run `Acquire` on `node-2`. `node-2` receives `node-1`'s Ack but waits forever with `acksHave=1` of 2.
- Partition: block both directions between `node-1` and `node-3`, run `Acquire` on `node-1`; `node-3`'s Request is dropped, `node-1` waits with `acksHave=1` of 2, the call times out, and healing the link does not replay the Request or Ack.
- Design variant (message count): one acquisition in a three-node group produces 6 algorithm messages, compared with 4 for Ricart–Agrawala.

Saved exports: [contention](report/lamportmutex_scenA.json), [crash](report/lamportmutex_scenB.json), [partition](report/lamport_mutex_scenC.json).

## Discussion questions

- [Why do the queue and all Acks guarantee mutual exclusion?](report/report.md#1-why-do-the-queue-and-all-acks-guarantee-mutual-exclusion)
- [Why defer an Ack while waiting?](report/report.md#2-why-defer-an-ack-while-waiting)
- [Why does one crashed node block everyone?](report/report.md#3-why-does-one-crashed-node-block-everyone)
- [Why does Lamport need more messages than Ricart-Agrawala?](report/report.md#4-why-does-lamport-need-more-messages-than-ricart-agrawala)
