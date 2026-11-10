# Cristian Clock Synchronization

A clock synchronization algorithm where each client asks a time server for its clock reading, Ts. The client measures RTT (the time from sending the request to receiving the reply) and sets its clock to `Ts + RTT/2`. By default, `node-1` is the time server and starts with an accurate clock.

## Implementation

[main.go](main.go) sets up the nodes and reports their state every two seconds. [protocol.go](protocol.go) handles time requests and synchronization. [clock.go](clock.go) gives each node a simulated clock (a separate clock reading without changing the computer's clock), with an offset (how far ahead or behind it starts) and drift (how much faster or slower it runs).

A client records its clock before sending `TimeService.Ask` and after receiving the reply. The server returns Ts, read after any configured server delay. The client calculates RTT from its two readings, estimates the current time as `Ts + RTT/2`, and applies the difference as a step (an immediate clock change, which may move time backwards). A failed request leaves the clock unchanged.

This implementation has no automatic synchronization, retry, or replacement server, and clocks are not saved across restarts. Synchronization corrects the clock reading, not its drift. The `RTT/2` error bound assumes an accurate server and a client measuring elapsed time at the correct rate; it is not an unconditional guarantee when clocks drift or another, inaccurate node is chosen as the server.

## Inputs

- `Application.Sync(server, mode)`: synchronize once. An empty `server` selects `node-1`; `mode` defaults to `half` (`Ts + RTT/2`). The alternatives are `zero` (Ts alone) and `full` (`Ts + RTT`).
- `Application.Status()`: refresh and return this node's state.
- `Application.SetClock(offsetMs, driftPPM)`: reset the clock to real time plus `offsetMs` milliseconds, with the chosen drift. `driftPPM` means parts per million; +1000 makes the clock gain 1 millisecond per second.
- `Application.SetServerDelay(delayMs)`: make this node wait 0–8000 milliseconds before reading its clock to answer a time request.

## State fields

- `role`: `server` for `node-1`, otherwise `client`. Any node can answer time requests if explicitly selected.
- `clock`, `clockMs`: the local clock reading as UTC text (hours, minutes, and seconds in Coordinated Universal Time) and Unix milliseconds (milliseconds since 1 January 1970 UTC).
- `errorMs`: clock minus real time; positive means ahead. This is shown for observation and is not used by the algorithm.
- `driftPPM`: the clock's speed error in parts per million.
- `serverDelayMs`, `served`: the configured delay before answering a time request and the number of requests answered.
- `syncs`, `failedSyncs`, `busy`: successful synchronizations, failed time requests, and whether this node is waiting for a synchronization reply.
- `lastSync`: the latest successful synchronization, including the selected server and mode, send/server/receive readings (`t0Ms`, `tsMs`, `t1Ms`), `rttMs`, assumed reply delay (`assumedOneWayMs`), estimated time (`estimateMs`), clock adjustment (`correctionMs`), `errorBoundMs`, and errors before and after the adjustment (`errorBeforeMs`, `errorAfterMs`).
- `lastError`: the most recent synchronization request error; cleared after a successful synchronization.

## Experiments

With Docker running, start the platform with `npm start` from the repository root. In another terminal, also from the repository root:

```bash
DISTVIS_RUNTIME=docker DISTVIS_EVIDENCE_DIR=./evidence \
  node scripts/scenarios.mjs run christian \
  e-variant
```

[scenarios.json](scenarios.json) currently contains the inputs and checks for the `zero`, `full`, and `half` design variant. The [report](report/report.md) covers normal operation, unequal request and reply delays, server waiting time, and the design variant, with screenshots and observations.

## Discussion questions

- [Cristian's algorithm assumes the round-trip delay is symmetric; how large is the error when it is not?](report/report.md#cristians-algorithm-assumes-the-round-trip-delay-is-symmetric-how-large-is-the-error-when-it-is-not)
- [Why add RTT/2 rather than 0 or RTT?](report/report.md#design-variant)

see the answers in my [report](report/report.md)
