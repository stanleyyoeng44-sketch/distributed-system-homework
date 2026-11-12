# Bully Election

An election protocol that chooses the live node with the highest numeric ID as coordinator. Any node can start a round manually; nothing is sent at startup.

## Implementation

[main.go](main.go) sets up the node priority and services. [protocol.go](protocol.go) implements Election, OK replies, Coordinator announcements, and the application inputs. The numeric suffix of the node ID is the priority, so `node-10` outranks `node-2`.

`StartElection` makes the caller a candidate and sends Election to every higher-ID member concurrently, with a 3 s call deadline. A higher node replies OK and, unless it is already a candidate or coordinator, starts its own campaign; a coordinator re-announces itself. If at least one OK arrives, the caller becomes a follower and leaves the round. If no higher node answers, the caller becomes coordinator and broadcasts Coordinator to every member. A candidate that receives a higher Coordinator announcement stands down. A lower announcement is ignored if it is stale; if it comes from a lower node with a newer term, the receiver starts a new election, or re-announces itself if it is the coordinator.

Failed calls are treated as dead nodes. There is no automatic failure detector, heartbeat, or startup election: a crash is noticed only when someone starts a new round. A recovered node starts as a follower with no coordinator. Membership is fixed, and election state is in memory. A partition can leave two coordinators; a later election or Coordinator exchange reconciles them only if messages flow again.

Election and Coordinator messages carry a term. A node ignores a coordinator from an older term, or a lower-priority coordinator from the same term, so a delayed old announcement cannot replace a newer one. The term does not prevent two coordinators during a partition; it only rejects stale announcements after a newer election has run.

## Inputs

- `Application.StartElection()`: start one election round from this node and return when its campaign finishes, or immediately if it is already a candidate.
- `Application.Read()`: inspect local state without starting an election.

## State fields

- `role`: `follower`, `candidate`, or `coordinator`.
- `id`: numeric election priority parsed from the node ID.
- `coordinator`: locally recognized coordinator, empty before the first election.
- `term`, `coordinatorTerm`: latest election term seen and the term of the current coordinator.
- `electionRound`: local generation used to discard an obsolete campaign; not a global term.
- `electionsWon`, `backedDown`: local victories and withdrawals.
- `sent`: Election and Coordinator calls attempted, including failed calls; replies are excluded.
- `lastError`: reserved; failed calls are treated as absent nodes.

## Experiments

With Docker running, start the platform with `npm start` from the repository root, create the Docker experiment described in each scenario, and run `StartElection` from the node panel. These runs were created manually rather than from a `scenarios.json`. The [report](report/report.md) contains the observations and screenshots.

- Five nodes, start an election on `node-1`: `node-5` wins and the others become followers.
- Coordinator crash: crash `node-5`, then start an election on `node-1`; `node-4` waits for `node-5` to time out and wins.
- Eight nodes: elect `node-8`, crash it, then start an election on `node-4`; `node-7` wins.
- Two more crashes: with `node-8`, `node-7`, and `node-6` down, an election from a lower node makes `node-5` the winner.
- Partition: block every link between `{node-1,node-2,node-3}` and `{node-4,node-5}`, then start an election on `node-3`; node-3 becomes coordinator on its side while node-5 remains coordinator on the other, so two coordinators coexist. Heal the links and start an election on `node-5`; it re-announces itself and all nodes converge on node-5.
- Design variant: while `node-6` is coordinator after `node-7` crashed, recover `node-7` and start an election; `node-7` takes over again and `node-6` steps down.

Saved exports: [five nodes](report/bully_scenA.json), [coordinator crash](report/bully_scenB.json), [eight nodes](report/bully_scenc.json), [partition](report/bully_networkfault.json), [design variant](report/bully_designvariant.json).

## Discussion questions

- [Why does the highest live node win?](report/report.md#1-why-does-the-highest-live-node-win)
- [Why can an election cost quadratic messages?](report/report.md#2-why-can-an-election-cost-quadratic-messages)
- [Can a timeout prove that a node crashed?](report/report.md#3-can-a-timeout-prove-that-a-node-crashed)
- [Why does a recovered highest node take over?](report/report.md#4-why-does-a-recovered-highest-node-take-over)
