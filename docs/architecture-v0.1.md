# Joy Pi Health v0.1 Architecture

**Status:** Frozen and approved  
**Applies to:** Joy Pi Health v0.1  
**Authority:** Normative for internal architecture, dependency direction, runtime behavior, packaging strategy, testability boundaries, and repository layout.

Product scope and acceptance are defined by the [v0.1 requirements](requirements-v0.1.md). Consumer-visible behavior is defined by the [v0.1 HTTP API](http-api-v0.1.md). Architecture is subordinate to those contracts. Genuine conflicts require explicit baseline correction.

## 1. Language and runtime

- Implement the service in Go.
- Produce one native Linux/ARM64 executable.
- Target the ARMv8.0/AArch64 baseline supported by Pi 3B.
- Use a pure-Go runtime graph with cgo disabled and no external language runtime.
- Pin the exact Go patch release for reproducible release builds.
- Preserve enough symbols for useful fatal diagnostics unless Pi 3B measurements justify a change.

Go provides a small deployable artifact, direct Linux integration, efficient blocking I/O, standard HTTP/JSON support, and simple bounded concurrency without a framework.

## 2. Dependency policy

- Standard library first; no web, configuration, logging, dependency-injection, or application framework.
- Runtime dependencies must be pure Go. Native system libraries and cgo require an explicit architecture revision.
- A third-party package is justified only when a small local implementation would be riskier, less maintainable, or incorrect and the package has a materially better reviewed solution.
- Review the complete transitive graph, but do not use an arbitrary numeric dependency cap.
- Pin direct and transitive versions, retain checksum verification, and pin the toolchain.
- Do not vendor by default.
- Two clean release builds must produce the same executable hash.
- Accept permissive licenses by default; exceptions require explicit review.
- Use reachability-aware vulnerability review and deliberate dependency updates rather than automatic unreviewed upgrades.

No individual third-party library is selected for v0.1. The planned baseline requires none.

## 3. Process and concurrency

- One foreground OS process; no helper daemon or production subprocess.
- Main/root lifecycle owns all long-lived work.
- Use ordinary blocking I/O in bounded goroutines; no asynchronous event loop.
- Long-lived work is limited to transport-required activity, one CPU observer, one firmware executor, and minimal signal/lifecycle handling.
- At most one collection cycle is active.
- Overlapping admitted requests join that cycle and write their responses independently.
- At most 16 connections and 8 admitted requests.
- Excess work is rejected promptly with `503`; there is no request queue.
- No worker pool, actor system, event bus, general cache, or history.
- All goroutines have explicit ownership and termination behavior.

The firmware executor is a narrow exception to the original no-worker rule: it isolates one potentially non-cancellable ioctl, has no queue, performs no periodic work, and can retain at most one blocked transaction.

## 4. Transport and wire encoding

- Plain HTTP/1.1 with UTF-8 JSON.
- Standard-library server and JSON encoder.
- JSON null plus issues preserve partial availability.
- Exact cumulative counters are emitted as unquoted decimal integers.
- No CBOR, MessagePack, Protocol Buffers, custom TCP protocol, Unix-socket protocol, compression, TLS, or streaming in v0.1.

HTTP/JSON is preferred for interoperability, manual inspection, HomeDNS compatibility, and low operational complexity. Compact encodings do not justify their dependencies and debugging cost for one small snapshot.

## 5. HTTP operation mapping

The architecture implements exactly the [public HTTP contract](http-api-v0.1.md):

- `GET /v1/snapshot`.
- Default `127.0.0.1:8080`.
- Complete and partial snapshots use `200`.
- Invalid requests, routing, overload, total unavailability, and defects use the frozen status mapping.
- Responses are encoded before headers.
- Keep-alive is supported within fixed connection and idle bounds.
- No other endpoint exists.

Port 8080 is conventional and configurable, not assigned specifically to Joy Pi Health.

## 6. Generic metric collection

- Collect generic metrics on demand for each collection cycle.
- Use direct read-only Linux/kernel interfaces through pure Go.
- Do not invoke external commands in production collection.
- Keep a compile-time fixed collector set; no registry or plugin mechanism.
- Collect sequentially in this domain order:
  1. host identity and uptime;
  2. CPU topology and load;
  3. memory;
  4. root storage;
  5. Raspberry Pi metrics;
  6. networking;
  7. copy and validate the published CPU utilization observation.
- Each collector returns typed domain observations and does not emit JSON, start goroutines, retry, or retain history.
- All reads, input sizes, traversal, interface count, and execution time are bounded.

Network classification follows kernel topology. It includes external-capable physical and layered interfaces, excludes loopback and internal-only virtual devices, handles cycles, and fails closed rather than broadening results. More than 64 interfaces makes the network group unavailable.

## 7. Trailing one-second CPU observer

- One fixed one-hertz observer takes an immediate baseline and then samples on monotonic one-second targets without drift.
- Retain only previous cumulative busy/total counters, monotonic sample time, topology signature, boot/counter epoch, and latest immutable publication.
- Utilization is `100 × busy_delta / total_delta`, normalized from 0 through 100 for the host.
- Valid sample interval: 0.9 through 1.1 seconds.
- Maximum publication age when copied: 1.25 seconds.
- Before two valid samples, utilization is unavailable; the service may already be ready.
- Failed/delayed reads, decreasing counters, reboot/uptime regression, zero/inconsistent delta, or topology change invalidate publication and re-baseline at the next regular tick.
- Do not retry immediately.
- Use monotonic time for intervals, age, and scheduling; wall time is irrelevant.
- Publish a complete immutable observation atomically.
- Expected read failure yields a partial snapshot. Observer panic, invariant failure, or unexpected exit is process-fatal.

The exact Go clock, ticker, and synchronization types are implementation details.

## 8. Snapshot coordination and consistency

- The service owns a collection cycle independently of any single request.
- The first admitted request starts a cycle; overlapping requests join it.
- Request cancellation detaches that requester but does not cancel the shared cycle.
- An orphaned cycle continues within its 500 ms bound; shutdown may cancel it.
- Collectors remain sequential unless Pi 3B profiling proves a baseline revision necessary.
- Each domain has at most 100 ms and cannot exceed the remaining cycle time.
- When the cycle budget is exhausted, skip later collectors and mark their fields unavailable.
- Discard any result that completes after its domain deadline.
- Use values only from the current cycle; no stale substitution.
- Copy CPU publication last and validate topology and age against monotonic cycle completion.
- Capture wall-clock `observed_at` after observation validation and immediately before encoding.
- Finalize and validate one immutable logical snapshot, then encode it once before headers.
- Joined responses share that immutable snapshot and encoded bytes.
- Clear the active-cycle slot after encoding, allowing a new cycle while prior responses transmit.

## 9. Raspberry Pi integration

### 9.1 Temperature

- Discover the typed primary `cpu-thermal` kernel thermal zone rather than relying only on its numeric index.
- Read its millidegree value and expose degrees Celsius.
- Do not fall back to firmware or `vcgencmd`; temperature fails independently.

### 9.2 Throttling and undervoltage

Read one firmware `get_throttled` mask and derive:

- bit 0: undervoltage active;
- bit 2: thermal throttling active;
- bit 16: undervoltage occurred since boot;
- bit 18: thermal throttling occurred since boot.

Source order:

1. restricted firmware command device when present and accessible;
2. Trixie's general mailbox using structured `GET_THROTTLED`;
3. unavailable if neither works.

Do not use `vcgencmd`, shell commands, kernel-log inference, or partial hwmon reconstruction. All four flags share one outcome.

### 9.3 Firmware executor

- One service-owned non-periodic single-flight executor isolates the kernel ioctl.
- Submit only when idle; do not queue.
- A busy executor immediately makes the four fields temporarily unavailable.
- Wait only for the remaining domain deadline.
- Tag results to their originating cycle and discard late results.
- Shutdown requests executor termination but never exceeds the service's two-second bound; process exit safely terminates a still-blocked read-only operation.

Temperature and firmware transaction sources have separate narrow test boundaries.

## 10. Failure and partial-result architecture

Collectors produce a closed outcome:

- present validated value;
- unavailable for an expected acquisition reason;
- explicit internal defect.

Expected reasons map deterministically:

| Outcome | Public issue |
|---|---|
| Capability absent or explicitly unsupported | `unsupported` |
| Access denied | `permission_denied` |
| Timeout, busy source, transient I/O, disappearance, or safely rejected malformed platform data | `temporarily_unavailable` |

Rules:

- Fail the smallest independently coherent unit.
- Derived metrics fail with required inputs.
- Previous-cycle values, fabricated zeros, and fabricated false values are forbidden.
- Collectors return outcomes but do not log or construct public issues.
- The coordinator owns issue mapping, ordering, and degradation reporting.
- Malformed external data rejected by a correct validator is expected acquisition failure.
- An impossible state after validation is an internal defect.

Any explicit defect, invariant failure, panic, shared-state corruption, observer/executor lifecycle defect, builder defect, or encoding defect invalidates the entire cycle. Stop safe work, discard collected metrics, return request-level `500 internal_error` before headers when possible, close admission, and begin controlled shutdown. Never emit `issues[].code = internal_error`.

Client cancellation, disconnect, slow-client timeout, and socket read/write failure remain request-local and non-fatal.

Log expected degradation on state transitions, material reason changes, bounded reminders, and recovery. Internal defects bypass degradation suppression and produce a fatal diagnostic. Low-level details remain private logs.

## 11. Configuration architecture

The configuration surface is exactly:

| CLI | Environment | Default |
|---|---|---|
| `--listen-address` | `JOY_PI_HEALTH_LISTEN_ADDRESS` | `127.0.0.1` |
| `--listen-port` | `JOY_PI_HEALTH_LISTEN_PORT` | `8080` |
| `--allow-non-loopback` | `JOY_PI_HEALTH_ALLOW_NON_LOOPBACK` | `false` |
| `--log-level` | `JOY_PI_HEALTH_LOG_LEVEL` | `info` |

- Per-setting precedence is CLI, effective environment, then default.
- Repeated CLI options and unknown CLI options fail.
- Read each recognized key once from the effective environment; do not implement environment-duplicate detection.
- Unknown exact-prefix `JOY_PI_HEALTH_` keys fail.
- Empty supplied values fail.
- Accept canonical IPv4 literals only, with a separate acknowledgement for non-loopback and wildcard binding.
- Accept port 1–65535 and log levels `debug`, `info`, `warn`, `error`.
- Resolve and validate once before any runtime activity; pass one immutable effective value.
- No configuration file, reload, profile, nested tree, or offline validation mode.
- Configuration failures emit bounded stderr diagnostics and exit 2; operational startup failures exit 1; help exits 0.

## 12. Packaging and service supervision

### 12.1 Artifacts and layout

- Primary managed artifact: ARM64 Debian package named `joy-pi-health`.
- Secondary rootless artifact: script-free ARM64 tar archive with the same executable and licenses.
- Executable path: `/usr/bin/joy-pi-health`.
- Package files are root-owned and not writable by the service.
- No application state, cache, PID, socket, spool, log, or runtime directory.
- Publish SHA-256 manifests and release metadata; signing and package repositories are deferred.

### 12.2 Identity and permissions

- Dedicated locked system user and group `_joy-pi-health`, with dynamic IDs, no home, and no login shell.
- Retain the identity after removal to avoid unsafe ID reuse.
- Preferred mailbox access: supplementary `video` credential through the unit plus closed systemd device policy allowing only `/dev/vcio`, `/dev/vcio_gencmd` when present, and standard pseudo-devices.
- Grant no capabilities, root, unrestricted devices, setuid code, or global ownership replacement.
- Validate actual DAC/group/device behavior on the release-gating Pi 3B Trixie system.
- If the preferred combination cannot provide narrow ioctl access, use a service-specific ACL that survives device recreation without disrupting OS permissions.
- Failure of both unprivileged mechanisms blocks release.

### 12.3 systemd

- Run directly in the foreground with intentional `Type=notify`.
- Explicitly notify readiness only after configuration, listener binding, initial CPU baseline, firmware-executor readiness, and request admission.
- Readiness is distinct from process start and does not wait for the second CPU sample.
- Implement notification minimally without a framework added solely for `sd_notify`.
- Journal captures stdout/stderr; application logs use stderr.
- No watchdog in v0.1.
- Restart unexpected failures with exponential backoff from 1 second through five steps to 30 seconds.
- Configuration exit 2, clean exit, and explicit operator stop do not restart.
- Continue unexpected-failure attempts at the capped delay.
- Enforce the two-second shutdown bound.

### 12.4 Lifecycle

- Fresh managed installation creates identity, installs permission integration, enables boot start, and starts with safe defaults.
- Administrator-owned systemd drop-ins provide CLI/environment overrides and survive upgrades.
- Upgrade restarts only an already-running service and preserves enabled/stopped state.
- Rollback installs a retained older package; no application backup or self-update.
- Removal stops/disables service, removes package-owned files and permissions, and retains administrator drop-ins, journal data, and the service identity.

Exact Debian metadata, maintainer code, unit directives, and file modes remain implementation details subject to acceptance.

## 13. Testability and fault injection

Substitute only nondeterministic boundaries:

- platform observations;
- wall and monotonic time and scheduled waits;
- firmware transactions;
- collector invocation at the coordinator boundary;
- readiness and fatal lifecycle notification;
- snapshot encoding at the outer boundary;
- log output capture.

Production composition always selects production implementations. No fault control is reachable through HTTP, CLI, environment, configuration, or a test route. Use no generic dependency-injection or mock framework.

Required deterministic coverage includes unavailable metrics, permission denial, malformed data, CPU reset/reboot/topology change, firmware timeout/busy/late response, explicit defects, panics, joined cycles, issue ordering, cancellation, and shutdown during active work. Use a manually advanced time source rather than sleeps where possible.

Test layers:

1. unit tests for parsing, arithmetic, validation, outcomes, and configuration;
2. integration tests for composed production logic with controlled sources;
3. HTTP/JSON contract and exact-number fixtures;
4. Linux/Raspberry Pi platform adapter tests;
5. Debian/systemd installation and lifecycle tests;
6. exact-release-candidate tests on the real Pi 3B.

Use package-local tests and `testdata`; reserve top-level `test/` for black-box, package, systemd, Pi, and release work. Prefer the standard library and deterministic lifecycle-completion assertions. Race and goroutine-profile/count checks are additional diagnostics.

## 14. Performance architecture

Frozen release gates:

- RSS no more than 25 MiB.
- Idle CPU no more than 0.5% of one Pi 3B core.
- Readiness no more than one second.
- Snapshot p95 no more than 100 ms.

Engineering targets:

- steady-state RSS no more than 20 MiB;
- idle CPU no more than 0.25%;
- readiness no more than 500 ms;
- snapshot p95 no more than 75 ms;
- encoded snapshot hard maximum 64 KiB.

Architecture rules:

- CPU observer is the only application-scheduled periodic wake-up.
- Build one logical snapshot and one encoded buffer per cycle.
- Share encoded bytes; avoid unnecessary copies.
- Eight retained maximum-size buffers account for 512 KiB of encoded snapshot data only, not request objects, HTTP/runtime buffers, or stacks.
- Keep connections, admission, interfaces, reads, traversal, tasks, and logs bounded.
- Perform no full snapshot, network enumeration, second CPU wait, or firmware transaction before readiness.
- Do not log successful requests by default or add asynchronous logging.
- Profile the exact production package externally and through test/benchmark harnesses; expose no profiling endpoint or production flag.

Do not introduce object pools, `sync.Pool`, custom allocators, arenas, unsafe conversions, GC tuning, hand-written JSON, collector parallelism, or specialized parsing until measured Pi 3B evidence proves it necessary and an approved revision permits it.

## 15. Repository and module boundaries

Canonical module:

`github.com/Adrien-hue/joy-pi-health`

One module produces one executable. All reusable implementation packages are internal.

```text
joy-pi-health/
├── go.mod
├── cmd/
│   └── joy-pi-health/
├── internal/
│   ├── app/
│   ├── config/
│   ├── httpapi/
│   ├── observe/
│   ├── platform/
│   └── snapshot/
├── debian/
├── docs/
└── test/
```

Responsibilities:

- `cmd/joy-pi-health`: thin process entry point.
- `internal/app`: production composition, startup, readiness, fatal coordination, and shutdown.
- `internal/config`: CLI/environment resolution and immutable validation.
- `internal/snapshot`: schema-facing types, validation, immutability, JSON, and size invariant.
- `internal/platform`: narrow Linux/Raspberry Pi I/O and real clocks.
- `internal/observe`: collectors, CPU observer, firmware executor, cycle coordinator, outcomes, issues, and degradation state.
- `internal/httpapi`: HTTP routing, admission, bounds, status mapping, and response writing.
- `debian/`: managed packaging and systemd assets.
- package-local `testdata`: owned fixtures.
- top-level `test/`: cross-package system and release testing only.

Dependency direction:

```text
cmd/joy-pi-health
        |
        v
   internal/app
     |----> internal/config
     |----> internal/httpapi ----> internal/snapshot
     |----> internal/observe ----> internal/snapshot
     |                |
     |                v
     `--------> internal/platform
```

- `cmd` imports only `app`.
- No production package imports `cmd` or `app`.
- `observe` does not import `httpapi`; `httpapi` does not import `observe`.
- `snapshot`, `config`, and `platform` do not depend on higher-level packages.
- Resolve shared ownership correctly; do not introduce broad `util`, `common`, `shared`, repository, or service-layer packages.

Individual files, concrete Go types/interfaces, constructors, synchronization primitives, parser code, test helpers, and packaging scripts belong to implementation planning.

## 16. Consistency and implementation validation

The architecture decisions are mutually consistent, including these deliberate reconciliations:

- Firmware execution is the single narrow non-periodic worker exception.
- A blocked ioctl is isolated from snapshot deadlines and bounded to one executor operation.
- Internal defects invalidate the full cycle and never become metric issues.
- `Type=notify` is lightweight and does not wait for a CPU utilization publication.
- Administrator systemd drop-ins configure the environment without adding an application configuration file.
- Encoded-buffer memory is only one part of the complete RSS budget.
- Sequential collection remains authoritative until real Pi 3B evidence justifies review.

The following are required validations with predetermined responses, not open architecture choices:

- Prove the worst-case schema encoding fits 64 KiB.
- Validate Pi 3B mailbox DAC and systemd device policy, using the approved ACL fallback if necessary.
- Measure gates and engineering targets using the exact package.
- Pin the concrete Go patch version.
- Choose implementation primitives within the frozen behavior.

No unresolved architecture decision blocks implementation planning.

## 17. Change control

### Approved v0.1 transport errata

The standard-library server retains ownership of failures that occur before handler dispatch. Its header-limit rejection may emit its native connection-closing `431` response rather than an application JSON envelope. HTTP also suppresses the body for unsupported `HEAD /v1/snapshot` responses; the status remains `405` with `Allow: GET`. No custom HTTP pre-parser or HEAD-specific transport is introduced for these cases.

This document is frozen. A material change—such as a new dependency, endpoint, background task, cache, process, protocol, public Go package, or privilege—requires explicit architecture review and corresponding requirements/API review where applicable. Future releases receive self-contained versioned architecture documents; v0.1 corrections must be labelled as errata.
