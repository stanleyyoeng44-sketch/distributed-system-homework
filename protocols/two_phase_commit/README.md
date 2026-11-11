# Two-Phase Commit

A bank-transfer protocol with one coordinator (the transaction manager, `node-1`) and one resource manager per other node. Every RM starts with a balance of 100. The TM prepares every RM, commits only if all vote yes, writes the decision to its log before notifying, and resends that decision after a crash.

## Implementation

[main.go](main.go) registers the TM or RM service, loads saved state, and starts recovery. [protocol.go](protocol.go) implements Prepare/vote, the durable decision, notification, participant status polling, and the application inputs. Membership is fixed and the TM is always `node-1`.

A transfer assigns `T1`, `T2`, ... and persists the preparing transaction before sending Prepare to every RM with its signed delta. An RM votes no if a debit would make its balance negative; otherwise it saves a prepared record before voting yes and starts polling `TM.Status` every 2 s. The TM commits only if every vote is yes; any no or missing vote selects abort. It persists the decision before sending Commit or Abort, then resends to any RM that has not acknowledged, every 2 s, until all have. A committed RM applies its delta once and saves the new balance before acknowledging.

If the TM crashes before writing a decision, recovery writes abort (presumed abort). If it crashes after writing commit, recovery resends commit until every RM acknowledges. A prepared RM that cannot reach the TM becomes `uncertain` and keeps its lock. A crash before a vote is saved is unsafe: the design variant removes that save and shows a recovered RM forgetting a yes vote that the TM already used to commit. There is no coordinator election, no three-phase commit, and only one transfer at a time.

## Inputs

- `Application.Transfer(from, to, amount, pauseBeforeDecisionMs, pauseAfterDecisionMs)`: start a transfer on the TM. `amount` is 1–1000000; pauses are 0–60000 ms. `from` and `to` must be different RMs. At least two RMs are required.
- `Application.Read()`: inspect local TM or RM state.

`Transfer` returns after the decision has been delivered to every RM. A long pause or a crashed participant can keep the call open beyond the platform's ten-second deadline; the transaction continues on the nodes and can be observed through `Read` and the state reports. If the TM itself crashes, recovery resumes the transaction from its log.

## State fields

- `role`: `TM` or `RM`.
- TM `txn`, `from`, `to`, `amount`: current transaction and transfer.
- TM `phase`: empty before the first transfer, then `preparing`, `decided`, or `finished`.
- TM `decision`, `reason`: `commit` or `abort` and why.
- TM `votes`, `acks`: per-RM vote and decision acknowledgment.
- RM `balance`, `delta`: current balance and pending/recent change.
- RM `vote`, `state`: yes/no and `idle`, `prepared`, `uncertain`, `committed`, or `aborted`.
- `lastError`: latest storage or protocol error.

## Experiments

With Docker running, start the platform with `npm start` from the repository root, create the Docker experiment described in each scenario, and run the inputs from the node panel. These runs were created manually rather than from a `scenarios.json`. The [report](report/report.md) contains the observations and screenshots.

- Normal commit: three nodes, two transfers (30 from `node-2` to `node-3`, then 20 back). The balances go 70/130 then 90/110; the total stays 200, and the TM and both RMs report commit for both transactions.
- Veto: transfer 150 from `node-2`. `node-2` votes no for insufficient funds while `node-3` has already locked a +150 delta; the TM aborts and releases the lock, and both stay at 100.
- Crash before decision: transfer 100 with `pauseBeforeDecisionMs=30000`, then crash `node-1` while the RMs are prepared. They become uncertain and stay locked; recovery finds no decision and aborts.
- Crash after decision: transfer 100 with `pauseAfterDecisionMs=30000`, then crash `node-1` after the commit log write. Recovery resends commit and each balance changes exactly once.
- Partition during commit: transfer 100 with `pauseBeforeDecisionMs=30000`; wait for both RMs to be prepared, then block `node-1` ↔ `node-3`. The TM commits and node-2 applies, but node-3 stays `uncertain` with balance 100 while the TM retries the missing Commit; the Transfer call hits the 10 s platform deadline. Heal the link; the retry reaches node-3, balances become 0/200, and the TM finishes.
- Design variant: transfer 100 with `pauseAfterDecisionMs=30000` after removing the save in `RM.Prepare`; a yes-voting RM that crashes loses its prepared record while the TM commits and the other RM applies. node-2 stays at 100, node-3 reaches 200, and the transaction never finishes.

Saved exports: [normal](report/2pc_scenA.json), [veto](report/2pc_scenB.json), [crash before decision](report/2pc_scenC.json), [crash after decision](report/2pc_scenD.json), [partition](report/2pc_networkfault.json), [design variant](report/2pc_designVariant.json).

## Discussion questions

- [Why is a majority not enough?](report/report.md#1-why-is-a-majority-not-enough)
- [Why must a prepared RM stay locked?](report/report.md#2-why-must-a-prepared-rm-stay-locked)
- [Why save the prepared record before voting yes?](report/report.md#3-why-save-the-prepared-record-before-voting-yes)
- [Why is the log write the commit point?](report/report.md#4-why-is-the-log-write-the-commit-point)
- [Why is resending Commit safe?](report/report.md#5-why-is-resending-commit-safe)
