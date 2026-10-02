# Batching message-save statements

## Measurement and change

Measured on 2026-10-01 with Go 1.27.1, macOS/arm64, Apple M1, PostgreSQL 18.1
(aarch64, Docker), READ COMMITTED, `fsync=on`, and `synchronous_commit=on`.
The production baseline is `fa4c523`, following the
[single-transaction optimization](message-save.md). Benchstat is from
`golang.org/x/perf` revision `406019bb8b68`.

Initial benchmarks measured serial saves around 2 ms and parsing around 0.2 µs.
Under delivery load, the baseline CPU profile attributes 90.7% of samples to
`runtime.kevent`, `syscall.rawsyscalln`, and pthread wait/signal operations.
The healthy-delivery goroutine snapshot has all eight writers waiting on
PostgreSQL reads. This points to database I/O as the optimization target.

`messageStore.Save` now sends the membership insert, message insert, and optional
sender lookup through one `pgx.Batch`, in statement order. `Close` consumes the
results and propagates SQL or scan errors before committing. Conversation
resolution still obtains the actual ID first, including the separate SELECT
needed to see a concurrent creator's commit at READ COMMITTED.

## Repeated results

Both binaries use the same instrumented harness, eight warmed database
connections, `-cpu=8`, and isolated schemas. Ten one-second measurements per
workload alternate baseline/after execution order on successive iterations.
Profiling is performed separately. Workloads are described in the
[previous report](message-save.md#environment-and-method); delivery measures
database save, serialization, and queue fanout, excluding WebSocket transport.

These are benchstat medians of mean request latency (`latency-ns/op`), including
pool and queue waits. Concurrent `ns/op` instead measures aggregate throughput
cost; healthy delivery improves from 554.5 to 416.3 µs/op.

| Workload | Request latency before → after | Change | Database calls/save before → after |
| --- | ---: | ---: | ---: |
| New, serial | 1.669 → 1.233 ms | −26.09% | 6 → 4 |
| New, concurrent | 3.998 → 2.949 ms | −26.22% | 6 → 4 |
| Existing, serial | 1.665 → 1.248 ms | −25.03% | 7 → 5 |
| Existing, concurrent | 4.352 → 3.277 ms | −24.70% | 7 → 5 |
| Contended creation | 4.376 → 3.332 ms | −23.85% | 6.875 → 4.875 |
| Healthy delivery | 4.430 → 3.328 ms | −24.88% | 7 → 5 |
| Saturated delivery | 7.982 → 7.976 ms | −0.07% | 7 → 5 |

All database-bound latency improvements have `p < 0.001`, `n=10`. Saturated
delivery has `p=0.019` but a negligible effect: its one-message-per-millisecond
receiver drain rate remains the limiting factor.

`db-calls/op` counts driver query calls plus `SendBatch` calls, including
BEGIN/COMMIT/ROLLBACK. A warm batch combines three statements in one exchange;
cold statement preparation can add exchanges and is excluded from this metric.
`db-ops/op` counts individual SQL executions, including those inside batches:
6 for new saves, 7 for existing saves, and 6.875 for contended creation in both
binaries. Successful saves use one transaction and zero SQL rollbacks.
Self messages omit the sender lookup and batch two inserts, saving one call;
the timing table uses non-self messages.

### Allocation tradeoff

| Workload | KiB/op before → after | Allocs/op before → after |
| --- | ---: | ---: |
| New, serial | 3.247 → 3.646 | 68 → 92 |
| New, concurrent | 3.250 → 3.651 | 68 → 92 |
| Existing, serial | 3.830 → 4.229 | 74 → 98 |
| Existing, concurrent | 3.825 → 4.229 | 74 → 98 |
| Contended creation | 3.781 → 4.186 | 76 → 100 |
| Healthy delivery | 3.887 → 4.292 | 75 → 99 |
| Saturated delivery | 3.897 → 4.292 | 75 → 99 |

The batch trades about 0.4 KiB and 24 additional allocations per save for lower
latency.

## Profiles and correctness

Separate 12-second healthy/saturated delivery runs captured CPU and block
profiles plus live heap/goroutine snapshots every five seconds after GC, using
a 16-KiB heap sampling rate. The after CPU profile remains I/O/scheduler dominated
(91.3% in the same four functions); `runtime.gcDrain` accounts for 1.27% cumulatively.
Retained heap snapshots are approximately 2.12 → 2.30 MiB (healthy) and
2.13 → 2.10 MiB (saturated), dominated by profiler buffers and connection caches.
These sampled snapshots do not establish a retained-memory improvement.

Delivery goroutine counts are 6 idle → 15 peak → 6 at completion in both timing
binaries. Live profiling snapshots include the CPU profiler and have 16
goroutines. These counts describe the bounded eight-writer benchmark.

The race-enabled suite passes with real PostgreSQL, including new/existing/self
conversations, sender metadata, concurrent creation, commit/rollback of a
competing creator, and rollback on membership/message/sender/commit errors.
Additional checks cover batch preparation failure, a missing sender causing a
scan callback error after successful inserts, and a subsequent successful save.
Unit tests cover batch-close errors and error propagation. Lint and formatting
checks pass.

## Reproduction

Start PostgreSQL using the command in the previous report and export
`POPSOCKET_TEST_DATABASE_URL`. Build `fa4c523` as `tmp/save-batching/baseline.test`
with the current `message_bench_test.go` and `message_postgres_test.go` harness
files, then build the changed version:

```sh
go test -c -o tmp/save-batching/after.test ./pkg/popsocket
```

With fresh output files, run in Bash or Zsh:

```sh
for iteration in {1..10}; do
  revisions=(baseline after)
  if (( iteration % 2 == 0 )); then revisions=(after baseline); fi
  for revision in "${revisions[@]}"; do
    tmp/save-batching/$revision.test -test.run='^$' \
      -test.bench='BenchmarkMessage(StoreSave|Delivery)Postgres' \
      -test.benchmem -test.benchtime=1s -test.count=1 -test.cpu=8 \
      >> tmp/save-batching/$revision.txt || exit 1
  done
done
benchstat tmp/save-batching/baseline.txt tmp/save-batching/after.txt
```

For each binary, capture profiles separately with
`-test.bench=BenchmarkMessageDeliveryPostgres -test.benchtime=12s -test.count=1
-test.cpu=8 -test.memprofilerate=16384`, setting `POPSOCKET_PROFILE_DIR` for live
heap/goroutine snapshots and `-test.cpuprofile`/`-test.blockprofile` output paths.
Inspect heap with `go tool pprof -inuse_space`.

Raw timing results, benchstat comparison, binaries, and profiles are retained
locally in the git-ignored `tmp/save-batching/` directory.
