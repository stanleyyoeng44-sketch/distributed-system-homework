# Primary-Backup Replication

A replicated key-value store with one fixed primary (`node-1`) and backups on every other node. Writes are applied at the primary and copied to backups; reads can be local, from the primary, or session-ordered. There is no automatic failover.

## Implementation

[main.go](main.go) registers the replica and application services. [protocol.go](protocol.go) handles writes, replication, reads, and mode changes. The primary assigns each write the next log `index`, applies it locally, and persists the log and store before replying or replicating.

A write sent to a backup is forwarded to the primary. In `sync` mode the primary sends Apply to every backup and waits up to 4 s for all acknowledgments; the write returns an error if any backup is missing, even though the primary and some backups may already have applied it. In `async` mode the primary replies immediately (`acks` is 0) and replicates after `replicationDelayMs`; a later `lastWrite` update records the acknowledgments. A backup buffers an entry that arrives with a gap and applies it once the missing indexes arrive. A sync acknowledgment means the backup accepted the Apply; a backup that is missing an earlier index can acknowledge a later entry while buffering it, so `acks` does not prove that every backup applied it.

Read modes: `local` reads this node's store (a backup marks the result `stale`), `primary` forwards to `node-1`, and `session` waits on a backup until `appliedIndex >= minIndex` (up to 9 s), which gives read-your-writes. Data, indexes, log, and replication mode are saved to disk. A recovered backup keeps its applied index, but the primary does not repair gaps, so a backup that missed entries can remain behind. If the primary crashes, a write or primary read sent to a backup fails after the 10 s forwarding timeout; local reads still work. There is no election or failover.

## Inputs

- `Application.Put(key, value)`: write through the primary. Any node accepts the call; a backup forwards it.
- `Application.Get(key, mode, minIndex)`: read with `mode` = `local` (default), `primary`, or `session`. `minIndex` is used by session reads and defaults to 0.
- `Application.Mode(replication, replicationDelayMs)`: set `async` or `sync` and the async delay; a backup forwards this to the primary, which mirrors it to backups.

There is no `Read` input; state is published after each operation. A `sync` write that times out may still have applied on the primary and some backups.

## State fields

- `role`: `primary` or `backup`; `primary` is always `node-1`.
- `replication`, `replicationDelayMs`: current policy, mirrored from the primary.
- `index`, `appliedIndex`: last assigned log index and highest contiguous applied index.
- `store`, `log`: local key-value contents and applied entries.
- `pending`: entries buffered because an earlier index is missing.
- `lastWrite`: key, value, index, replication mode, acknowledgments, and any error.
- `lastGet`: key, value, found, mode, index, `stale`, and `waitedMs`.
- `lastError`: latest replication or write-forwarding error; read failures are reflected in `lastGet`.

## Experiments

With Docker running, start the platform with `npm start` from the repository root, create a five-node Docker experiment, and run the inputs from the node panel. These runs were created manually rather than from a `scenarios.json`. The [report](report/report.md) contains the observations.

- Async stale reads: run `Mode(async, replicationDelayMs=4000)`, then `Put(x,1)` on `node-2`. The write returns immediately with `acks:0`; an immediate local read on `node-2` returns `found:false, stale:true`; a primary read returns 1; a session read with `minIndex=1` waits until the delayed update arrives (about 3 s after the read starts, about 4 s after the write).
- Sync and a backup crash: run `Mode(sync)` and write `x=1` (4 acks); crash `node-3`, then write `x=2`. The write fails after 4 s with a missing ack, but `x=2` is on the primary and the live backups.
- Partition and no log repair: run `Mode(sync)` and write `x=1`; block `node-1` ↔ `node-3`, then write `x=2`. The write fails after 4 s with a missing ack from `node-3`, but `x=2` is on the primary and the live backups; a local read on `node-3` returns `x=1, stale:true`. Heal the link and write `x=3`: the write returns `acks:4`, but `node-3` has `index=3`, `appliedIndex=1`, `pending=1`, and still reads `x=1`; a session read with `minIndex=3` waits 9 s and times out with `lastGet.stale=true`.
- Design variant: crash `node-1`. Writes and primary reads sent to `node-2` wait 10 s and fail; only local reads work, and no backup takes over.

Saved exports: [async](report/primary_back_scenA.json), [sync and crash](report/primary_back_scenB.json), [partition](report/primary_backup_scenC.json), [design variant](report/primary_back_designvariant.json).

## Discussion questions

- [Why can an async write be invisible to a local read?](report/report.md#1-why-can-an-async-write-be-invisible-to-a-local-read)
- [Why does a sync timeout not mean the write failed?](report/report.md#2-why-does-a-sync-timeout-not-mean-the-write-failed)
- [What do local, primary, and session reads guarantee?](report/report.md#3-what-do-local-primary-and-session-reads-guarantee)
- [Why is a separate primary election needed?](report/report.md#4-why-is-a-separate-primary-election-needed)
