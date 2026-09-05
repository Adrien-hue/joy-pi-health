# Project instructions

These instructions apply to the entire repository.

## Required context

Joy Pi Health monitors Raspberry Pi system health only. It is not medical software, is unrelated to the Joy-Pi electronics kit, and must remain independent from HomeDNS.

Before material changes, read the frozen documents relevant to the task:

- `docs/requirements-v0.1.md` — product behavior and release gates
- `docs/http-api-v0.1.md` — public HTTP/JSON contract
- `docs/architecture-v0.1.md` — internal architecture and repository boundaries

Do not reread unrelated documents for trivial changes, but do not make a material change without consulting its normative source.

## Change control

- Treat the v0.1 requirements, API, and architecture documents as frozen and approved.
- Do not silently alter a public contract, release gate, compatibility claim, or architecture invariant.
- Report conflicts between a requested change and a frozen baseline before implementing it.
- Apply explicitly approved baseline changes to every affected normative document in the same change.
- Preserve historical versioned documents; use a labelled erratum or a newly approved version.

## Engineering guardrails

- Keep one pure-Go Linux/ARM64 executable at module `github.com/Adrien-hue/joy-pi-health`.
- Prefer the standard library. Any new runtime dependency requires explicit justification and architecture review before introduction.
- Do not add a framework, cgo, production subprocess collection, plugin registry, or generic `util`/`common` layer without approved architecture revision.
- Keep production packages under `internal`; the entry point belongs at `cmd/joy-pi-health`.
- Preserve the package dependency direction defined in `docs/architecture-v0.1.md`; do not solve import cycles with `shared`, `common`, or service-locator packages.
- Keep runtime observation read-only and unprivileged.
- Do not add production fault-injection controls or test-only HTTP, CLI, or environment switches.
- Do not add HTTP operations beyond `GET /v1/snapshot` without an approved API change.
- Preserve bounded concurrency, one active collection cycle, immutable shared results, and the absence of a general snapshot cache.
- Metric issue codes are only `unsupported`, `permission_denied`, and `temporarily_unavailable`. Internal defects invalidate the cycle and use request-level `500 internal_error`.
- Preserve the Pi 3B release gates and the engineering targets documented in the architecture.

## Verification and commands

The supported development and verification command catalog is in `README.md`. Use those commands and keep that catalog current as workflows change; do not duplicate the catalog here.

Run focused checks while iterating and all applicable checks before handoff. Never claim Pi 3B acceptance without results from the real release-gating device. Record results from Pi 3B+, 4B, and 5 without strengthening compatibility claims beyond their evidence.

Keep changes scoped, avoid unrelated rewrites, prefer package-local tests and `testdata`, and do not commit generated or release artifacts unless repository policy later explicitly requires them.
