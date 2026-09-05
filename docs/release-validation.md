# Release validation and Raspberry Pi 3B handoff

This document describes validation evidence and handoff for Joy Pi Health v0.1. It is an operational procedure, not a replacement for the frozen [requirements](requirements-v0.1.md), [HTTP contract](http-api-v0.1.md), or [architecture](architecture-v0.1.md).

## Evidence classes

### Class A: developer evidence

Developer evidence includes deterministic unit, integration, contract, static-policy, native-build, and Linux/ARM64 compile-only checks. It supports development but does not validate Debian Trixie behavior or Raspberry Pi hardware.

### Class B: CI Debian evidence

CI Debian evidence includes construction of the exact artifacts in the pinned Debian Trixie environment, package and archive inspection, lintian, systemd unit verification, checksums, and byte-for-byte reproducibility. Uploaded files are CI validation artifacts, not official releases.

### Class C: real Pi 3B release-gating evidence

Only Class C satisfies the v0.1 hardware and release gates. It uses the exact Class B bytes on a real Raspberry Pi 3B running the supported Raspberry Pi OS Lite Trixie image. It covers installation, metrics, firmware-device permissions, readiness, restart behavior, lifecycle operations, resource gates, and soak testing.

## Handoff to the Pi 3B

1. Select a successful push or manually dispatched CI run for the candidate commit.
2. Download the artifact whose name begins `joy-pi-health-ci-validation-` without rebuilding it.
3. Run `sha256sum --check SHA256SUMS` before transfer.
4. Record the workflow run, version, commit, artifact hashes, and `firmware_access` value from the release metadata.
5. Transfer the complete artifact directory to the Pi through the operator's chosen manual channel.
6. Run `sha256sum --check SHA256SUMS` again on the Pi before installation.
7. Install and test the included Debian package. Do not substitute a locally rebuilt package.

## Evidence record

Record at least:

- operator and UTC date;
- CI workflow run URL and attempt;
- version and commit;
- SHA-256 of every artifact;
- selected firmware permission profile;
- Raspberry Pi model and board revision, excluding unique serial identifiers;
- Raspberry Pi OS release and image identity;
- kernel version;
- `/dev/vcio` and `/dev/vcio_gencmd` presence, ownership, and modes;
- install, upgrade, downgrade, rollback, remove, and purge results;
- `READY=1`, shutdown, exit-code, and restart-backoff results;
- functional and metric-conformance results;
- RSS, idle CPU, readiness latency, snapshot p95, and one-hour soak results;
- overall pass/fail result and any deviations.

Pi 3B+, Pi 4B, and Pi 5 observations may be recorded using the same fields, but remain non-blocking and must not receive claims stronger than their evidence.

