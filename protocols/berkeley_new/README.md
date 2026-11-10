# Berkeley Clock Synchronization

A clock synchronization protocol where a fixed coordinator polls the other nodes, discards clocks far from the median, averages the remaining offsets, and tells every reachable node how far to adjust. The goal is for the group to agree with each other, not with real time.

## Implementation

[main.go](main.go) creates the software clock and registers the services. [protocol.go](protocol.go) runs the polling, filtering, and adjustment phases. `node-1` is the coordinator; membership is fixed. Each node starts with an offset of 750 ms times its position in `members`, so a three-node run begins at 0, +750, and +1500 ms.

`Application.Sync` runs one round on the coordinator. It polls every peer concurrently with a 3 s call deadline and records `offset = peerTime + RTT/2 - localTimeAtReceipt`; the coordinator adds itself with offset zero. It takes the median, keeps samples within `outlierThresholdMS` of the median, and averages them. Every responsive node, including an excluded outlier, receives `delta = mean - itsOffset`. At least two responsive clocks must be within the threshold of the median; otherwise the round refuses to adjust.

Corrections are applied by slewing by default: the clock changes its rate by at most 10% of elapsed time and never jumps backward. `SetMode(step=true)` switches to an immediate jump for comparison. A node rejects an adjustment if the coordinator instance, clock generation, or round number is stale, or if its previous correction is still pending. Poll and Sync also reject a clock that is still slewing. State is in memory; a restart creates a new clock. The coordinator is fixed, so a crashed `node-1` cannot be replaced.

## Inputs

- `Application.Sync(outlierThresholdMS)`: run one round; coordinator only. Optional `outlierThresholdMS` defaults to 2000 ms when omitted or zero; the valid range is (0, 120000].
- `Application.ConfigureClock(offsetMS, driftPPM)`: reset this node's software clock relative to the host clock and invalidate older measurements. Offset must be within ±60000 ms and drift within ±10000 ppm.
- `Application.SetMode(step)`: choose immediate jump (`true`) or smooth slew (`false`, default) for future corrections.
- `Application.Read()`: inspect local state without changing the clock.

`Sync` waits for the round to finish. If a peer is unreachable, the responsive nodes can still be corrected, but the round ends in `partial_failure` and the call reports an error; if too few clocks remain, it stops before sending corrections.

## State fields

- `role`: `coordinator` or `participant`.
- `coordinator`: fixed coordinator ID (`node-1`).
- `clock`: current software time, host-relative offset (`hostOffsetMS`), drift, and remaining slew (`slewMsLeft`).
- `busy`, `stepMode`: whether a round is running and whether corrections jump or slew.
- `phase`: latest round phase: idle, polled, computed, sending, sent, or partial_failure.
- `round`: current/latest round number, threshold, median, mean, and per-sample node, RTT, offset, generation, included, delta, scheduled, and error.
- `lastAppliedRound`, `lastAdjMs`: latest accepted adjustment round and its size.
- `lastError`: latest round or validation error.

## Experiments

With Docker running, start the platform with `npm start` from the repository root, create a three-node Docker experiment, and use the node panel to run these inputs. These runs were created manually rather than from a `scenarios.json`. The [report](report/report.md) contains the observations and screenshots.

- Convergence: leave the default offsets 0/+750/+1500 ms, run `Sync` on `node-1`, then `Read` each node after the slews finish; all should settle near +750 ms.
- Outlier: set `node-3` to +10000 ms with `ConfigureClock`, run `Sync` again; `node-3` is excluded from the mean (about +375 ms) but still receives a large negative correction.
- Jump versus slew: run the same round with `SetMode(false)` and `SetMode(true)`; the jump reaches the target immediately but can move time backward.
- Design variant: crash `node-1`; `Sync` from another node is rejected and no failover occurs.

Saved exports: [slew](report/berkeley_scenarioCslew.json), [jump](report/berkeley_scenarioCjump.json), [design variant](report/berkeley_designvariant.json).

## Discussion questions

- [Why converge to the group average instead of real time?](report/report.md#1-why-converge-to-the-group-average-instead-of-real-time)
- [Why exclude an outlier but still correct it?](report/report.md#2-why-exclude-an-outlier-but-still-correct-it)
- [Why is a clock jump unsafe?](report/report.md#3-why-is-a-clock-jump-unsafe)
- [What happens when the fixed coordinator crashes?](report/report.md#4-what-happens-when-the-fixed-coordinator-crashes)
