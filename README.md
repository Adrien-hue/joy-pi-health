# Joy Pi Health

Joy Pi Health is a lightweight, read-only system-health and observability microservice for Raspberry Pi computers. In this project, **health** means Raspberry Pi host health only: this is not medical software and is unrelated to the Joy-Pi electronics kit.

The service will expose one current snapshot of CPU, load, memory, root-filesystem storage, uptime, networking, SoC temperature, thermal throttling, and undervoltage state. It is independent from HomeDNS, although HomeDNS may consume its metrics in the future.

## Project status

The v0.1 requirements and architecture are frozen and approved. The configuration and foreground HTTP lifecycle foundations exist, but the public API and metric collection have not started. There is currently no functional monitoring service or release artifact.

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
go mod tidy -diff
gofmt -l cmd internal
go test ./...
go vet ./...
go build ./...
```

`go version` must report Go 1.27.1, and the module-tidiness and formatting checks must produce no output. The executable currently supports configuration and lifecycle development only.

The implemented command-line configuration can be inspected with:

```text
go run ./cmd/joy-pi-health --help
```

To run the pre-API listener in the foreground for lifecycle development:

```text
go run ./cmd/joy-pi-health
```

Stop it with Ctrl+C. The listener currently returns a non-normative `404 Not Found` for every request, including `/v1/snapshot`; no public API or metric response is implemented yet. Production systemd readiness notification also remains intentionally disabled until every frozen readiness gate exists.

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
