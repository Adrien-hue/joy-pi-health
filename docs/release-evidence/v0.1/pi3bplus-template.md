# Joy Pi Health v0.1 — Raspberry Pi 3B+ Class C evidence

**Record status:** TEMPLATE — no acceptance result
**Normative gates:** [v0.1 requirements](../../requirements-v0.1.md)
**Procedure:** [release-validation runbook](../../release-validation.md)

Copy this template to `pi3bplus-<UTC-date>-<candidate-short-SHA>/result.md` only for a real completed or stopped Class C run. Never infer a pass or remove an unperformed gate.

## Run identity

| Field | Recorded value |
|---|---|
| Release version | NOT RECORDED |
| Debian package version | NOT RECORDED |
| Candidate commit SHA | NOT RECORDED |
| Acceptance-harness commit SHA | NOT RECORDED |
| CI workflow URL | NOT RECORDED |
| CI run ID / attempt | NOT RECORDED |
| CI artifact name | NOT RECORDED |
| Firmware permission profile | NOT RECORDED |
| Operator | NOT RECORDED |
| Started at, UTC | NOT RECORDED |
| Completed at, UTC | NOT RECORDED |

## Artifact identity

| File | SHA-256 on workstation | SHA-256 on Pi | Match |
|---|---|---|---|
| `joy-pi-health_0.1.0-1_arm64.deb` | NOT RECORDED | NOT RECORDED | NOT RUN |
| `joy-pi-health-v0.1.0-linux-arm64.tar.gz` | NOT RECORDED | NOT RECORDED | NOT RUN |
| `joy-pi-health-v0.1.0-release.json` | NOT RECORDED | NOT RECORDED | NOT RUN |
| `SHA256SUMS` | NOT RECORDED | NOT RECORDED | NOT RUN |

## Reference platform

| Field | Recorded value |
|---|---|
| Raspberry Pi model, raw | NOT RECORDED |
| Canonical release-gating model | Raspberry Pi 3 Model B Plus |
| Known maintained board | Raspberry Pi 3 Model B Plus Rev 1.3; revision `a020d3` |
| Board revision, excluding serial | NOT RECORDED |
| Architecture | NOT RECORDED |
| Raspberry Pi OS release/image | NOT RECORDED |
| Kernel | NOT RECORDED |
| Logical CPUs / online topology | NOT RECORDED |
| Memory / root storage | NOT RECORDED |
| systemd version | NOT RECORDED |
| dpkg version | NOT RECORDED |
| `/dev/vcio` owner/group/mode | NOT RECORDED |
| `/dev/vcio_gencmd` owner/group/mode | NOT RECORDED |
| `video` group | NOT RECORDED |
| Thermal zone identity | NOT RECORDED |
| Firmware/reference-tool version | NOT RECORDED |
| `policy-rc.d` state | NOT RECORDED |
| Acceptance tool versions | NOT RECORDED |

Do not record a unique board serial, machine ID, secret, private key, or unnecessary network address.

## Gate results

Use only `PASS`, `FAIL`, `NOT RUN`, or `BLOCKED`. Add the exact raw-evidence path and a concise factual note for every row.

| Gate | Status | Raw evidence | Result/notes |
|---|---|---|---|
| Same-commit Class A suite | NOT RUN | — | |
| Exact-candidate Class B workflow | NOT RUN | — | |
| Workstation artifact identity | NOT RUN | — | |
| Pi artifact identity | NOT RUN | — | |
| Pi 3B+ / Trixie reference platform | NOT RUN | — | |
| Rootless tar execution | NOT RUN | — | |
| Fresh Debian installation | NOT RUN | — | |
| Managed identity and file ownership | NOT RUN | — | |
| systemd readiness semantics | NOT RUN | — | |
| Clean shutdown within two seconds | NOT RUN | — | |
| Default loopback exposure | NOT RUN | — | |
| HTTP/JSON contract smoke | NOT RUN | — | |
| Partial and issue behavior | NOT RUN | — | |
| Firmware permission profile | NOT RUN | — | |
| Firmware-device confinement | NOT RUN | — | |
| CPU utilization correctness | NOT RUN | — | |
| Logical CPU count | NOT RUN | — | |
| Load averages | NOT RUN | — | |
| Memory semantics | NOT RUN | — | |
| Root-filesystem semantics | NOT RUN | — | |
| Uptime | NOT RUN | — | |
| Network topology/state/counters | NOT RUN | — | |
| SoC temperature | NOT RUN | — | |
| Firmware flag semantics | NOT RUN | — | |
| Degradation logging/suppression | NOT RUN | — | |
| Exit 0 remains stopped | NOT RUN | — | |
| Exit 1 restarts | NOT RUN | — | |
| Exit 2 does not restart | NOT RUN | — | |
| Unexpected failure restarts | NOT RUN | — | |
| Restart backoff progression/cap | NOT RUN | — | |
| Reinstall behavior | NOT RUN | — | |
| Running/stopped upgrade | NOT RUN | — | |
| Running/stopped downgrade | NOT RUN | — | |
| Rollback fixture behavior | NOT RUN | — | |
| Remove behavior | NOT RUN | — | |
| Purge behavior | NOT RUN | — | |
| Administrator drop-in retention | NOT RUN | — | |
| Identity and journal retention | NOT RUN | — | |
| RSS ≤25 MiB | NOT RUN | — | |
| Idle CPU ≤0.5% of one core | NOT RUN | — | |
| Every readiness run ≤1 second | NOT RUN | — | |
| Snapshot p95 ≤100 ms | NOT RUN | — | |
| One-hour endurance | NOT RUN | — | |
| Final hardened-unit capabilities | NOT RUN | — | |
| Final proc/sysfs/statfs/rtnetlink access | NOT RUN | — | |
| Final journal-only logging/no state | NOT RUN | — | |

## Engineering targets and diagnostics

These do not replace the release gates.

| Observation | Result | Target met | Raw evidence |
|---|---:|---|---|
| Maximum steady-state RSS | NOT RECORDED | NOT RUN | — |
| Idle CPU | NOT RECORDED | NOT RUN | — |
| Startup/readiness | NOT RECORDED | NOT RUN | — |
| Snapshot p95 | NOT RECORDED | NOT RUN | — |
| Soak RSS change | NOT RECORDED | NOT RUN | — |

## Lifecycle fixture

| Field | Recorded value |
|---|---|
| Classification | Class C lifecycle fixture — not a release candidate |
| Fixture version | `0.1.0~classc1-1` |
| Source candidate SHA-256 | NOT RECORDED |
| Fixture SHA-256 | NOT RECORDED |
| Payload identity | NOT RUN |
| Maintainer-script identity | NOT RUN |

## Commands executed

Reference `commands.log` and record any manual command not captured by a harness. Do not record secrets.

## Known observations and deviations

- NOT RECORDED

## Final decision

**Decision:** BLOCKED — template only; no Class C run has occurred.

**Rationale:** NOT RECORDED

**Reviewer:** NOT RECORDED
**Review timestamp, UTC:** NOT RECORDED

After review, generate `EVIDENCE_SHA256SUMS` for every retained evidence file except the checksum file itself.
