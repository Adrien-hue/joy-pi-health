# Joy Pi Health

Joy Pi Health is a lightweight, read-only system-health and observability microservice for Raspberry Pi computers. In this project, **health** means Raspberry Pi host health only: this is not medical software and is unrelated to the Joy-Pi electronics kit.

The service will expose one current snapshot of CPU, load, memory, root-filesystem storage, uptime, networking, SoC temperature, thermal throttling, and undervoltage state. It is independent from HomeDNS, although HomeDNS may consume its metrics in the future.

## Project status

The v0.1 requirements and architecture are frozen and approved. The service now exposes real generic Linux host observations, trailing CPU utilization after its initial sampling interval, and Raspberry Pi SoC temperature and firmware health observations through the public HTTP/JSON snapshot contract. There is no release artifact yet.

Canonical repository and Go module path:

`github.com/Adrien-hue/joy-pi-health`

## Normative documentation

- [v0.1 requirements](docs/requirements-v0.1.md) define product scope, metric semantics, constraints, and release acceptance.
- [v0.1 HTTP API](docs/http-api-v0.1.md) defines the consumer-visible HTTP/JSON contract.
- [v0.1 architecture](docs/architecture-v0.1.md) defines the internal design and repository boundaries.

These documents are frozen for v0.1. This README is an onboarding summary, not a substitute for them.

## Supported target

The release-gating target is a real Raspberry Pi 3B running 64-bit Raspberry Pi OS Lite based on Debian 13 (Trixie). Raspberry Pi 3B+, 4B, and 5 are intended members of the supported hardware family, but v0.1 claims for those models must not exceed the validation evidence available for each model.

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

Compile the production code for the Raspberry Pi 3B architecture without executing it:

```text
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 go build ./...
```

From Windows PowerShell, run the same check in a child process so the cross-build environment does not persist:

```powershell
powershell -NoProfile -Command '$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="arm64"; $env:GOARM64="v8.0"; go build ./...'
```

## License

No project license has been selected or added yet.
