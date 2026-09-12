#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
evaluator="$script_dir/package-baseline-evaluate.awk"
verifier="$script_dir/verify-readonly.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

checks='package_query_absent selection_absent payload_absent dpkg_info_absent dpkg_state_absent unit_not_found unit_not_enabled systemd_state_absent helper_system_absent helper_user_absent acl_absent identity_valid process_absent port_available policy_recorded'

write_fixture() {
    output=$1
    blocked_name=${2:-}
    : >"$output"
    for name in $checks; do
        status=PASS
        [ "$name" != "$blocked_name" ] || status=BLOCKED
        printf '%s\t%s\tfixture\n' "$name" "$status" >>"$output"
    done
}

write_fixture "$work/clean.tsv"
awk -f "$evaluator" "$work/clean.tsv" >"$work/clean.summary"
grep -Fx 'classification=PASS' "$work/clean.summary" >/dev/null

for residual in $checks; do
    write_fixture "$work/$residual.tsv" "$residual"
    if awk -f "$evaluator" "$work/$residual.tsv" >"$work/$residual.summary"; then
        echo "joy-pi-health: expected $residual to block the baseline" >&2
        exit 1
    fi
    grep -Fx 'classification=BLOCKED' "$work/$residual.summary" >/dev/null
done

awk -F '\t' '$1 != "payload_absent"' "$work/clean.tsv" >"$work/missing.tsv"
if awk -f "$evaluator" "$work/missing.tsv" >/dev/null; then
    echo 'joy-pi-health: expected a missing check to block the baseline' >&2
    exit 1
fi

cp "$work/clean.tsv" "$work/duplicate.tsv"
awk -F '\t' '$1 == "identity_valid"' "$work/clean.tsv" >>"$work/duplicate.tsv"
if awk -f "$evaluator" "$work/duplicate.tsv" >/dev/null; then
    echo 'joy-pi-health: expected a duplicate check to block the baseline' >&2
    exit 1
fi

printf 'identity_valid\tUNKNOWN\tfixture\n' >"$work/malformed.tsv"
if awk -f "$evaluator" "$work/malformed.tsv" >/dev/null; then
    echo 'joy-pi-health: expected malformed status to block the baseline' >&2
    exit 1
fi

cp "$work/clean.tsv" "$work/unexpected.tsv"
printf 'unexpected_check\tPASS\tfixture\n' >>"$work/unexpected.tsv"
if awk -f "$evaluator" "$work/unexpected.tsv" >/dev/null; then
    echo 'joy-pi-health: expected an unexpected check to block the baseline' >&2
    exit 1
fi

grep -F 'package-baseline pre CANDIDATE_DEB EVIDENCE_DIR' "$verifier" >/dev/null
grep -F 'package-baseline post CANDIDATE_DEB EVIDENCE_DIR' "$verifier" >/dev/null
sed -n '/^package_baseline()/,/^require_command()/p' "$verifier" >"$work/package-baseline-function.sh"
if grep -E 'rm[[:space:]]+-r|systemctl[[:space:]]+(enable|start)' "$work/package-baseline-function.sh" >/dev/null; then
    echo 'joy-pi-health: package baseline verifier contains a mutating service operation' >&2
    exit 1
fi
if grep -E '/var/log|journalctl' "$work/package-baseline-function.sh" >/dev/null; then
    echo 'joy-pi-health: package baseline verifier must not classify or delete retained history' >&2
    exit 1
fi

echo 'Raspberry Pi 3B+ package baseline methodology tests passed'
