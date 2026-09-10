# Historical Class C attempt — BLOCKED under former Pi 3B baseline

**Final status:** `BLOCKED`
**Status authority:** Former v0.1 baseline and harness commit `be7a0d824f62b94c842d8e5326110b1852c5a0bc`
**Candidate commit:** `5d9593486bd057619f7e214007d988308cf5cbae`
**CI run:** `34051895987`, attempt `1`
**CI artifact:** `joy-pi-health-ci-validation-5d9593486bd057619f7e214007d988308cf5cbae-34051895987-1`
**Firmware permission profile:** `video`

## Outcome

Workstation and Pi artifact verification completed with matching hashes. Pre-install inspection then recorded the physical board as `Raspberry Pi 3 Model B Plus Rev 1.3`, revision `a020d3`. The baseline in force required Raspberry Pi 3 Model B, so the run stopped before rootless execution, package installation, or any later Class C gate.

The former validator used a prefix-compatible substring check and did not reject the Pi 3B+ raw model. That harness defect and the reference-hardware mismatch blocked further execution. No later gate was run.

The approved 2026-09-10 reference-hardware correction does not reinterpret this result. It remains historical `BLOCKED` evidence tied to the old harness and old artifact set. Because the corrected normative documents are packaged, the old artifacts cannot be reused for the restarted Class C procedure.

## Recorded artifact hashes

```text
35476e6dae4e6900d61cd273c0097187cde516b4c0c19c405e9d618dd484b665  joy-pi-health_0.1.0-1_arm64.deb
821cfd04a0b411a172a9cc402fe2b9bcfaa2d3be0361b5fd29efb4bb15bafd72  joy-pi-health-v0.1.0-linux-arm64.tar.gz
4d086f1d6e711b644f93dfb9e6ff27fd37a935d41a0c41fc4c2564e365f8d38e  joy-pi-health-v0.1.0-release.json
899513a9a58224430a4bf75b1cea2cbd9fb53a070b98fe76e40c5e05a21820b6  SHA256SUMS
```

The executable extracted from both release artifacts had SHA-256 `a8fee16c0d65dd27f9030980aa69b0dca09ae5eb8d03c4118d7aef2f4c8907cd`.

## Gate status

| Gate | Status | Evidence |
|---|---|---|
| Class B run identity supplied | PASS | CI run and artifact identifiers above |
| Workstation artifact verification | PASS | Recorded hashes and frozen verifier output |
| Pi artifact verification | PASS | Recorded hashes and frozen verifier output |
| Former Pi 3B reference-platform gate | BLOCKED | Actual model was Pi 3B+; validator was not exact |
| Rootless and all later Class C gates | NOT RUN | Execution stopped at the required boundary |

No board serial number or machine ID is retained.
