#!/bin/sh
set -eu
export LC_ALL=C

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
evaluator="$script_dir/cpu-evaluate.awk"
harness="$script_dir/measure-performance.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

fail() {
    echo "joy-pi-health: $1" >&2
    exit 1
}

assert_equal() {
    expected=$1
    actual=$2
    label=$3
    [ "$actual" = "$expected" ] || fail "$label: got $actual, expected $expected"
}

assert_summary_value() {
    file=$1
    key=$2
    expected=$3
    label=$4
    awk -F= -v key="$key" -v expected="$expected" \
        '$1 == key && $2 == expected {found=1} END {exit !found}' "$file" || \
        fail "$label: expected $key=$expected"
}

make_fixture() {
    mode=$1
    output=$2
    awk -v mode="$mode" 'BEGIN {
        OFS="\t"
        print "phase", "sample", "monotonic_start_seconds", "monotonic_end_seconds", "interval_seconds", "busy_delta", "total_delta", "slice_reference_percent", "service_percent", "snapshot_timestamp"
        phases[1]="idle"; phases[2]="half"; phases[3]="full"
        limits[1]=10; limits[2]=10; limits[3]=5
        for (p=1; p<=3; p++) {
            phase=phases[p]
            for (i=1; i<=limits[p]; i++) {
                if (mode=="invalid-count" && phase=="full" && i==5) continue
                total=100
                reference=(phase=="idle" ? 5 : (phase=="half" ? 50 : 95))
                if (mode=="oscillatory" && phase=="half") reference=(i%2 ? 30 : 75)
                if (mode=="weak-full" && phase=="full") reference=89
                if (mode=="weighted" && phase=="half") {
                    if (i<=5) { total=900; reference=45 } else { total=100; reference=50 }
                }
                busy=total*reference/100
                service=reference
                comparison=(phase=="idle" ? i : 10+i)
                if (mode=="shifted" && phase!="full") service=(phase=="idle" ? 5 : 50)
                if (mode=="boundary" && phase!="full") service=reference+(comparison<=11 ? 5 : 10)
                if (mode=="median-fail" && phase!="full") service=reference+(comparison<=11 ? 6 : 10)
                if (mode=="p95-fail" && phase!="full") service=reference+(comparison<=18 ? 5 : 11)
                if (mode=="service-full-fail" && phase=="full" && i==3) service=89
                start=(p-1)*20+i
                stop=start+1
                interval=1
                recorded=reference
                if (mode=="malformed-number" && phase=="idle" && i==1) service="not-a-number"
                if (mode=="zero-delta" && phase=="idle" && i==1) { total=0; busy=0; recorded=0 }
                if (mode=="counter-decrease" && phase=="idle" && i==1) busy=-1
                if (mode=="inconsistent-delta" && phase=="idle" && i==1) recorded=reference+1
                if (mode=="invalid-interval" && phase=="idle" && i==1) { stop=start+0.8; interval=0.8 }
                print phase, i, start, stop, interval, busy, total, recorded, service, "2026-09-12T00:00:00Z"
            }
        }
    }' >"$output"
}

evaluate() {
    expected_status=$1
    expected_classification=$2
    name=$3
    fixture="$work/$name.samples.tsv"
    make_fixture "$name" "$fixture"
    set +e
    awk -v comparisons="$work/$name.comparisons.tsv" \
        -v plateaus="$work/$name.plateaus.tsv" \
        -v all_core="$work/$name.all-core.tsv" \
        -v summary="$work/$name.summary.txt" \
        -f "$evaluator" "$fixture"
    status=$?
    set -e
    assert_equal "$expected_status" "$status" "$name evaluator status"
    assert_summary_value "$work/$name.summary.txt" classification "$expected_classification" "$name classification"
}

evaluate 0 PASS shifted
comparison_count=$(awk 'END {print NR-1}' "$work/shifted.comparisons.tsv")
assert_equal 20 "$comparison_count" "shifted comparison count"
evaluate 0 PASS boundary
assert_summary_value "$work/boundary.summary.txt" median_absolute_difference 5.000000 "boundary median"
assert_summary_value "$work/boundary.summary.txt" p95_absolute_difference 10.000000 "boundary p95"
evaluate 1 FAIL median-fail
evaluate 1 FAIL p95-fail
evaluate 1 FAIL service-full-fail
evaluate 2 BLOCKED weak-full
evaluate 2 BLOCKED oscillatory
evaluate 2 BLOCKED invalid-count
evaluate 2 BLOCKED malformed-number
evaluate 2 BLOCKED zero-delta
evaluate 2 BLOCKED counter-decrease
evaluate 2 BLOCKED inconsistent-delta
evaluate 2 BLOCKED invalid-interval
evaluate 0 PASS weighted
awk -F '\t' '$1 == "half" && $2 == "10" && $3 == "45.500000" {found=1} END {exit !found}' \
    "$work/weighted.plateaus.tsv" || fail "weighted aggregate: expected half plateau with 10 samples at 45.500000 percent"

mkdir -p "$work/existing/raw/cpu"
set +e
sh "$harness" cpu "$work/existing" --allow-load >"$work/refusal.out" 2>"$work/refusal.err"
status=$?
set -e
assert_equal 1 "$status" "existing evidence refusal status"
grep -F 'refusing to overwrite existing CPU evidence directory' "$work/refusal.err" >/dev/null || \
    fail "existing evidence refusal diagnostic is missing"

sed '/^\[ "$#" -ge 1 \] || usage$/,$d' "$harness" >"$work/harness-functions.sh"
(
    . "$work/harness-functions.sh"
    cpu_response="$work/temporary-response"
    : >"$cpu_response"
    sleep 30 &
    stress_pid=$!
    owned_pid=$stress_pid
    cpu_cleanup
    [ ! -e "$cpu_response" ] || fail "CPU cleanup did not remove its temporary response"
    if kill -0 "$owned_pid" 2>/dev/null; then
        fail "CPU cleanup did not terminate and join its workload"
    fi
)
grep -F "trap 'cpu_cleanup; exit 130' HUP INT TERM" "$harness" >/dev/null || fail "CPU interruption cleanup trap is missing"
grep -F -- '--cpu-method loop' "$harness" >/dev/null || fail "fixed CPU workload method is missing"
grep -F -- '.cpu.logical_cpu_count == 4' "$harness" >/dev/null || fail "four-CPU service precondition is missing"
if grep -F -- '--cpu-load 50' "$harness" >/dev/null; then
    fail 'bursty --cpu-load 50 must not be used'
fi

echo 'Pi 3B+ CPU methodology tests passed'
