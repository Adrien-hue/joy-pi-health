# Joy Pi Health

Joy Pi Health is a lightweight, read-only system-health and observability microservice for Raspberry Pi computers. In this project, **health** means Raspberry Pi host health only: this is not medical software and is unrelated to the Joy-Pi electronics kit.

The service will expose one current snapshot of CPU, load, memory, root-filesystem storage, uptime, networking, SoC temperature, thermal throttling, and undervoltage state. It is independent from HomeDNS, although HomeDNS may consume its metrics in the future.

## Project status

The v0.1 requirements and architecture are frozen and approved. Product implementation has **not started yet**. There is currently no executable, build command, test command, package, or release artifact.

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

## Development and operation

No development, validation, installation, or execution commands exist yet. This section must be updated when implementation introduces supported commands; commands must not be invented in advance.

## License

No project license has been selected or added yet.
