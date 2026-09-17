# Final measurement summary

Candidate and executed harness: `ad6a4b4e2eb307502c822529863532f2947914d3`. These are retained observations, not new measurements. All raw locators below are relative to the local archive identified in [archive-reference.json](../archive-reference.json).

## Performance

| Gate | Observed | Frozen limit | Engineering target |
|---|---|---|---|
| RSS, 1200 samples after 60 s warmup | Maximum 12444 KiB; 300 idle / 600 requests / 300 idle | 25600 KiB | 20480 KiB |
| Idle CPU, 600.01 s | 0.186664% of one core | 0.5% | 0.25% |
| Readiness, 20 active starts | Maximum/p95 200/200 ms | Every run ≤1000 ms | ≤500 ms |
| Latency, 600 requests | Median 3.172 ms; p95 5.583 ms; all HTTP 200 | p95 ≤100 ms | p95 ≤75 ms |
| Managed shutdown | 156 ms, success, inactive/dead, no restart | ≤2000 ms | — |
| Rootless shutdown | 5 ms | ≤2000 ms | — |
| One-hour soak | 3600 HTTP 200/no-issue requests; 360 resource rows | No sustained resource growth | — |

Soak first/final ten-minute RSS p95: 12484/12484 KiB, growth 0 KiB (limit 1024 KiB), maximum 12652 KiB. PID 748, six FDs and ten threads stayed constant; zero restarts and journal records. No live goroutine instrumentation was added; deterministic bounded-lifecycle coverage is complementary Class A evidence.

Locators: `finalized/raw/pi/raw/{rss-summary.txt,idle-cpu.txt,readiness-summary.txt,latency-summary.txt,soak-summary.txt,managed-shutdown.txt}` and corresponding sample files; rootless shutdown is `finalized/raw/pi/raw/rootless-shutdown.tsv`.

## CPU accuracy

| Plateau | Aggregate reference % | Slice range % | Maximum deviation, percentage points |
|---|---:|---|---:|
| Idle | 0.719246 | 0.497512–1.228501 | 0.509255 |
| Half capacity | 50.608091 | 50.374065–50.746269 | 0.234026 |
| Full capacity | 100.000000 | 100–100 | 0 |

All plateaus valid. Exactly 20 idle/half comparisons: median absolute error 1.534680 pp (limit 5), nearest-rank p95 2.023896 pp (limit 10). Minimum full-load reference and service values both 100% (minimum required 90%). Locators: `finalized/raw/pi/raw/cpu/`, including samples, comparisons, plateaus, full-load observations and summary.

## Logging and supervision

Physical captures establish five stale snapshots per attempt with one warning and one recovery, plus quiet healthy/400 requests. The five-minute reminder and recurrence timing are established by the explicitly approved same-commit controlled-clock test, not by physical waiting. See [result](../result.md#evidence-allocation-and-deviations) and [approvals](../approvals.md).

Type=exec exit-status fixtures: exit 0 stopped successfully without restart; intentional configuration exit 2 did not restart; operational exit 1 restarted seven times. Backoff intervals were approximately 1.15, 2.11, 4.12, 7.81, 15.30, 30.07 and 30.06 seconds.

The packaged Type=notify unit, GOTRACEBACK=crash, Restart=on-failure and RestartPreventExitStatus=2 observed the unexpected failure as `code=killed, status=6/ABRT`, not numeric exit 2. PID changed 60388 → 60416, NRestarts 0 → 1; service recovered active/running and HTTP succeeded. Core soft/hard limits were zero; the explicit tested-process core search returned no files. Locators: `finalized/raw/pi/raw/restart-policy/` and `finalized/raw/pi/raw/degradation/`.

## Lifecycle and final filesystem

Running/stopped reinstall, upgrade and downgrade, version-only rollback fixture and candidate restoration passed. Remove stopped the service; purge preserved administrator drop-in, identity and journal. After removing only the acceptance-created drop-in, all 15 post-purge baseline invariants passed without a compensating helper purge. The next install enabled/started automatically and a distinct-boot capture proved boot startup. Initial blocked/skipped attempts remain preserved separately from passing attempts.

Fixture version: `0.1.0~classc1-1`; payload and maintainer scripts identical to candidate; SHA-256 `8c8b086565cd49e6e0f82381b667eb7a3eb64db4885835dd10900d9fb056cade`. Administrator drop-in SHA-256 stayed `d5200b366d9d95faaaa1b473fa8d37fe62b50ce92d4535698defdd57708c1c62` until its explicit cleanup. Locators: `finalized/raw/pi/raw/lifecycle/`, `finalized/raw/pi-lifecycle-post-purge/`, `finalized/raw/pi/raw/boot-start/`.

Final filesystem capture records 25 package-listed paths, all root:root: directories/executable 0755, data/unit/sysusers files 0644. `dpkg --verify` exited 0 with empty stdout/stderr. Explicitly absent:

- `/var/lib/joy-pi-health`
- `/var/log/joy-pi-health`
- `/var/cache/joy-pi-health`
- `/run/joy-pi-health`

This is not a claim to have exhaustively searched arbitrary unrelated paths. All 13 checksums in the original filesystem manifest verified. Capture UTC 2026-09-16T22:16:46Z–22:16:47Z and directory date 2026-09-17 are retained as observed. Locator: `finalized/raw/pi-final-filesystem-2026-09-17/`.

Final service: packaged Type=notify, active/running, Result=success, PID 748, zero restarts, no drop-ins. UID/GID 985, supplementary video GID 44, all capability sets zero, NoNewPrivileges=yes, ProtectSystem=strict, ProtectHome=yes, PrivateTmp=yes, closed device policy with only the two firmware read allowances. Journal stdout/stderr inheritance; no configured state/log/write paths. Firmware confinement probe succeeded for the firmware query and denied unrelated cec0 access. Locators: `finalized/raw/pi-final-hardening/`, `finalized/raw/pi/raw/confinement-probe/`, `finalized/raw/pi-final-identity/`.
