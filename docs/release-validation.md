# Release validation and Raspberry Pi 3B handoff

This operational runbook defines Class C acceptance for Joy Pi Health v0.1. It does not replace the frozen [requirements](requirements-v0.1.md), [HTTP contract](http-api-v0.1.md), or [architecture](architecture-v0.1.md). When they overlap, those documents remain normative.

The acceptance harness is frozen and reviewed before a physical run. A run must not change the harness in response to observed results. If a harness defect affects a gate, mark that gate `BLOCKED`, preserve its output, correct the harness separately, and restart the affected procedure.

## Evidence classes

- **Class A — developer:** deterministic unit, integration, contract, static-policy, native-build, and Linux/ARM64 compile-only checks.
- **Class B — CI Debian:** deterministic artifact construction and inspection in Debian Trixie, lintian, systemd verification, checksums, and reproducibility.
- **Class C — real Pi 3B:** the exact Class B bytes tested on a real Pi 3B running Raspberry Pi OS Lite 64-bit Debian 13 Trixie.

All three classes are required. CI success cannot establish Pi 3B acceptance.

## Fixed Class C inputs

Use one successful `release-validation` workflow run and exactly these extracted files:

```text
joy-pi-health_0.1.0-1_arm64.deb
joy-pi-health-v0.1.0-linux-arm64.tar.gz
joy-pi-health-v0.1.0-release.json
SHA256SUMS
```

The first permission-profile attempt must have `firmware_access` set to `video` in its metadata. Do not rebuild, rename, edit, or substitute any candidate file between CI and the Pi.

The operator also records the candidate commit, CI workflow URL, run ID, run attempt, downloaded artifact name, and committed acceptance-harness revision.

Before the first command, create `commands.log`. Record each command with a UTC timestamp and capture its stdout/stderr into the named raw-evidence file. Never place passwords, tokens, private keys, board serials, or machine IDs in the transcript.

## Harness commands

The scripts require an explicit evidence directory and never report a release decision. They retain observations from which the operator assigns gate status.

```text
sh test/release/pi3b/verify-readonly.sh verify-artifacts ARTIFACT_DIR EVIDENCE_DIR EXPECTED_COMMIT [video|acl]
sh test/release/pi3b/verify-readonly.sh inspect-platform EVIDENCE_DIR
sh test/release/pi3b/verify-readonly.sh rootless ARTIFACT_DIR EVIDENCE_DIR
sh test/release/pi3b/verify-readonly.sh managed EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh cpu EVIDENCE_DIR http://127.0.0.1:8080/v1/snapshot --allow-load
sh test/release/pi3b/measure-performance.sh rss EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh idle-cpu EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh readiness EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh latency EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh soak EVIDENCE_DIR
sh test/release/pi3b/derive-lifecycle-fixture.sh CANDIDATE_DEB TEMPORARY_OUTPUT_DIRECTORY
```

The CPU command deliberately requires `--allow-load`. The readiness command must be run by an operator already authorized to start and stop the service. The scripts never invoke `sudo`, install software, alter device permissions, or edit systemd configuration.

Recommended acceptance-only tools are `curl`, `jq`, `sysstat`, `stress-ng`, `file`, `binutils`, `iproute2`, and the Raspberry Pi firmware reference utility. Installation is an explicit operator action and every installed tool version belongs in the evidence.

## 1. Artifact handoff

On the workstation:

1. Download the artifact whose name starts with `joy-pi-health-ci-validation-` from the chosen successful workflow run.
2. Extract it into a new directory containing only the four fixed inputs.
3. Run the artifact verification harness with the candidate commit.
4. Record the SHA-256 of all four files, including `SHA256SUMS` itself.
5. Transfer that directory unchanged through an operator-controlled channel such as `scp -p` or removable media.

Use separate `EVIDENCE_DIR/workstation` and `EVIDENCE_DIR/pi` destinations so neither side overwrites the other's hashes. On the Pi, run the same verification command again before extracting or installing anything. A missing file, unexpected file, checksum mismatch, metadata mismatch, wrong version/architecture, dynamic executable, or executable mismatch is a release-blocking `FAIL`.

## 2. Pre-install reference-platform inspection

Use a fresh reference image with no prior Joy Pi Health package. Installing acceptance tools does not make the application a runtime dependency, but the installations and versions must be recorded.

Run:

```text
sh test/release/pi3b/verify-readonly.sh inspect-platform EVIDENCE_DIR
```

Review and record:

- `aarch64`, Debian `trixie`, and Raspberry Pi 3 Model B identity;
- OS release, kernel, CPU revision and online topology, excluding the unique serial number;
- memory and root storage;
- systemd and dpkg versions;
- `/dev/vcio` and `/dev/vcio_gencmd` presence, type, ownership, group, mode, and major/minor numbers;
- `video` group presence;
- thermal zones and their types;
- firmware reference-tool version;
- whether `policy-rc.d` is present;
- absence of an installed Joy Pi Health package.

Do not change device DAC, ACLs, ownership, groups, or service configuration before the managed permission test. If the image is not a fresh Pi 3B Trixie environment, mark the affected gates `BLOCKED` and stop.

## 3. Rootless portable-artifact acceptance

As an ordinary non-root user, run:

```text
sh test/release/pi3b/verify-readonly.sh rootless ARTIFACT_DIR EVIDENCE_DIR
```

The harness checks archive path safety, ARM64 execution, `--help`, default loopback startup, immediate and warmed snapshots, trailing CPU availability, clean shutdown, unused stdout, and absence of files in the process working directory.

Review the captured snapshot against independent host observations. Firmware fields may be unavailable when the ordinary user lacks device access; correct null-plus-issue behavior is acceptable here. Do not use root or change permissions to make the portable run succeed. Shell redirection into the operator-owned evidence directory is not an application-owned log file.

## 4. Fresh Debian installation

First record whether `/usr/sbin/policy-rc.d` exists and what policy it returns. Then install the exact package:

```text
sudo apt install ./joy-pi-health_0.1.0-1_arm64.deb
```

If policy blocks automatic start, record that as correct Debian-policy behavior and explicitly start the service for later checks.

Verify manually:

- package, version, architecture, installed file list, ownership, and modes;
- `/usr/bin/joy-pi-health` identity;
- locked `_joy-pi-health` dynamic system user/group, `/nonexistent` home, and `/usr/sbin/nologin`;
- no writable application state, cache, runtime, spool, or log directory;
- unit and sysusers installation;
- no package-owned file beneath `/etc/systemd/system/joy-pi-health.service.d/`;
- Debian policy-aware enable/start behavior.

Do not invoke maintainer scripts directly. Their helper policy is Class A/B evidence; Class C observes package-manager behavior.

## 5. Managed readiness and HTTP behavior

With the exact packaged unit active, run:

```text
sh test/release/pi3b/verify-readonly.sh managed EVIDENCE_DIR
```

Confirm the process uses `_joy-pi-health`, has zero effective capabilities, listens only on `127.0.0.1:8080`, uses `Type=notify`, and exposes no health, readiness, metrics, or administrative route.

The harness captures the frozen route/method/query/body behavior and the current snapshot. Review response headers, schema version, timestamp, exact integer tokens, nulls, allowed issues, issue ordering, and the 64 KiB bound. Values above `2^53` remain same-commit Class A fixture evidence unless a real host counter naturally reaches that range.

Run the readiness measurement after returning the unit to its packaged configuration:

```text
sh test/release/pi3b/measure-performance.sh readiness EVIDENCE_DIR
```

Every one of 20 clean starts must reach systemd readiness in no more than one second. Record the 500 ms engineering target separately. An immediate response may still have unavailable CPU utilization because readiness does not wait for a second CPU sample.

Exact-once notification, the initial baseline-attempt ordering, and absence of a readiness-time firmware request are combined with same-commit Class A evidence; do not claim a direct observation that systemd does not expose.

Measure clean operator stop separately and require completion within two seconds with no restart.

## 6. Firmware permission profile

Test the `video` profile exactly as packaged:

1. Record device DAC and service supplementary groups.
2. Confirm `DevicePolicy=closed` and exactly `/dev/vcio r` plus `/dev/vcio_gencmd r` allowances.
3. Require the warmed managed snapshot to contain all four firmware booleans.
4. Compare them with one independent `vcgencmd get_throttled` mask using bits 0, 2, 16, and 18.
5. Preserve observed `false`; do not convert it to unavailability.
6. Confirm the service has no root helper, setuid helper, ambient capability, or effective capability.
7. With explicit operator approval, use a separate transient service carrying the same identity, address-family, capability, and device restrictions to prove allowed firmware access and read-only denial of an existing unrelated sensitive device. Do not write to any device.

An absent device node is recorded. It does not alone fail the release if the supported alternate node supplies the coherent observation and both exact unit allowances remain present.

If firmware acquisition fails, preserve the result and stop the run. Do not patch the unit or device permissions. Build an ACL-profile candidate through the normal release workflow, rerun Class A/B, and begin a new Class C evidence record with those exact bytes. Evidence from the two profiles must not be combined. Failure of both profiles blocks v0.1.

## 7. Metric acceptance

Bracket service requests closely with independent observations and retain every pair.

- **CPU utilization:** 20 pairs, ten at normal idle and ten under steady 50% all-core load. Median absolute difference must be at most 5 percentage points and nearest-rank p95 at most 10. A separate sustained all-core workload must produce at least 90% in every retained steady-state sample.
- **Logical CPUs:** exact agreement with online OS topology.
- **Load:** each value must fall within the immediately bracketing `/proc/loadavg` values, allowing only the source's 0.01 display quantum.
- **Memory:** bracket with `/proc/meminfo`; total/available semantics must agree and `used = total - available` exactly.
- **Root filesystem:** compare with a filesystem-stat observation made as `_joy-pi-health`; available means available to that identity and arithmetic must be exact.
- **Uptime:** value must fall within bracketing `/proc/uptime` observations with the frozen +1 second allowance.
- **Network:** compare topology, operational state, RX, and TX to kernel sources. Exclude loopback, retain eligible down interfaces, and use only operator-approved traffic to a private-LAN peer or default gateway to prove counter growth.
- **Temperature:** compare with the selected `cpu-thermal` zone within ±1°C.
- **Firmware:** compare all four fields to the same independent firmware mask. Never induce undervoltage, overheating, or throttling solely for acceptance, and never clear historical bits.

Run the fixed CPU procedure only after workload approval:

```text
sh test/release/pi3b/measure-performance.sh cpu EVIDENCE_DIR http://127.0.0.1:8080/v1/snapshot --allow-load
```

Counter resets, reboots, topology mutation, malformed data, impossible values, and internal defects use same-commit deterministic Class A evidence. Do not alter the Pi to manufacture them.

## 8. Degradation logging

After firmware permission acceptance, safely induce stale CPU publication by stopping the service process with SIGSTOP for longer than 1.25 seconds and then sending SIGCONT. Do not stop systemd itself.

Verify:

- the first resulting CPU issue logs promptly;
- repeated identical snapshots are suppressed;
- a continuing issue may log after the fixed five-minute interval;
- recovery logs once;
- recurrence after recovery logs promptly;
- successful requests and client 4xx responses do not create degradation logs;
- no complete snapshot payload is logged.

Do not manufacture firmware busy/timeout, unsupported-platform, or internal-defect paths on the device.

## 9. Restart and shutdown acceptance

Use one acceptance-only runtime drop-in under `/run/systemd/system/joy-pi-health.service.d/`. Remove it and run `daemon-reload` after every case. Never edit the packaged unit.

Test these `ExecStart` substitutions independently:

- `/usr/bin/joy-pi-health --help`: clean exit 0, no restart.
- `/usr/bin/joy-pi-health --listen-port=0`: configuration exit 2, no readiness and no restart.
- `/usr/bin/joy-pi-health --listen-address=192.0.2.1 --allow-non-loopback=true`: valid configuration followed by operational bind failure, exit 1 and restart.

For unexpected failure, restore the packaged unit, send SIGABRT to the main process, and verify restart. Record this as unexpected-failure supervision evidence, not as an induced internal collector defect. SIGTERM/operator stop must remain graceful and stopped.

Use `NRestarts` and monotonic journal timestamps to record the initial one-second retry, five increasing configured steps, approximately 30-second maximum, and continued attempts at the maximum. A permanent `start-limit-hit` contradicts continued bounded restart and is a release-blocking failure.

## 10. Package lifecycle

Create the non-release fixture in a temporary directory:

```text
sh test/release/pi3b/derive-lifecycle-fixture.sh joy-pi-health_0.1.0-1_arm64.deb TEMPORARY_OUTPUT_DIRECTORY
```

Record its provenance and hash. It exists only because no older official package exists and must never be described as a release candidate.

Create one labelled administrator-owned drop-in under `/etc/systemd/system/joy-pi-health.service.d/`, record its hash, and verify it survives:

- exact-candidate reinstall while running;
- exact-candidate reinstall while stopped;
- running upgrade from fixture to `0.1.0-1`;
- stopped upgrade;
- running downgrade with `--allow-downgrades`;
- stopped downgrade;
- rollback to the retained fixture;
- restoration to exact `0.1.0-1`;
- remove;
- purge.

Verify restart-only-when-running behavior, intentionally stopped behavior, removal of package-owned assets, retention of the administrator drop-in, service identity, and journal, and absence of state migration. Remove only the acceptance-created drop-in during cleanup.

## 11. Performance and endurance

Restore the exact `0.1.0-1` package, final packaged unit, default loopback settings, and accepted permission profile first. Record ambient load, temperature, CPU frequency/governor, memory, tool versions, and process identity without tuning the machine in response to results.

Run:

```text
sh test/release/pi3b/measure-performance.sh rss EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh idle-cpu EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh latency EVIDENCE_DIR
sh test/release/pi3b/measure-performance.sh soak EVIDENCE_DIR
```

Fixed conditions and gates:

- **RSS:** 60-second warm-up, five-minute idle, ten minutes at one request/second, five-minute idle; maximum `VmRSS` ≤25 MiB, with ≤20 MiB recorded as the engineering target.
- **Idle CPU:** ten minutes without requests; process ticks divided by monotonic elapsed time and `CLK_TCK`; ≤0.5% of one core, with ≤0.25% as the target.
- **Readiness:** 20 starts; every result ≤1 second, with ≤500 ms as the target.
- **Latency:** 600 sequential fresh loopback connections started one second apart; nearest-rank p95 ≤100 ms, with ≤75 ms as the target.
- **Endurance:** one hour at one request per second; retain PID, restart count, RSS, CPU ticks, file descriptors, threads, statuses, issue counts, and journal counts every ten seconds.

Endurance requires no fatal exit, restart, stuck-executor growth, descriptor/thread growth, or log storm. The final ten-minute RSS p95 must be no more than 1 MiB above the first ten-minute p95, while the 25 MiB hard ceiling remains authoritative.

Engineering-target misses are recorded but do not replace or tighten the frozen release gates. Never adjust a threshold after observing a result.

## 12. Final hardening acceptance

Under the restored final unit, use successful service observations plus read-only system inspection to confirm procfs, thermal sysfs, root statfs, rtnetlink, TCP loopback, systemd readiness, firmware ioctl, device confinement, zero unexpected capabilities, no writable application paths, journal-only logging, and unprivileged execution. Do not substitute a relaxed unit for this final check.

## 13. Evidence and release decision

Copy the [evidence template](release-evidence/v0.1/pi3b-template.md) into:

```text
docs/release-evidence/v0.1/pi3b-<UTC-date>-<candidate-short-SHA>/result.md
```

Store reviewed text, JSON, and TSV evidence under its `raw/` directory. Do not commit candidate artifacts, unique serials, machine IDs, secrets, private keys, or unfiltered system logs. Produce `EVIDENCE_SHA256SUMS` after review.

Statuses mean:

- `PASS`: executed with retained evidence and met the frozen condition;
- `FAIL`: executed and violated the condition;
- `NOT RUN`: not attempted;
- `BLOCKED`: a prerequisite, environment, or harness defect prevented a valid result.

Every release-blocking Class A, B, and C gate must pass. Any release-blocking `FAIL`, `NOT RUN`, or `BLOCKED` prevents acceptance. Pi 3B+, 4B, and 5 remain non-blocking and evidence-limited.

## 14. Harness defect rule

During Class C execution, do not modify the committed harness, rebuild the artifacts, or combine results from different candidate bytes or permission profiles.

If a harness defect is found:

1. stop the affected gate;
2. mark it `BLOCKED`;
3. retain the raw output;
4. correct the harness in a separate reviewed change;
5. rerun applicable repository and shell checks;
6. restart every affected measurement whose comparability is no longer guaranteed.

## 15. Cleanup

After success, remove temporary fixtures and acceptance-created drop-ins, restore the exact `0.1.0-1` package and packaged unit, reload systemd, and leave the service enabled, active, unprivileged, and loopback-only.

After failure, preserve evidence before cleanup. Remove only acceptance-created files. Do not manually repair firmware permissions and reinterpret the failed run. Package removal retains the service identity, administrator drop-ins, and journal as required.

## Operator-only command patterns

The following commands intentionally change package or service state and are never run by the read-only harness. Run them individually, capture their output, and verify each resolved path before use.

Inspect the installed identity and unit:

```sh
dpkg-query -W -f='${Package} ${Version} ${Architecture} ${Status}\n' joy-pi-health
getent passwd _joy-pi-health
getent group _joy-pi-health
sudo passwd -S _joy-pi-health
systemctl cat joy-pi-health.service
systemctl show joy-pi-health.service -p Type -p NotifyAccess -p User -p Group -p SupplementaryGroups -p MainPID -p NRestarts
```

Perform the fresh installation and final restoration:

```sh
sudo apt install ./joy-pi-health_0.1.0-1_arm64.deb
sudo systemctl enable joy-pi-health.service
sudo systemctl start joy-pi-health.service
```

For each exit-status case, create only this runtime drop-in and replace the final `ExecStart` line with the case under test:

```sh
sudo install -d -m 0755 /run/systemd/system/joy-pi-health.service.d
printf '%s\n' '[Service]' 'ExecStart=' 'ExecStart=/usr/bin/joy-pi-health --help' | sudo tee /run/systemd/system/joy-pi-health.service.d/acceptance.conf >/dev/null
sudo systemctl daemon-reload
sudo systemctl reset-failed joy-pi-health.service
sudo systemctl start joy-pi-health.service
systemctl show joy-pi-health.service -p ExecMainCode -p ExecMainStatus -p Result -p NRestarts
```

The other exact `ExecStart` values are:

```text
ExecStart=/usr/bin/joy-pi-health --listen-port=0
ExecStart=/usr/bin/joy-pi-health --listen-address=192.0.2.1 --allow-non-loopback=true
```

Remove the runtime override after every case:

```sh
sudo rm -f /run/systemd/system/joy-pi-health.service.d/acceptance.conf
sudo systemctl daemon-reload
sudo systemctl reset-failed joy-pi-health.service
```

Exercise controlled signal behavior only after recording the main PID:

```sh
systemctl show joy-pi-health.service -p MainPID -p NRestarts
sudo systemctl kill --kill-whom=main --signal=ABRT joy-pi-health.service
sleep 2
systemctl show joy-pi-health.service -p MainPID -p NRestarts -p Result
sudo systemctl stop joy-pi-health.service
```

For CPU degradation, send STOP and CONT to that recorded main PID, confirm it has not changed between commands, and retain the journal interval:

```sh
sudo kill -STOP MAIN_PID
sleep 2
sudo kill -CONT MAIN_PID
curl --silent --show-error http://127.0.0.1:8080/v1/snapshot
journalctl --unit joy-pi-health.service --since 'UTC_START_TIME' --no-pager
```

Compare firmware flags without changing them:

```sh
vcgencmd get_throttled
curl --silent --show-error http://127.0.0.1:8080/v1/snapshot | jq '.raspberry_pi'
```

An operator-approved, read-only confinement probe may mirror the package unit in a transient service. Select an existing firmware node and an existing unrelated sensitive device first. The probe must use the service identity, no capabilities, and the same closed two-device policy. It may attempt a one-byte read from the unrelated device but must never write to it.

Use these package-manager forms for lifecycle tests:

```sh
sudo apt install --reinstall ./joy-pi-health_0.1.0-1_arm64.deb
sudo apt install --allow-downgrades ./joy-pi-health_0.1.0~classc1-1_arm64.deb
sudo apt install ./joy-pi-health_0.1.0-1_arm64.deb
sudo apt remove joy-pi-health
sudo apt purge joy-pi-health
```

Before every install, reinstall, upgrade, or downgrade, record `systemctl is-active` and `systemctl is-enabled`. After it, record those values again plus `MainPID`, `NRestarts`, package version, drop-in hash, identity, and journal presence. Explicitly alternate running and stopped states according to the package-lifecycle section; do not treat one command sequence as evidence for both states.

The acceptance-created administrator drop-in must be a uniquely named file beneath `/etc/systemd/system/joy-pi-health.service.d/`. Record its exact path and hash. Cleanup removes that exact file only; never recursively remove the drop-in directory or touch pre-existing administrator files.

## Commit boundaries

The harness/runbook is reviewed and committed before physical testing, separately from evidence. A suitable harness commit is:

```text
test: add Raspberry Pi 3B release acceptance harness
```

During physical measurement, do not commit. After the completed run, a separate evidence-only change may add genuine reviewed results. A suitable later commit is:

```text
test: record Raspberry Pi 3B v0.1 release acceptance
```

No product, runtime, packaging, CI, or permission correction belongs in the evidence commit.
