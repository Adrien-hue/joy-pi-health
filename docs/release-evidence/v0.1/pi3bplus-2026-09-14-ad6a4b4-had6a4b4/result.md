# Joy Pi Health v0.1 — Pi 3B+ final acceptance

**COMPLETE — PASS: 49/49 gates PASS; 0 FAIL, 0 BLOCKED, 0 NOT RUN.**

This is the curated record of the completed physical campaign, not a new test run. [Requirements](../../../requirements-v0.1.md) and the [runbook](../../../release-validation.md) remain authoritative. The [retention policy](README.md) and [archive reference](archive-reference.json) identify the complete unchanged local evidence. The full original result and detailed evidence index remain at local archive locator `finalized/result.md`.

## Identity and provenance

| Field | Recorded value |
|---|---|
| Candidate SHA | `ad6a4b4e2eb307502c822529863532f2947914d3` |
| Executed harness SHA | `ad6a4b4e2eb307502c822529863532f2947914d3` |
| Release / Debian version | 0.1.0 / 0.1.0-1, arm64 |
| Class A | [Quality run 34893626368](https://github.com/Adrien-hue/joy-pi-health/actions/runs/34893626368) |
| Class B | [Release-validation run 34893626428](https://github.com/Adrien-hue/joy-pi-health/actions/runs/34893626428), attempt 1 |
| Downloaded artifact | `joy-pi-health-ci-validation-ad6a4b4e2eb307502c822529863532f2947914d3-34893626428-1` |
| Artifact ID | `10367678391` |
| Reference platform | Raspberry Pi 3 Model B Plus Rev 1.3, revision a020d3; four CPUs; Debian 13 Trixie, aarch64 |
| Kernel / systemd | 6.18.34+rpt-rpi-v8 / 257.13-1~deb13u1 |
| Baseline / permission profile | Verified package reset, second attempt PASS / video |
| Operator / staging | joyteaser / `/home/joyteaser/joy-pi-health-classc-2026-09-14-ad6a4b4-had6a4b4` |
| Campaign start | 2026-09-14 staging date; exact campaign-start timestamp NOT RETAINED |
| Last physical capture, UTC | 2026-09-16T22:16:47Z; capture directory named 2026-09-17, unchanged |
| Review / explicit approvals | 2026-09-17 Europe/Paris; [approvals](approvals.md) |
| Transcript | **commands.log: NOT RETAINED** |

Authenticated job metadata and exact checkout logs established successful Ubuntu quality, Windows quality, Linux ARM64 compile-only, and Debian Trixie artifact validation/reproducibility for this SHA. Windows shell-harness checks were intentionally Linux-only; Windows Go quality passed. The independent CI download matched the artifact digest and all four bundle files. See [provenance](summaries/provenance.json) and [artifact identity](summaries/artifact-identity.txt). No candidate artifact is part of this Git record.

## Gate classifications

The 49 gate names, statuses and result notes below are preserved from the finalized record. Evidence locators are relative to the ignored local archive, not Git links. Each row gives a primary locator; `finalized/result.md` retains the complete multi-file index. PASS describes the valid final observation, not every historical attempt. Physical and deterministic coverage are explicitly distinguished.

| Gate | Status | Primary local archive locator | Result/notes |
|---|---|---|---|
| Same-commit Class A suite | PASS | `finalized/raw/ci/run-34893626368-jobs.json` | Exact SHA; native tests/vet/build plus ARM64 compile-only checks |
| Exact-candidate Class B workflow | PASS | `finalized/raw/ci/run-34893626428-jobs.json` | Trixie artifact validation and reproducibility, attempt 1 |
| Workstation artifact identity | PASS | `finalized/raw/workstation-artifact-hashes.txt` | Four exact files agree |
| Pi artifact identity | PASS | `finalized/raw/pi/raw/artifact-hashes.txt` | Exact candidate bytes retained |
| Pi 3B+ / Trixie reference platform | PASS | `finalized/raw/pi/raw/platform.txt` | Correct model, revision, architecture and release |
| Rootless tar execution | PASS | `finalized/raw/pi/raw/rootless-help.txt` | Rootless capture completed; warm CPU available; stdout empty; stop 5 ms |
| Fresh or verified fresh-install-equivalent Debian installation | PASS | `finalized/raw/pi-baseline-attempt-2/raw/package-baseline-post-checks.tsv` | Automatic enabled/active Type=notify, no manual enable/start used to establish this result |
| Automatic startup after reboot | PASS | `finalized/raw/pi/raw/boot-start` | Distinct original boot IDs verified; active/enabled PID 748, NRestarts=0; successful snapshot |
| Package baseline state | PASS | `finalized/raw/pi-baseline-attempt-2/raw/package-baseline-pre.txt` | All 15 post-reset invariants pass |
| Managed identity and file ownership | PASS | `finalized/raw/pi-final-hardening/raw/process-security.txt` | UID/GID 985; package paths root:root, directories/executable 0755, data/unit 0644 |
| systemd readiness semantics | PASS | `finalized/raw/pi/raw/systemd.txt` | Real Type=notify starts; exact-once/order assertions use same-commit deterministic tests as allowed by runbook §5 |
| Clean shutdown within two seconds | PASS | `finalized/raw/pi/raw/managed-shutdown.txt` | 0.156 seconds; success, inactive/dead, no restart |
| Default loopback exposure | PASS | `finalized/raw/pi-final-hardening/raw/managed-listeners.txt` | Only 127.0.0.1:8080 for this service; port 22 belongs to SSH |
| HTTP/JSON contract smoke | PASS | `finalized/raw/pi/raw/http-contract.tsv` | All 15 cases agree; schema, headers, error bodies and size checked by unchanged harness |
| Partial and issue behavior | PASS | `finalized/raw/pi/raw/degradation/attempt-2` | CPU null with exactly temporarily_unavailable; healthy recovery. Other malformed/internal/large-counter paths are deterministic evidence, not induced Pi failures |
| Firmware permission profile | PASS | `finalized/raw/pi/raw/firmware-check` | Video profile supplies all four booleans without root/capabilities |
| Firmware-device confinement | PASS | `finalized/raw/pi/raw/confinement-probe/probe.txt` | Firmware query succeeds; unrelated cec0 read denied EPERM under transient equivalent restrictions; final unit retains closed policy |
| CPU utilization correctness | PASS | `finalized/raw/pi/raw/cpu/cpu-samples.tsv` | Valid plateaus; 20 comparisons; median 1.534680 pp, p95 2.023896 pp; full load 100% |
| Logical CPU count | PASS | `finalized/raw/pi/raw/metric-observations/online-before.txt` | 0-3 corresponds to 4 |
| Load averages | PASS | `finalized/raw/pi/raw/metric-observations/load-before.txt` | 0.35 / 0.22 / 0.08 exact agreement |
| Memory semantics | PASS | `finalized/raw/pi/raw/metric-observations/memory-before.txt` | Total 949071872, available 728920064, used 220151808 bytes; KiB conversion and subtraction agree |
| Root-filesystem semantics | PASS | `finalized/raw/pi/raw/metric-observations/root-before.txt` | Identity-scoped statfs bracket; available 11656126464 lies between references; total/used arithmetic exact |
| Uptime | PASS | `finalized/raw/pi/raw/metric-observations/uptime-before.txt` | 531265 seconds against 531265.28–531265.35, within one-second allowance |
| Network topology/state/counters | PASS | `finalized/raw/pi/raw/metric-observations` | eth0 up, wlan0 down retained, lo excluded; counters bracketed; approved gateway traffic yields RX +6650 and TX +6564 |
| SoC temperature | PASS | `finalized/raw/pi/raw/metric-observations/thermal-types.txt` | 53.154 °C against 53.154 / 53.692 °C, within ±1 °C |
| Firmware flag semantics | PASS | `finalized/raw/pi/raw/firmware-check/reference-before.txt` | 0x80000: monitored bits 0,2,16,18 clear; four false values agree. No fault or history reset induced |
| Degradation logging/suppression | PASS | `finalized/raw/pi/raw/degradation` | Combined physical/deterministic evidence under approved clarification; see allocation below |
| Exit 0 remains stopped | PASS | `finalized/raw/pi/raw/restart-policy/help-result.txt` | Type=exec fixture: status 0, success, inactive/dead, NRestarts=0 |
| Exit 1 restarts | PASS | `finalized/raw/pi/raw/restart-policy/exit1-result.txt` | Operational failure status 1, seven automatic restarts |
| Exit 2 does not restart | PASS | `finalized/raw/pi/raw/restart-policy/exit2-result.txt` | Intentional configuration status 2, NRestarts=0 despite GOTRACEBACK=crash |
| Unexpected failure restarts | PASS | `finalized/raw/pi/raw/restart-policy/abrt-before.txt` | Packaged Type=notify: killed by signal 6, automatic restart, replacement ready PID |
| Restart backoff progression/cap | PASS | `finalized/raw/pi/raw/restart-policy/exit1-before.txt` | Approximately 1.15, 2.11, 4.12, 7.81, 15.30, 30.07, 30.06 seconds; continued at cap, no permanent start limit |
| Reinstall behavior | PASS | `finalized/raw/pi/raw/lifecycle/reinstall-running-after.txt` | Running restarted; stopped remained stopped; package-manager statuses 0 0 |
| Running/stopped upgrade | PASS | `finalized/raw/pi/raw/lifecycle/upgrade-running-attempt-2-after.txt` | Final versions 0.1.0-1; running PID replaced, intentionally stopped PID 0; original skipped attempt retained |
| Running/stopped downgrade | PASS | `finalized/raw/pi/raw/lifecycle/downgrade-running-after.txt` | Fixture version 0.1.0~classc1-1; state preserved appropriately |
| Rollback fixture behavior | PASS | `finalized/raw/pi/raw/lifecycle/fixture-provenance.txt` | Version-only fixture, then exact candidate restored; no state migration required |
| Remove behavior | PASS | `finalized/raw/pi/raw/lifecycle/remove-remove.txt` | Running service stopped; unit not found; no 8080 listener; residual rc package state before purge expected |
| Purge behavior | PASS | `finalized/raw/pi/raw/lifecycle/purge-purge.txt` | No compensating helper purge; all 15 invariants pass; next install automatically enabled/started |
| Administrator drop-in retention | PASS | `finalized/raw/pi/raw/lifecycle` | Same SHA throughout; only acceptance-created drop-in removed after retention proof |
| Identity and journal retention | PASS | `finalized/raw/pi/raw/lifecycle/purge-identity-after.txt` | UID/GID 985 and prior journal entries retained |
| RSS ≤25 MiB | PASS | `finalized/raw/pi/raw/rss.tsv` | 1200 samples, maximum 12444 KiB; 300 idle / 600 requests / 300 idle |
| Idle CPU ≤0.5% of one core | PASS | `finalized/raw/pi/raw/idle-cpu.txt` | 0.186664% over 600.01 seconds |
| Every readiness run ≤1 second | PASS | `finalized/raw/pi/raw/readiness.tsv` | All 20 active; maximum and p95 200 ms |
| Snapshot p95 ≤100 ms | PASS | `finalized/raw/pi/raw/latency.tsv` | 600 fresh connections, p95 5.583 ms, median 3.172 ms |
| One-hour endurance | PASS | `finalized/raw/pi/raw/soak.tsv` | 3600 HTTP 200/no-issue responses; 360 resource rows; PID 748, six FDs, ten threads constant; no restarts or logs; max RSS 12652 KiB, p95 growth zero |
| Final hardened-unit capabilities | PASS | `finalized/raw/pi-final-hardening/raw/hardening.txt` | All capability sets zero; NNP; strict system protection; closed devices; no override |
| Final proc/sysfs/statfs/rtnetlink access | PASS | `finalized/raw/pi-final-hardening/raw/managed-snapshot.json` | All metric groups observed under packaged unprivileged unit; issues empty |
| Final journal-only logging/no state | PASS | `finalized/raw/pi-final-hardening/raw/hardening.txt` | stdout journal, stderr inherit; no configured state/log directory, all four named application paths absent |
| Final filesystem capture integrity | PASS | `finalized/raw/pi-final-filesystem-2026-09-17/SHA256SUMS` | All 13 capture checksums verified, error files empty, dpkg verification exit 0 |

## Evidence allocation and deviations

The [measurement summary](summaries/measurements.md) records performance, CPU, filesystem and supervision outcomes. No measurements were rerun for this restructuring.

- **Logging:** physical Pi evidence proves initial warning, suppression across five stale requests, recovery, and healthy/400 request quietness. Same-commit `TestDegradationReporterSuppressesRemindsAndRecovers` proves suppression at five minutes minus one nanosecond, reminder at five minutes, recovery and immediate recurrence; production interval is `5 * time.Minute`. Physical recurrences about sixteen minutes apart do not prove the within-five-minute reset. The explicitly approved combined evidence model changes no requirement or threshold.
- **commands.log: NOT RETAINED.** The approved 2026-09-17 one-run deviation relies on retained per-gate command files and outputs; no transcript was recreated and no future-run exemption is implied.
- Initial baseline BLOCKED by an empty prior runtime drop-in directory is preserved; the independent second reset attempt supplies the passing inventory and all 15 invariants. See `finalized/raw/pi/raw/package-baseline-post-checks.tsv` and `finalized/raw/pi-baseline-attempt-2/`.
- The first running-upgrade operation was skipped, leaving the fixture version/PID unchanged. Its outputs remain preserved; only `upgrade-running-attempt-2` establishes PASS. Both are under `finalized/raw/pi/raw/lifecycle/`.
- Supplementary final-hardening writes initially failed with permission denied; the honest operator note and subsequent read-only captures remain under `finalized/raw/pi-final-hardening/`. No exit status was fabricated.
- Readiness produced tolerated unloaded-unit reset-failed messages; all 20 measured starts were active within threshold with exit status 0.
- Earlier 0922f75/30bef36/8b837a6 blocked observations and the valid 1d91952 unexpected-failure FAIL remain separate, unchanged historical runs, not failures of ad6a4b4.
- The finalized reviewed copy retains existing redactions of boot IDs and unnecessary LAN addresses, and its ABRT journal omits the runtime stack/register dump while retaining signal/lifecycle evidence. `finalized/SOURCE_INVENTORY.tsv` records source hashes and treatments. Both the reviewed files and unredacted `original-capture/` are now preserved byte-for-byte; no new redaction occurred.

## Decision and integrity

**Overall Class C: PASS — acceptance record complete.** All 49 required current-candidate gates remain supported by the finalized evidence and explicit approvals. No required gate remains FAIL, BLOCKED or NOT RUN. Compatibility claims for other Pi models are not strengthened.

Reviewer: Codex evidence review; evidence-model and transcript-deviation decisions explicitly approved by the user on 2026-09-17. Retention restructuring is documentation-only and does not change candidate or executed harness identity.

`RECORD_SHA256SUMS` seals this curated record. `archive-reference.json` pins the complete local archive and original finalized seal. Historical evidence is not rewritten. No automatic commit, release tag or publication is authorized.
