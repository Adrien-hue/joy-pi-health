# Joy Pi Health

Joy Pi Health is a lightweight, read-only system-health and observability microservice for Raspberry Pi computers. In this project, **health** means Raspberry Pi host health only: this is not medical software and is unrelated to the Joy-Pi electronics kit.

The service will expose one current snapshot of CPU, load, memory, root-filesystem storage, uptime, networking, SoC temperature, thermal throttling, and undervoltage state. It is independent from HomeDNS, although HomeDNS may consume its metrics in the future.

## Project status

The v0.1 requirements and architecture are frozen and approved. The service now exposes real generic Linux host observations, trailing CPU utilization after its initial sampling interval, and Raspberry Pi SoC temperature and firmware health observations through the public HTTP/JSON snapshot contract. Debian and portable release packaging is implemented, but no release has been published and the real Pi 3B+ release gates have not yet been claimed.

Canonical repository and Go module path:

`github.com/Adrien-hue/joy-pi-health`

## Normative documentation

- [v0.1 requirements](docs/requirements-v0.1.md) define product scope, metric semantics, constraints, and release acceptance.
- [v0.1 HTTP API](docs/http-api-v0.1.md) defines the consumer-visible HTTP/JSON contract.
- [v0.1 architecture](docs/architecture-v0.1.md) defines the internal design and repository boundaries.

These documents are frozen for v0.1. This README is an onboarding summary, not a substitute for them.

## Supported target

The release-gating target is a real Raspberry Pi 3B+ running 64-bit Raspberry Pi OS Lite based on Debian 13 (Trixie). Raspberry Pi 3B, Pi 4B, and Pi 5 are intended members of the supported hardware family, but v0.1 claims for those models must not exceed the validation evidence available for each model.

## Security posture

Joy Pi Health is read-only and runs unprivileged. It listens on `127.0.0.1:8080` by default. Non-loopback exposure requires explicit operator acknowledgement and is intended only for trusted private networks. v0.1 provides neither authentication nor TLS.

## Development

Development requires Go 1.27.1. From the repository root, the supported foundation checks are:

```text
go version
go list -m
go mod tidy -diff
gofmt -l cmd internal
go test ./...
go vet ./...
go build ./...
go run ./cmd/joy-pi-health --help
git diff --check
git status --short
```

`go version` must report Go 1.27.1, and the module-tidiness and formatting checks must produce no output. On Linux, the executable currently collects hostname, uptime, logical CPU count, trailing CPU utilization, load averages, memory, root-filesystem storage, and network observations.

The implemented command-line configuration can be inspected with:

```text
go run ./cmd/joy-pi-health --help
```

To run the listener in the foreground:

```text
go run ./cmd/joy-pi-health
```

The frozen operation is available at:

```text
curl.exe -i http://127.0.0.1:8080/v1/snapshot
```

On the supported Linux target, CPU utilization is initially `null` while the observer establishes its baseline and becomes available after a valid trailing interval. Raspberry Pi temperature is read from the typed `cpu-thermal` kernel thermal zone. The four throttling and undervoltage fields depend on unprivileged access to `/dev/vcio_gencmd` or `/dev/vcio`; without suitable device access they remain `null` with the applicable issue while unrelated metrics remain available. Non-Linux builds are retained for development compatibility but are not supported runtime targets.

When systemd provides `NOTIFY_SOCKET`, the service sends `READY=1` after listener binding, firmware-executor initialization, the initial CPU baseline attempt, HTTP admission setup, and a minimal successful snapshot-capability probe. Manual execution does not require systemd. Expected metric degradation is logged to stderr on first occurrence, at most once every five minutes while unchanged, and once when it clears; successful requests are not logged. Stop the service with Ctrl+C.

Compile the production code for the ARMv8.0 Linux/ARM64 target without executing it:

```text
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 go build ./...
```

From Windows PowerShell, run the same check in a child process so the cross-build environment does not persist:

```powershell
powershell -NoProfile -Command '$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="arm64"; $env:GOARM64="v8.0"; go build ./...'
```

## Packaging

On Debian 13 (Trixie), build deterministic ARM64 Debian and portable artifacts from a clean committed worktree with:

```text
sh debian/build-release.sh
sh test/release/validate-artifacts.sh dist
```

The default managed permission profile combines the service's supplementary `video` credential with a closed systemd device policy for `/dev/vcio` and `/dev/vcio_gencmd`. The service remains unprivileged. An ACL build profile exists only as the frozen fallback for real Pi 3B+ validation:

```text
sh debian/build-release.sh --firmware-access acl
```

Artifacts are written beneath `dist/` and are not committed. Install the generated Debian package through the package manager:

```text
sudo apt install ./dist/joy-pi-health_0.1.0-1_arm64.deb
```

The package installs a managed systemd service at `/usr/bin/joy-pi-health`, creates the locked `_joy-pi-health` service identity, enables boot start, and starts subject to Debian service policy. Administrator overrides belong under `/etc/systemd/system/joy-pi-health.service.d/` and are preserved across package lifecycle operations. The portable tar archive contains no installation script and can be run without root or systemd.

## Continuous validation

GitHub Actions runs repository quality checks on Linux and Windows, performs Linux/ARM64 compile-only validation, and builds the release artifacts twice inside a pinned Debian Trixie environment. The Debian job calls the canonical release builder and validates the resulting package, systemd unit, archive, metadata, checksums, and reproducibility.

CI outputs are retained only as labelled validation artifacts. They are not official releases and do not establish Raspberry Pi compatibility. The exact validated bytes must still pass the real Pi 3B+ release gates described in the [release-validation and handoff procedure](docs/release-validation.md).

## Raspberry Pi 3B+ release acceptance

The operator-assisted Class C procedure and evidence rules are defined in the [release-validation runbook](docs/release-validation.md). The committed harness must be frozen before physical measurements begin. It does not download CI artifacts, rebuild the service, install packages, invoke `sudo`, change firmware-device permissions, or decide that a release passed.

Run its read-only stages on the Pi with explicit artifact and evidence directories:

```text
sh test/release/pi3bplus/verify-readonly.sh verify-artifacts ARTIFACT_DIR EVIDENCE_DIR EXPECTED_COMMIT [video|acl]
sh test/release/pi3bplus/verify-readonly.sh inspect-platform EVIDENCE_DIR
sh test/release/pi3bplus/verify-readonly.sh rootless ARTIFACT_DIR EVIDENCE_DIR
sh test/release/pi3bplus/verify-readonly.sh managed EVIDENCE_DIR
```

The fixed performance procedures are:

```text
sh test/release/pi3bplus/measure-performance.sh cpu EVIDENCE_DIR http://127.0.0.1:8080/v1/snapshot --allow-load
sh test/release/pi3bplus/measure-performance.sh rss EVIDENCE_DIR
sh test/release/pi3bplus/measure-performance.sh idle-cpu EVIDENCE_DIR
sh test/release/pi3bplus/measure-performance.sh readiness EVIDENCE_DIR
sh test/release/pi3bplus/measure-performance.sh latency EVIDENCE_DIR
sh test/release/pi3bplus/measure-performance.sh soak EVIDENCE_DIR
```

The CPU command starts a bounded `stress-ng` workload only with the explicit `--allow-load` acknowledgement. The readiness command requires an operator already authorized to start and stop the managed service. Package installation, systemd test overrides, firmware confinement probes, lifecycle operations, and final evidence review remain explicit operator steps.

When no older official package exists, create only the labelled, non-release lifecycle fixture described by the runbook:

```text
sh test/release/pi3bplus/derive-lifecycle-fixture.sh CANDIDATE_DEB TEMPORARY_OUTPUT_DIRECTORY
```

Do not claim Pi 3B+ acceptance until every release-blocking Class A, Class B, and Class C gate has genuine reviewed evidence.

## License

Joy Pi Health is licensed under the [MIT License](LICENSE).
