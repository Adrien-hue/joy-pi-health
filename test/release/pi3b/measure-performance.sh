#!/bin/sh
set -eu

service=joy-pi-health.service
default_url=http://127.0.0.1:8080/v1/snapshot

usage() {
    cat >&2 <<'EOF'
usage:
  test/release/pi3b/measure-performance.sh rss EVIDENCE_DIR [SNAPSHOT_URL]
  test/release/pi3b/measure-performance.sh idle-cpu EVIDENCE_DIR
  test/release/pi3b/measure-performance.sh readiness EVIDENCE_DIR
  test/release/pi3b/measure-performance.sh latency EVIDENCE_DIR [SNAPSHOT_URL]
  test/release/pi3b/measure-performance.sh cpu EVIDENCE_DIR [SNAPSHOT_URL] --allow-load
  test/release/pi3b/measure-performance.sh soak EVIDENCE_DIR [SNAPSHOT_URL]

Durations, sample counts, and release thresholds are fixed. The script never
installs tools, invokes sudo, edits service configuration, or changes device
permissions. The CPU mode starts stress-ng only with explicit --allow-load.
The readiness mode must be invoked by an operator already authorized to start
and stop the service.
EOF
    exit 2
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "joy-pi-health: required measurement command is missing: $1" >&2
        exit 1
    }
}

prepare() {
    evidence_dir=$1
    [ -n "$evidence_dir" ] || usage
    mkdir -p "$evidence_dir/raw"
    evidence_dir=$(CDPATH= cd -- "$evidence_dir" && pwd)
    export LC_ALL=C
}

main_pid() {
    pid=$(systemctl show --property=MainPID --value "$service")
    case "$pid" in
        ''|0|*[!0-9]*) echo "joy-pi-health: service has no main PID" >&2; exit 1 ;;
    esac
    [ -r "/proc/$pid/status" ] || { echo "joy-pi-health: service process is not readable" >&2; exit 1; }
    printf '%s\n' "$pid"
}

monotonic_ns() {
    # GNU date uses CLOCK_REALTIME, so elapsed calculations also retain the
    # kernel monotonic uptime value and reject a wall-clock discontinuity.
    wall=$(date +%s%N)
    mono=$(awk '{printf "%.0f", $1 * 1000000000}' /proc/uptime)
    printf '%s %s\n' "$wall" "$mono"
}

sleep_to_one_second() {
    started=$1
    now=$(awk '{printf "%.0f", $1 * 1000000000}' /proc/uptime)
    remaining=$(awk -v start="$started" -v current="$now" 'BEGIN { value=(1000000000-(current-start))/1000000000; if (value>0) printf "%.6f", value; else print "0" }')
    [ "$remaining" = 0 ] || sleep "$remaining"
}

nearest_rank() {
    file=$1
    column=$2
    percentile=$3
    awk -v column="$column" 'NR > 1 {print $column}' "$file" | sort -n | \
        awk -v p="$percentile" '{values[NR]=$1} END {if (NR==0) exit 1; rank=int(NR*p); if (rank < NR*p) rank++; if (rank<1) rank=1; print values[rank]}'
}

median() {
    file=$1
    column=$2
    awk -v column="$column" 'NR > 1 {print $column}' "$file" | sort -n | \
        awk '{values[NR]=$1} END {if (NR==0) exit 1; if (NR%2) print values[(NR+1)/2]; else printf "%.6f\n", (values[NR/2]+values[NR/2+1])/2}'
}

assert_at_most() {
    value=$1
    limit=$2
    label=$3
    awk -v value="$value" -v limit="$limit" 'BEGIN {exit !(value <= limit)}' || {
        echo "joy-pi-health: $label is $value; limit is $limit" >&2
        exit 1
    }
}

rss_mode() {
    [ "$#" -ge 1 ] && [ "$#" -le 2 ] || usage
    prepare "$1"
    url=${2:-$default_url}
    for command_name in awk curl date jq mktemp sleep systemctl; do require_command "$command_name"; done
    pid=$(main_pid)
    output="$evidence_dir/raw/rss.tsv"
    printf 'monotonic_seconds\tphase\trss_kib\thttp_status\n' >"$output"

    echo "warming for 60 seconds" >&2
    sleep 60
    sample_rss_phase "$pid" idle_before 300 no "$url" "$output"
    sample_rss_phase "$pid" requests 600 yes "$url" "$output"
    sample_rss_phase "$pid" idle_after 300 no "$url" "$output"

    maximum=$(awk 'NR>1 && $3>maximum {maximum=$3} END {print maximum+0}' "$output")
    printf 'maximum_rss_kib=%s\nhard_limit_kib=25600\nengineering_target_kib=20480\n' "$maximum" \
        >"$evidence_dir/raw/rss-summary.txt"
    assert_at_most "$maximum" 25600 "maximum RSS KiB"
}

sample_rss_phase() {
    pid=$1
    phase=$2
    seconds=$3
    request=$4
    url=$5
    output=$6
    index=0
    while [ "$index" -lt "$seconds" ]; do
        started=$(awk '{printf "%.0f", $1 * 1000000000}' /proc/uptime)
        rss=$(awk '/^VmRSS:/ {print $2}' "/proc/$pid/status")
        [ -n "$rss" ] || { echo "joy-pi-health: VmRSS became unavailable" >&2; exit 1; }
        status=-
        if [ "$request" = yes ]; then
            response=$(mktemp)
            status=$(curl --silent --show-error --max-time 1 --output "$response" --write-out '%{http_code}' "$url")
            [ "$status" = 200 ]
            jq -e '.schema_version == "1.0"' "$response" >/dev/null
            rm -f "$response"
        fi
        timestamp=$(awk '{print $1}' /proc/uptime)
        printf '%s\t%s\t%s\t%s\n' "$timestamp" "$phase" "$rss" "$status" >>"$output"
        index=$((index + 1))
        sleep_to_one_second "$started"
    done
}

idle_cpu_mode() {
    [ "$#" -eq 1 ] || usage
    prepare "$1"
    require_command awk
    require_command getconf
    require_command sleep
    require_command systemctl
    pid=$(main_pid)
    ticks_per_second=$(getconf CLK_TCK)
    before_ticks=$(awk '{print $14+$15}' "/proc/$pid/stat")
    before_mono=$(awk '{print $1}' /proc/uptime)
    sleep 600
    after_ticks=$(awk '{print $14+$15}' "/proc/$pid/stat")
    after_mono=$(awk '{print $1}' /proc/uptime)
    percent=$(awk -v before="$before_ticks" -v after="$after_ticks" -v hz="$ticks_per_second" -v start="$before_mono" -v stop="$after_mono" \
        'BEGIN {elapsed=stop-start; if (elapsed<=0 || after<before) exit 1; printf "%.6f", 100*((after-before)/hz)/elapsed}')
    {
        echo "elapsed_seconds=$(awk -v a="$before_mono" -v b="$after_mono" 'BEGIN {printf "%.6f", b-a}')"
        echo "cpu_percent_of_one_core=$percent"
        echo "hard_limit_percent=0.5"
        echo "engineering_target_percent=0.25"
    } >"$evidence_dir/raw/idle-cpu.txt"
    assert_at_most "$percent" 0.5 "idle CPU percent of one core"
}

readiness_mode() {
    [ "$#" -eq 1 ] || usage
    prepare "$1"
    require_command awk
    require_command date
    require_command systemctl
    output="$evidence_dir/raw/readiness.tsv"
    printf 'run\telapsed_milliseconds\tactive_state\n' >"$output"
    index=1
    while [ "$index" -le 20 ]; do
        systemctl stop "$service"
        systemctl reset-failed "$service" || true
        set -- $(monotonic_ns)
        wall_before=$1
        mono_before=$2
        systemctl start "$service"
        set -- $(monotonic_ns)
        wall_after=$1
        mono_after=$2
        wall_delta=$((wall_after - wall_before))
        mono_delta=$((mono_after - mono_before))
        difference=$((wall_delta - mono_delta))
        [ "$difference" -lt 0 ] && difference=$((-difference))
        [ "$difference" -le 100000000 ] || { echo "joy-pi-health: wall clock moved during readiness measurement" >&2; exit 1; }
        elapsed_ms=$((mono_delta / 1000000))
        state=$(systemctl is-active "$service")
        printf '%s\t%s\t%s\n' "$index" "$elapsed_ms" "$state" >>"$output"
        [ "$state" = active ]
        assert_at_most "$elapsed_ms" 1000 "startup-to-readiness milliseconds"
        index=$((index + 1))
    done
    maximum=$(awk 'NR>1 && $2>maximum {maximum=$2} END {print maximum+0}' "$output")
    p95=$(nearest_rank "$output" 2 0.95)
    printf 'maximum_milliseconds=%s\np95_milliseconds=%s\nhard_limit_milliseconds=1000\nengineering_target_milliseconds=500\n' "$maximum" "$p95" \
        >"$evidence_dir/raw/readiness-summary.txt"
}

latency_mode() {
    [ "$#" -ge 1 ] && [ "$#" -le 2 ] || usage
    prepare "$1"
    url=${2:-$default_url}
    for command_name in awk curl jq mktemp sleep sort; do require_command "$command_name"; done
    output="$evidence_dir/raw/latency.tsv"
    response=$(mktemp)
    trap 'rm -f "$response"' EXIT HUP INT TERM
    printf 'request\telapsed_milliseconds\thttp_status\n' >"$output"
    index=1
    while [ "$index" -le 600 ]; do
        started=$(awk '{printf "%.0f", $1 * 1000000000}' /proc/uptime)
        result=$(curl --silent --show-error --max-time 1 --output "$response" --write-out '%{time_total} %{http_code}' "$url")
        seconds=${result% *}
        status=${result##* }
        [ "$status" = 200 ]
        jq -e '.schema_version == "1.0"' "$response" >/dev/null
        milliseconds=$(awk -v seconds="$seconds" 'BEGIN {printf "%.6f", seconds*1000}')
        printf '%s\t%s\t%s\n' "$index" "$milliseconds" "$status" >>"$output"
        index=$((index + 1))
        sleep_to_one_second "$started"
    done
    p95=$(nearest_rank "$output" 2 0.95)
    median_value=$(median "$output" 2)
    printf 'median_milliseconds=%s\np95_milliseconds=%s\nhard_p95_limit_milliseconds=100\nengineering_p95_target_milliseconds=75\n' "$median_value" "$p95" \
        >"$evidence_dir/raw/latency-summary.txt"
    assert_at_most "$p95" 100 "snapshot p95 milliseconds"
}

read_cpu_counters() {
    awk '/^cpu / {idle=$5+$6; total=0; for (i=2;i<=9;i++) total+=$i; print total-idle, total; exit}' /proc/stat
}

cpu_pair() {
    phase=$1
    index=$2
    url=$3
    output=$4
    set -- $(read_cpu_counters)
    busy_before=$1
    total_before=$2
    sleep 1
    set -- $(read_cpu_counters)
    busy_after=$1
    total_after=$2
    reference=$(awk -v b1="$busy_before" -v b2="$busy_after" -v t1="$total_before" -v t2="$total_after" \
        'BEGIN {busy=b2-b1; total=t2-t1; if (busy<0 || total<=0 || busy>total) exit 1; printf "%.6f", 100*busy/total}')
    service_value=$(curl --silent --show-error --fail --max-time 1 "$url" | jq -er '.cpu.utilization_percent | numbers')
    difference=$(awk -v reference="$reference" -v service="$service_value" 'BEGIN {d=reference-service; if (d<0) d=-d; printf "%.6f", d}')
    printf '%s\t%s\t%s\t%s\t%s\n' "$phase" "$index" "$reference" "$service_value" "$difference" >>"$output"
}

cpu_mode() {
    [ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
    prepare "$1"
    url=$default_url
    approval=
    if [ "$#" -eq 2 ]; then approval=$2; else url=$2; approval=$3; fi
    [ "$approval" = --allow-load ] || { echo "joy-pi-health: CPU workload requires explicit --allow-load" >&2; exit 2; }
    for command_name in awk curl jq nproc sleep sort stress-ng; do require_command "$command_name"; done
    output="$evidence_dir/raw/cpu-comparisons.tsv"
    printf 'phase\tsample\treference_percent\tservice_percent\tabsolute_difference\n' >"$output"
    index=1
    while [ "$index" -le 10 ]; do cpu_pair idle "$index" "$url" "$output"; index=$((index + 1)); done

    stress-ng --cpu "$(nproc)" --cpu-load 50 --timeout 30s >"$evidence_dir/raw/stress-mixed.log" 2>&1 &
    stress_pid=$!
    trap 'kill "$stress_pid" 2>/dev/null || true; wait "$stress_pid" 2>/dev/null || true' EXIT HUP INT TERM
    sleep 2
    index=1
    while [ "$index" -le 10 ]; do cpu_pair mixed "$index" "$url" "$output"; index=$((index + 1)); done
    wait "$stress_pid"
    trap - EXIT HUP INT TERM

    median_difference=$(median "$output" 5)
    p95_difference=$(nearest_rank "$output" 5 0.95)
    assert_at_most "$median_difference" 5 "median CPU absolute difference"
    assert_at_most "$p95_difference" 10 "CPU p95 absolute difference"

    busy_output="$evidence_dir/raw/cpu-all-core.tsv"
    printf 'sample\tservice_percent\n' >"$busy_output"
    stress-ng --cpu "$(nproc)" --cpu-load 100 --timeout 15s >"$evidence_dir/raw/stress-all-core.log" 2>&1 &
    stress_pid=$!
    trap 'kill "$stress_pid" 2>/dev/null || true; wait "$stress_pid" 2>/dev/null || true' EXIT HUP INT TERM
    sleep 2
    index=1
    while [ "$index" -le 5 ]; do
        sleep 1
        value=$(curl --silent --show-error --fail --max-time 1 "$url" | jq -er '.cpu.utilization_percent | numbers')
        printf '%s\t%s\n' "$index" "$value" >>"$busy_output"
        awk -v value="$value" 'BEGIN {exit !(value >= 90)}' || {
            echo "joy-pi-health: sustained all-core utilization is below 90%: $value" >&2
            exit 1
        }
        index=$((index + 1))
    done
    wait "$stress_pid"
    trap - EXIT HUP INT TERM
    printf 'median_absolute_difference=%s\np95_absolute_difference=%s\nmedian_limit=5\np95_limit=10\nall_core_minimum=90\n' \
        "$median_difference" "$p95_difference" >"$evidence_dir/raw/cpu-summary.txt"
}

soak_mode() {
    [ "$#" -ge 1 ] && [ "$#" -le 2 ] || usage
    prepare "$1"
    url=${2:-$default_url}
    for command_name in awk curl date find journalctl jq mktemp sleep sort systemctl wc; do require_command "$command_name"; done
    pid=$(main_pid)
    requests="$evidence_dir/raw/soak-requests.tsv"
    resources="$evidence_dir/raw/soak.tsv"
    response=$(mktemp)
    trap 'rm -f "$response"' EXIT HUP INT TERM
    printf 'second\thttp_status\telapsed_milliseconds\tissue_count\n' >"$requests"
    printf 'second\trss_kib\tcpu_ticks\tfile_descriptors\tthreads\tpid\trestarts\tjournal_records\n' >"$resources"
    journal_since=$(date --iso-8601=seconds)
    baseline_restarts=$(systemctl show --property=NRestarts --value "$service")
    second=0
    while [ "$second" -lt 3600 ]; do
        started=$(awk '{printf "%.0f", $1 * 1000000000}' /proc/uptime)
        result=$(curl --silent --show-error --max-time 1 --output "$response" --write-out '%{time_total} %{http_code}' "$url")
        elapsed=${result% *}
        status=${result##* }
        [ "$status" = 200 ]
        issues=$(jq -er '.issues | length' "$response")
        milliseconds=$(awk -v seconds="$elapsed" 'BEGIN {printf "%.6f", seconds*1000}')
        printf '%s\t%s\t%s\t%s\n' "$second" "$status" "$milliseconds" "$issues" >>"$requests"
        if [ $((second % 10)) -eq 0 ]; then
            current_pid=$(main_pid)
            [ "$current_pid" = "$pid" ] || { echo "joy-pi-health: service PID changed during soak" >&2; exit 1; }
            rss=$(awk '/^VmRSS:/ {print $2}' "/proc/$pid/status")
            ticks=$(awk '{print $14+$15}' "/proc/$pid/stat")
            descriptors=$(find "/proc/$pid/fd" -mindepth 1 -maxdepth 1 2>/dev/null | wc -l)
            threads=$(awk '/^Threads:/ {print $2}' "/proc/$pid/status")
            restarts=$(systemctl show --property=NRestarts --value "$service")
            journal_records=$(journalctl --quiet --no-pager --unit "$service" --since "$journal_since" 2>/dev/null | wc -l)
            printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$second" "$rss" "$ticks" "$descriptors" "$threads" "$pid" "$restarts" "$journal_records" >>"$resources"
            [ "$restarts" -eq "$baseline_restarts" ] || {
                echo "joy-pi-health: service restarted during soak" >&2
                exit 1
            }
            assert_at_most "$rss" 25600 "soak RSS KiB"
        fi
        second=$((second + 1))
        sleep_to_one_second "$started"
    done

    first_p95=$(awk 'NR>1 && $1<600 {print $2}' "$resources" | sort -n | awk '{v[NR]=$1} END {r=int(NR*.95); if(r<NR*.95)r++; print v[r]}')
    final_p95=$(awk 'NR>1 && $1>=3000 {print $2}' "$resources" | sort -n | awk '{v[NR]=$1} END {r=int(NR*.95); if(r<NR*.95)r++; print v[r]}')
    growth=$((final_p95 - first_p95))
    [ "$growth" -le 1024 ] || { echo "joy-pi-health: soak RSS p95 grew by more than 1 MiB" >&2; exit 1; }
    printf 'first_ten_minute_rss_p95_kib=%s\nfinal_ten_minute_rss_p95_kib=%s\ngrowth_kib=%s\nmaximum_growth_kib=1024\n' \
        "$first_p95" "$final_p95" "$growth" >"$evidence_dir/raw/soak-summary.txt"
}

[ "$#" -ge 1 ] || usage
mode=$1
shift
case "$mode" in
    rss) rss_mode "$@" ;;
    idle-cpu) idle_cpu_mode "$@" ;;
    readiness) readiness_mode "$@" ;;
    latency) latency_mode "$@" ;;
    cpu) cpu_mode "$@" ;;
    soak) soak_mode "$@" ;;
    *) usage ;;
esac
