# Joy Pi Health v0.1 Requirements

**Status:** Frozen and approved  
**Applies to:** Joy Pi Health v0.1  
**Authority:** Normative for product purpose, scope, metric semantics, operational constraints, compatibility claims, and release acceptance.

The exact public protocol is defined by the [v0.1 HTTP API](http-api-v0.1.md). The implementation design is defined by the [v0.1 architecture](architecture-v0.1.md). If these normative documents genuinely conflict, the conflict must be resolved explicitly rather than interpreted silently.

## 1. Purpose and boundary

Joy Pi Health is a lightweight system-monitoring microservice that runs on a Raspberry Pi and exposes information about the Raspberry Pi host itself.

The term **health** refers exclusively to Raspberry Pi system health and observability. The project:

- has nothing to do with human health or medical data;
- is unrelated to the Joy-Pi electronics kit;
- is independent from HomeDNS;
- may later be observed by HomeDNS or used beside it during DNS benchmarks, but must never depend on HomeDNS.

### 1.1 v0.1 purpose

v0.1 provides a single synchronous, read-only operation that returns a current combined snapshot of the host metrics defined below. It is intended for local-machine use and explicit trusted-private-LAN use.

### 1.2 In scope

- Current generic Linux host metrics.
- Current Raspberry Pi-specific thermal and power-condition metrics.
- Partial snapshots when some observations are unavailable.
- Lightweight foreground service operation.
- Managed systemd installation and rootless manual execution.
- A stable, inspectable HTTP/JSON consumer contract.

### 1.3 Explicitly deferred

- Historical storage, time-series aggregation, retention, and trends.
- Push delivery, subscriptions, streaming, alerts, and dashboards.
- Prometheus or other alternate metric protocols.
- Service-health monitoring of arbitrary applications.
- HomeDNS integration or dependency.
- Authentication, authorization, and TLS termination.
- Configuration files and dynamic reload.
- IPv6, Unix sockets, and hostname-based listener addresses.
- Containers, Snap, Flatpak, package repositories, fleet management, and self-update.
- Additional HTTP operations, administrative routes, readiness routes, and profiling routes.

## 2. Product constraints

The service must be:

- lightweight enough for a Raspberry Pi 3B;
- low overhead at idle and during snapshot requests;
- clean and maintainable;
- straightforward to test without requiring a Pi for every development iteration;
- read-only with respect to the observed host;
- runnable without root;
- independent of HomeDNS and other application services;
- reliable under partial platform failure;
- bounded in memory, concurrency, work, and response size.

The runtime must not require elevated capabilities, write host configuration, or mutate the metrics it observes. A managed installation may perform one explicit administrative setup step.

## 3. Target platform and compatibility

### 3.1 Operating system

The v0.1 target is 64-bit Raspberry Pi OS Lite based on Debian 13 (Trixie).

### 3.2 Hardware claims

- Raspberry Pi 3B is the required reference device and the sole release-gating hardware model for v0.1.
- The full functional, metric-conformance, operational, packaging, and performance suite must pass on a real Pi 3B.
- Pi 3B+, Pi 4B, and Pi 5 are intended members of the supported hardware family.
- Real-device validation on Pi 3B+, 4B, and 5 is non-blocking for the initial release.
- Results for those devices must be recorded when available.
- No model may receive a compatibility claim stronger than the evidence recorded for it.
- A later release may promote additional models to release-gating status once they are part of the regular test environment.

## 4. Metric catalog

All metrics describe the observed host. Values are current-cycle observations unless their semantics explicitly describe an interval or cumulative counter. Unavailable values must be represented as `null` with deterministic issues; zero and `false` are always valid observations and never failure sentinels.

### 4.1 Generic host metrics

#### CPU

- **Utilization:** host-wide non-idle CPU utilization over a trailing interval of approximately one second, expressed as a percentage from 0 through 100 and normalized across all logical CPUs. Idle and non-executing wait time are not counted as busy work. The request must not wait one second to produce this value.
- **Logical CPU count:** number of logical processors online for the observation.

CPU utilization is unavailable during initial sampling, after a counter reset or reboot, after a topology change, when the observation interval or publication age is invalid, or when the underlying counters cannot be observed safely.

#### Load average

- One-minute load average.
- Five-minute load average.
- Fifteen-minute load average.

These are the host operating system's conventional dimensionless load averages, not percentages.

#### Memory

- Total usable memory in bytes.
- Available memory in bytes, using the operating system's estimate of memory available without swapping.
- Used memory in bytes, defined as `total - available`.

The relationship must be internally consistent. Values must not be fabricated from a different semantic when the required source is absent.

#### Root-filesystem storage

- Total bytes for the filesystem containing `/`.
- Bytes available to the unprivileged service account.
- Used bytes, defined as `total - available`.

The metric describes the root filesystem rather than the physical capacity of a disk device.

#### Uptime

- Non-negative elapsed seconds since the current host boot.

Uptime is a duration, not a wall-clock boot timestamp.

#### Networking

For every included external-capable interface:

- Interface name.
- Current operational state.
- Cumulative received bytes since the interface or host counter epoch.
- Cumulative transmitted bytes since the interface or host counter epoch.

Inclusion is based on kernel topology rather than interface naming conventions:

- Include physical Ethernet, Wi-Fi, and external-capable USB network interfaces.
- Include bond, bridge, and VLAN interfaces when their lower-device path is external-capable.
- Exclude loopback and internal-only virtual interfaces.
- Include eligible interfaces even when they are down.
- Do not broaden the result set when topology classification fails.
- Cap processing at 64 interfaces; exceeding the cap makes the networking group unavailable.

RX and TX byte values are exact, non-negative cumulative counters and may exceed JavaScript's exactly representable integer range.

### 4.2 Raspberry Pi-specific metrics

#### Primary SoC temperature

- Primary SoC/CPU thermal-zone temperature in degrees Celsius.

#### Thermal throttling

- Whether thermal throttling is active at observation time.
- Whether thermal throttling has occurred at least once since boot.

#### Undervoltage

- Whether undervoltage is active at observation time.
- Whether undervoltage has occurred at least once since boot.

The four throttling and undervoltage booleans must come from one coherent platform observation. Failure of that observation makes all four unavailable together. Temperature remains independently observable.

Missing Raspberry Pi-specific capability, inaccessible board data, or unsupported firmware behavior must not fail unrelated generic metrics or the service itself.

## 5. Snapshot semantics

- v0.1 exposes one combined snapshot; category-specific operations are unnecessary.
- The snapshot identifies schema version `1.0`.
- `observed_at` is an RFC 3339 UTC timestamp using `Z` and represents completion of collection and validation, immediately before encoding.
- Host identity is the operating-system hostname.
- A snapshot may be complete or partial.
- A partial snapshot contains every successfully observed current-cycle value plus `null` and an issue for each unavailable coherent unit.
- Previous-cycle values must never be substituted or silently mixed with current values.
- If no contracted metric leaf is available, no successful snapshot is returned.
- Internal defects invalidate the entire collection cycle and can never be represented as a metric issue.

The only metric-level issue codes are:

- `unsupported`
- `permission_denied`
- `temporarily_unavailable`

Request-level error codes are:

- `invalid_request`
- `temporarily_unavailable`
- `internal_error`

Exact field names and wire behavior are normative in the [HTTP API](http-api-v0.1.md).

## 6. Configuration and exposure

Configuration sources, in descending precedence, are:

1. command line;
2. effective process environment;
3. built-in defaults.

v0.1 has no application configuration file.

Required settings:

| CLI | Environment | Default |
|---|---|---|
| `--listen-address` | `JOY_PI_HEALTH_LISTEN_ADDRESS` | `127.0.0.1` |
| `--listen-port` | `JOY_PI_HEALTH_LISTEN_PORT` | `8080` |
| `--allow-non-loopback` | `JOY_PI_HEALTH_ALLOW_NON_LOOPBACK` | `false` |
| `--log-level` | `JOY_PI_HEALTH_LOG_LEVEL` | `info` |

Requirements:

- Listener addresses are canonical IPv4 literals only.
- Non-loopback and wildcard binding require explicit acknowledgement.
- `0.0.0.0` is the supported wildcard representation.
- Hostnames, CIDR notation, multicast, broadcast, IPv6, and port zero are invalid.
- Port range is 1 through 65535.
- Repeating a CLI option is invalid.
- Each recognized environment key is evaluated once from the effective environment; no duplicate-environment detection is required.
- Unknown variables beginning with `JOY_PI_HEALTH_` are invalid.
- Unknown variables outside that prefix are ignored.
- Empty supplied values are invalid and do not fall through.
- Valid log levels are exactly lowercase `debug`, `info`, `warn`, and `error`.
- Configuration is fully validated and made immutable before readiness or runtime activity.
- `--help` exits successfully without starting the service.
- Configuration errors write a bounded diagnostic to stderr and exit with status 2.
- Operational startup failures exit with status 1.

## 7. Security and operational behavior

- Listen on `127.0.0.1:8080` by default.
- Port 8080 is a configurable conventional HTTP port, not an IANA assignment for Joy Pi Health, and may conflict with another local service.
- Non-loopback use is an explicit opt-in intended for a trusted private network.
- v0.1 has no authentication or TLS.
- Run as an unprivileged account and never require root at runtime.
- Observe the host read-only.
- Run in the foreground without daemonization or a PID file.
- Start automatically at boot for a managed installation.
- Explicitly notify systemd when ready; readiness is distinct from process creation.
- Stop accepting new work during shutdown and exit gracefully within two seconds.
- Restart unexpected failures with bounded backoff.
- A clean operator stop must remain stopped.
- Invalid configuration must fail before readiness and must not enter a restart loop.
- Write operational logs to stderr; create no service-owned log files.
- Avoid routine successful-request access logging and repeated degradation log spam.
- Expose no separate liveness or readiness HTTP operation in v0.1; service process state and the snapshot operation provide the applicable external evidence.

## 8. Installation and upgrade

- Provide a managed ARM64 Debian package and a script-free portable ARM64 archive.
- Use the same statically linked pure-Go executable in both.
- Managed installation may use one explicit privileged package-install step.
- Managed runtime uses a dedicated unprivileged service identity.
- Rootless manual execution remains possible from the portable artifact.
- Create no persistent application state and require no application runtime directory.
- Preserve administrator-owned systemd configuration across upgrades and downgrades.
- Restart on upgrade only when the service was running; an intentionally stopped service remains stopped.
- Rollback uses package-manager installation of a retained older package; no self-update or private binary backup is required.
- Removal stops and disables the service and removes package-owned integration without deleting administrator-owned drop-ins or system journal data.
- Publish release version, source revision, target, toolchain, dependency/license inventory, and SHA-256 integrity information.

## 9. Release acceptance

### 9.1 Functional and contract acceptance

- All required metric fields and semantics match this document and the HTTP contract.
- Complete, partial, total-unavailable, invalid-request, overload, and internal-error behavior conform exactly.
- All unavailable values are null and deterministically associated with allowed issues.
- Zero and false remain distinguishable from unavailability.
- No previous-cycle value enters a new snapshot.
- Exact large counters, including values above `2^53`, remain unquoted decimal JSON integers.
- Unknown routes, methods, bodies, queries, and excessive headers follow the frozen HTTP mapping.

### 9.2 Metric correctness

- Compare production observations with independent operating-system or Raspberry Pi reference observations under controlled conditions.
- Validate arithmetic relationships and units.
- Exercise counter reset, reboot, topology change, interface disappearance, malformed data, and board-data failure.
- Validate current and since-boot Raspberry Pi flag semantics independently.

### 9.3 Platform and hardware acceptance

- The exact release candidate must pass the complete suite on a real Pi 3B running the supported Trixie image.
- The final hardened systemd unit and firmware-device permissions are part of that test, not a relaxed test configuration.
- Verify actual `/dev/vcio` and `/dev/vcio_gencmd` presence, ownership, DAC permissions, ioctl access, and device confinement.
- Use the approved service-specific ACL fallback if supplementary `video` access plus systemd device policy cannot provide narrow access.
- Failure of both unprivileged permission mechanisms blocks release.
- Pi 3B+, 4B, and 5 results are non-blocking and evidence-limited for v0.1.

### 9.4 Performance gates on Pi 3B

Under the frozen reproducible measurement conditions:

- Resident memory: no more than 25 MiB RSS.
- Idle CPU: no more than 0.5% of one core.
- Startup to readiness: no more than one second in every acceptance run.
- Snapshot response latency: p95 no more than 100 ms under the defined normal workload of one request per second.
- One-hour soak: no sustained goroutine, memory, or resource growth.

Architecture engineering targets provide additional headroom but do not replace these gates.

### 9.5 Lifecycle and packaging acceptance

- Invalid configuration fails before readiness.
- Readiness is explicitly reported only after the listener and required components are ready.
- Graceful shutdown completes within two seconds.
- Unexpected failure restarts with backoff.
- Configuration failure and operator stop do not restart.
- Fresh install, boot start, running upgrade, stopped upgrade, downgrade, removal, and purge behavior match the contract.
- Operator configuration survives upgrades.
- Rootless manual execution works without privilege escalation.

### 9.6 Blocking versus diagnostic evidence

Release-blocking:

- Deterministic unit, integration, contract, Linux platform, and package checks.
- Full real Pi 3B functional, metric, operational, packaging, and performance acceptance.

Non-blocking but recorded:

- Pi 3B+, Pi 4B, and Pi 5 real-device results.
- Additional diagnostic profiles and benchmarks that do not define a frozen gate.

## 10. Change control

This document is frozen. A substantive change requires explicit approval and corresponding review of the HTTP and architecture documents. Future releases receive self-contained versioned requirements documents. Corrections to this file must be labelled as v0.1 errata and must not silently redefine shipped behavior.
