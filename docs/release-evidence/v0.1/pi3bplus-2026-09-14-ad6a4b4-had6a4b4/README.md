# v0.1 Pi 3B+ acceptance record

Final Class C result: **PASS — 49/49 gates PASS**. Candidate and executed harness SHA: `ad6a4b4e2eb307502c822529863532f2947914d3`.

Git retains the [final result and 49 classifications](result.md), [sign-off approvals](approvals.md), and curated [provenance](summaries/provenance.json), [artifact identity](summaries/artifact-identity.txt) and [measurements](summaries/measurements.md). Full raw Class C evidence is intentionally not versioned in Git for this run.

- Class A: https://github.com/Adrien-hue/joy-pi-health/actions/runs/34893626368
- Class B: https://github.com/Adrien-hue/joy-pi-health/actions/runs/34893626428 (attempt 1)
- Downloaded artifact: `joy-pi-health-ci-validation-ad6a4b4e2eb307502c822529863532f2947914d3-34893626428-1`
- **commands.log: NOT RETAINED**. No transcript was reconstructed.

## Local retention

The complete evidence is preserved locally, relative to the repository root, at:

```text
.local/release-evidence/v0.1/pi3bplus-2026-09-14-ad6a4b4-had6a4b4/
  finalized/             475 original finalized/reviewed files, unchanged
  original-capture/      454 original capture files, unchanged
  ARCHIVE_SHA256SUMS     checksum manifest for all 929 payload files
```

The finalized branch retains the full original result, review script/notes, source inventory, all raw gate evidence and the original `EVIDENCE_SHA256SUMS`. The original-capture branch retains the unredacted source, including the final filesystem capture; its external source copy was also left untouched. Earlier blocked/skipped attempts remain in both branches. Historical runs from other candidates remain separate and unchanged; no already-committed evidence was moved or removed.

This archive is ignored by Git and may contain private identifiers. **A clone does not restore it. Back it up separately with appropriate access controls.** The tracked record is sufficient to read the sign-off, but a detailed raw-evidence audit requires the archive. No new redaction or raw-content edit was made during retention restructuring.

## Integrity and retrieval

[archive-reference.json](archive-reference.json) pins both branch counts/byte totals, the original finalized seal digest and the complete archive manifest digest. `ARCHIVE_SHA256SUMS` uses archive-root-relative paths and excludes only itself. The nested original `EVIDENCE_SHA256SUMS` remains relative to `finalized/` and excludes only itself. [RECORD_SHA256SUMS](RECORD_SHA256SUMS) covers the other eight curated files, including `archive-reference.json`, and excludes itself. Scoped Git attributes prevent line-ending conversion from breaking this seal.

To verify using Git Bash or another shell with `sha256sum`, run from the repository root:

```sh
name=pi3bplus-2026-09-14-ad6a4b4-had6a4b4
record=docs/release-evidence/v0.1/$name
archive=.local/release-evidence/v0.1/$name
(cd "$record" && sha256sum -c RECORD_SHA256SUMS)
sha256sum "$archive/ARCHIVE_SHA256SUMS"
# Compare this digest with archive_manifest.sha256 in archive-reference.json.
(cd "$archive" && sha256sum -c ARCHIVE_SHA256SUMS)
(cd "$archive/finalized" && sha256sum -c EVIDENCE_SHA256SUMS)
```

The result table's `finalized/...` locators are paths inside this local archive, not missing Git links. The full archived result indexes additional supporting files. Retention restructuring changes no acceptance result, threshold, product byte or executed harness; see the labelled 2026-09-17 clarification in [runbook §13](../../../release-validation.md#13-evidence-and-release-decision). No commit, tag or publication is implied.
