# Joy Pi Health v0.1 HTTP API

**Status:** Frozen and approved  
**Applies to:** Joy Pi Health v0.1, snapshot schema `1.0`  
**Authority:** Normative for consumer-visible HTTP/1.1 and JSON behavior.

Metric meaning and release obligations are defined by the [v0.1 requirements](requirements-v0.1.md). Internal realization is defined by the [v0.1 architecture](architecture-v0.1.md). This document controls exact wire behavior; it cannot broaden product scope or weaken requirements.

## 1. Transport

- HTTP/1.1 over TCP.
- UTF-8 JSON.
- No TLS, authentication, compression, streaming, content negotiation, CBOR, or Protocol Buffers in v0.1.
- Default listener: `127.0.0.1:8080`.
- Port 8080 is a configurable conventional development/service HTTP port and IANA `http-alt`; it is not assigned to Joy Pi Health and may conflict with another service.

## 2. Snapshot operation

```http
GET /v1/snapshot HTTP/1.1
Host: 127.0.0.1:8080
```

The operation is parameterless, synchronous, read-only, and non-streaming.

### 2.1 Request rules

- Method must be `GET`.
- Path must be exactly `/v1/snapshot`.
- Query string must be empty.
- No request body is accepted.
- A positive `Content-Length` or any transfer encoding produces `400`, and the server closes the connection without consuming the body.
- `HEAD` and `OPTIONS` receive no special support.

### 2.2 Success

- Complete snapshot: `200 OK`.
- Partial snapshot with allowed issues: `200 OK`.
- `Content-Type: application/json`.
- `Cache-Control: no-store`.
- No `ETag` or other validator is required.
- A successful response is at most 64 KiB encoded.

### 2.3 Routing and request errors

| Condition | Status | Error code | Additional behavior |
|---|---:|---|---|
| Non-empty query, forbidden body, or otherwise invalid request | 400 | `invalid_request` | Close when a body was rejected without consumption |
| Unknown route | 404 | `invalid_request` | — |
| Unsupported method on `/v1/snapshot` | 405 | `invalid_request` | `Allow: GET` |
| Request headers exceed the limit | 431 when a response is possible | `invalid_request` | Connection may be closed |
| Admission capacity exhausted | 503 | `temporarily_unavailable` | Reject promptly; do not queue |
| Valid cycle with no useful metric leaf | 503 | `temporarily_unavailable` | No successful issues-only snapshot |
| Internal defect | 500 | `internal_error` | Invalidate cycle and begin controlled shutdown |

Request-level error envelopes are at most 2 KiB encoded.

### 2.4 Keep-alive and caching

- HTTP/1.1 persistent connections are supported.
- Client-requested connection close is honored.
- Responses are fully encoded before headers are committed.
- Slow clients are terminated at the write deadline and release their admission capacity.
- All successful snapshots are non-cacheable via `Cache-Control: no-store`.

## 3. Snapshot document

The v0.1 schema is represented by the following field hierarchy. Every metric value that may be unavailable is nullable.

```json
{
  "schema_version": "1.0",
  "observed_at": "2026-09-05T12:34:56.123Z",
  "host": {
    "hostname": "raspberrypi"
  },
  "cpu": {
    "utilization_percent": 12.5,
    "logical_cpu_count": 4
  },
  "load": {
    "one_minute": 0.12,
    "five_minutes": 0.18,
    "fifteen_minutes": 0.21
  },
  "memory": {
    "total_bytes": 1000000000,
    "available_bytes": 600000000,
    "used_bytes": 400000000
  },
  "root_filesystem": {
    "total_bytes": 32000000000,
    "available_bytes": 20000000000,
    "used_bytes": 12000000000
  },
  "uptime_seconds": 86400,
  "network": {
    "interfaces": [
      {
        "name": "eth0",
        "state": "up",
        "rx_bytes": 9007199254740993,
        "tx_bytes": 123456789
      }
    ]
  },
  "raspberry_pi": {
    "soc_temperature_celsius": 48.25,
    "thermal_throttling_active": false,
    "thermal_throttling_occurred_since_boot": false,
    "undervoltage_active": false,
    "undervoltage_occurred_since_boot": false
  },
  "issues": []
}
```

The example is illustrative. Field names, nesting, types, units, nullability, and issue rules are normative; example values and JSON whitespace are not.

### 3.1 Envelope

| Field | Type | Required | Meaning |
|---|---|---|---|
| `schema_version` | string | yes | Exactly `1.0` for this schema |
| `observed_at` | string | yes | RFC 3339 UTC timestamp using `Z`, captured after collection/validation and before encoding |
| `host.hostname` | string | yes | Operating-system hostname |
| `issues` | array | yes | Empty for a complete snapshot; deterministic entries for unavailable units in a partial snapshot |

A required-envelope acquisition failure prevents a successful snapshot. An expected hostname failure produces request-level `503`; an internal envelope defect produces request-level `500`.

### 3.2 CPU

| Field | Type | Unit |
|---|---|---|
| `cpu.utilization_percent` | number or null | percent, 0–100 |
| `cpu.logical_cpu_count` | integer or null | logical CPUs |

Utilization covers the validated trailing approximately-one-second interval and does not delay the request to establish that interval.

### 3.3 Load

| Field | Type | Unit |
|---|---|---|
| `load.one_minute` | number or null | dimensionless load average |
| `load.five_minutes` | number or null | dimensionless load average |
| `load.fifteen_minutes` | number or null | dimensionless load average |

### 3.4 Memory and storage

| Field | Type | Unit |
|---|---|---|
| `memory.total_bytes` | integer or null | bytes |
| `memory.available_bytes` | integer or null | bytes |
| `memory.used_bytes` | integer or null | bytes |
| `root_filesystem.total_bytes` | integer or null | bytes |
| `root_filesystem.available_bytes` | integer or null | bytes available to the unprivileged service |
| `root_filesystem.used_bytes` | integer or null | bytes |

For both groups, `used_bytes = total_bytes - available_bytes`. Inconsistent source data makes the coherent group unavailable.

### 3.5 Uptime

| Field | Type | Unit |
|---|---|---|
| `uptime_seconds` | integer or null | elapsed seconds since boot |

### 3.6 Network

`network` may be null when enumeration or topology classification fails. Otherwise, `network.interfaces` is an array ordered deterministically by interface name.

| Field | Type | Unit/meaning |
|---|---|---|
| `network.interfaces[].name` | string | Linux interface name |
| `network.interfaces[].state` | string or null | Current kernel operational state |
| `network.interfaces[].rx_bytes` | integer or null | Exact cumulative received bytes |
| `network.interfaces[].tx_bytes` | integer or null | Exact cumulative transmitted bytes |

An eligible down interface remains present. An interface-local read failure does not discard independently valid interfaces.

### 3.7 Raspberry Pi metrics

| Field | Type | Unit/meaning |
|---|---|---|
| `raspberry_pi.soc_temperature_celsius` | number or null | degrees Celsius |
| `raspberry_pi.thermal_throttling_active` | boolean or null | Active at observation |
| `raspberry_pi.thermal_throttling_occurred_since_boot` | boolean or null | Latched since boot |
| `raspberry_pi.undervoltage_active` | boolean or null | Active at observation |
| `raspberry_pi.undervoltage_occurred_since_boot` | boolean or null | Latched since boot |

The four firmware-derived booleans succeed or fail as one coherent observation. Temperature is independent.

## 4. Null and issue model

Each expected unavailable field or coherent group is `null` and has exactly one deterministic issue association. A group-level issue accounts for the null group; sibling fields without a common nullable subgroup receive individual issues.

```json
{
  "path": "/raspberry_pi/undervoltage_active",
  "code": "permission_denied",
  "message": "Firmware status is not accessible to the service account."
}
```

Issue fields:

| Field | Type | Meaning |
|---|---|---|
| `path` | string | Canonical JSON Pointer to the affected field or nullable group |
| `code` | string | One of the three allowed metric issue codes |
| `message` | string | Stable, bounded, safe consumer-facing description |

Allowed metric issue codes:

- `unsupported`: the platform capability is genuinely absent or explicitly unsupported.
- `permission_denied`: the capability exists but the runtime identity lacks access.
- `temporarily_unavailable`: timeout, busy source, transient I/O, disappearance, or malformed/inconsistent platform data.

`internal_error` is forbidden in `issues[].code`. An internal defect invalidates the entire cycle and produces a request-level error instead.

Issues are ordered by fixed collector order and then fixed field order. Map iteration and asynchronous completion order must not affect them.

## 5. Request-level error envelope

```json
{
  "error": {
    "code": "temporarily_unavailable",
    "message": "No useful snapshot is currently available."
  }
}
```

| Field | Type | Meaning |
|---|---|---|
| `error.code` | string | `invalid_request`, `temporarily_unavailable`, or `internal_error` |
| `error.message` | string | Stable, bounded, safe description |

Low-level OS errors, filesystem paths, raw platform data, and stack traces must not appear in public messages.

## 6. JSON numeric and text rules

- Byte counters, byte capacities, logical CPU count, and uptime are unquoted decimal JSON integer tokens.
- Consumers must parse cumulative counters with exact integer handling; IEEE-754 binary64 alone is insufficient.
- Fixtures include values above `2^53` and, where permitted, the maximum unsigned 64-bit value.
- Floating-point metrics must be finite; NaN and infinity are invalid internal states.
- Strings are valid UTF-8 JSON strings.
- JSON object-member order and insignificant whitespace are not contractual.

## 7. Partial and total failure

- All metrics present: complete `200` with an empty `issues` array.
- At least one metric present and at least one expected acquisition failure: partial `200` with nulls and issues.
- No metric leaf present: request-level `503 temporarily_unavailable`.
- Any internal invariant, panic, shared-state, builder, or encoding defect: request-level `500 internal_error`; no metrics from that cycle are published.
- A client or socket write failure does not alter the already-built snapshot and is not a metric issue.

## 8. Safety bounds

- Maximum TCP connections: 16.
- Maximum admitted requests: 8.
- Maximum request headers: 8 KiB.
- Header/read timeout: 2 seconds.
- Idle keep-alive timeout: 15 seconds.
- Overall request deadline: 1 second.
- Response/write deadline: 1 second.
- Maximum encoded successful snapshot: 64 KiB.
- Maximum encoded request-level error: 2 KiB.

These values are part of the v0.1 operational HTTP contract. They may be reconsidered only through an approved baseline revision.

## 9. Compatibility and change control

- `/v1` identifies the public operation family.
- `schema_version` identifies the JSON schema.
- Consumers must reject or deliberately handle unknown incompatible schema versions.
- v0.1 adds no alternate endpoint or representation.
- A breaking field, type, nullability, semantic, or error change requires appropriate API/schema versioning.

## 10. Approved v0.1 transport errata

These narrow corrections reconcile the contract with the approved standard-library HTTP/1.1 server and conventional HTTP semantics. They do not change the snapshot schema or application-level error mapping.

- A request rejected by the Go HTTP parser for exceeding the header limit does not reach the application handler. Its transport-generated `431 Request Header Fields Too Large` response may therefore use the standard-library plain-text body and close the connection instead of using the JSON `invalid_request` envelope.
- `HEAD /v1/snapshot` remains an unsupported method and returns `405 Method Not Allowed` with `Allow: GET`. In accordance with HTTP HEAD semantics, the response carries no JSON body even though its logical request-level classification is `invalid_request`.

This document is frozen. Future releases receive a self-contained release-specific HTTP contract. Corrections here must be labelled as v0.1 errata and must not silently redefine shipped behavior.
