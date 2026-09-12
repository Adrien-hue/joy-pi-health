#!/bin/sh
set -eu

service=joy-pi-health.service
default_url=http://127.0.0.1:8080/v1/snapshot

usage() {
    cat >&2 <<'EOF'
usage:
  test/release/pi3bplus/measure-performance.sh rss EVIDENCE_DIR [SNAPSHOT_URL]
  test/release/pi3bplus/measure-performance.sh idle-cpu EVIDENCE_DIR
  test/release/pi3bplus/measure-performance.sh readiness EVIDENCE_DIR
  test/release/pi3bplus/measure-performance.sh latency EVIDENCE_DIR [SNAPSHOT_URL]
  test/release/pi3bplus/measure-performance.sh cpu EVIDENCE_DIR [SNAPSHOT_URL] --allow-load
  test/release/pi3bplus/measure-performance.sh soak EVIDENCE_DIR [SNAPSHOT_URL]

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
    awk '/^cpu / {for (i=2;i<=9;i++) printf "%s%s", $i, (i==9 ? ORS : OFS); exit}' /proc/stat
}

cpu_cleanup() {
    if [ "${stress_pid:-}" ]; then
        kill "$stress_pid" 2>/dev/null || true
        wait "$stress_pid" 2>/dev/null || true
        stress_pid=
    fi
    rm -f "${cpu_response:-}"
}

cpu_measurement_invalid() {
    reason=$1
    if [ "${cpu_dir:-}" ] && [ -d "$cpu_dir" ] && [ ! -e "$cpu_dir/cpu-summary.txt" ]; then
        printf 'classification=BLOCKED\nreason=%s\n' "$reason" >"$cpu_dir/cpu-summary.txt"
    fi
    echo "joy-pi-health: CPU measurement invalid: $reason; Class C CPU gate is BLOCKED" >&2
    exit 2
}

cpu_sample() {
    phase=$1
    index=$2
    url=$3
    output=$4
    mono_before=$(awk '{print $1}' /proc/uptime)
    set -- $(read_cpu_counters)
    user_before=$1
    nice_before=$2
    system_before=$3
    idle_before=$4
    iowait_before=$5
    irq_before=$6
    softirq_before=$7
    steal_before=$8
    sleep_to_one_second "$(awk -v seconds="$mono_before" 'BEGIN {printf "%.0f", seconds * 1000000000}')"
    mono_after=$(awk '{print $1}' /proc/uptime)
    set -- $(read_cpu_counters)
    user_delta=$(($1 - user_before))
    nice_delta=$(($2 - nice_before))
    system_delta=$(($3 - system_before))
    idle_delta=$(($4 - idle_before))
    iowait_delta=$(($5 - iowait_before))
    irq_delta=$(($6 - irq_before))
    softirq_delta=$(($7 - softirq_before))
    steal_delta=$(($8 - steal_before))
    busy_delta=$((user_delta + nice_delta + system_delta + irq_delta + softirq_delta))
    total_delta=$((busy_delta + idle_delta + iowait_delta + steal_delta))
    if [ "$user_delta" -lt 0 ] || [ "$nice_delta" -lt 0 ] || [ "$system_delta" -lt 0 ] || \
        [ "$idle_delta" -lt 0 ] || [ "$iowait_delta" -lt 0 ] || [ "$irq_delta" -lt 0 ] || \
        [ "$softirq_delta" -lt 0 ] || [ "$steal_delta" -lt 0 ]; then
        busy_delta=-1
    fi
    interval=$(awk -v start="$mono_before" -v stop="$mono_after" 'BEGIN {printf "%.6f", stop-start}')
    reference=$(awk -v busy="$busy_delta" -v total="$total_delta" 'BEGIN {if (busy<0 || total<=0 || busy>total) exit 1; printf "%.6f", 100*busy/total}') || reference=invalid
    curl --silent --show-error --fail --max-time 1 --output "$cpu_response" "$url" || cpu_measurement_invalid "snapshot request failed"
    service_fields=$(jq -er '[.cpu.utilization_percent, .observed_at] | @tsv' "$cpu_response") || cpu_measurement_invalid "snapshot CPU value or timestamp is unavailable"
    set -- $service_fields
    [ "$#" -eq 2 ] || cpu_measurement_invalid "snapshot CPU value or timestamp is malformed"
    service_value=$1
    snapshot_timestamp=$2
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$phase" "$index" "$mono_before" "$mono_after" "$interval" "$busy_delta" "$total_delta" "$reference" "$service_value" "$snapshot_timestamp" >>"$output"
}

cpu_start_workload() {
    workers=$1
    log=$2
    stress-ng --cpu "$workers" --cpu-load 100 --cpu-method loop >"$log" 2>&1 &
    stress_pid=$!
}

cpu_stop_workload() {
    kill "$stress_pid" 2>/dev/null || true
    wait "$stress_pid" 2>/dev/null || true
    stress_pid=
}

cpu_mode() {
    [ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
    prepare "$1"
    url=$default_url
    approval=
    if [ "$#" -eq 2 ]; then approval=$2; else url=$2; approval=$3; fi
    [ "$approval" = --allow-load ] || { echo "joy-pi-health: CPU workload requires explicit --allow-load" >&2; exit 2; }
    cpu_dir="$evidence_dir/raw/cpu"
    mkdir "$cpu_dir" 2>/dev/null || {
        echo "joy-pi-health: refusing to overwrite existing CPU evidence directory: $cpu_dir" >&2
        exit 1
    }
    for command_name in awk curl jq nproc sleep stress-ng systemctl uname; do
        command -v "$command_name" >/dev/null 2>&1 || cpu_measurement_invalid "required command is missing: $command_name"
    done
    processor_count=$(nproc)
    [ "$processor_count" -eq 4 ] || {
        cpu_measurement_invalid "exactly four logical CPUs must be online; found $processor_count"
    }
    [ "$(systemctl is-active "$service")" = active ] || cpu_measurement_invalid "service is not active"
    pid=$(main_pid)
    cpu_response=$(mktemp)
    stress_pid=
    trap 'cpu_cleanup' EXIT
    trap 'cpu_cleanup; exit 130' HUP INT TERM
    curl --silent --show-error --fail --max-time 1 --output "$cpu_response" "$url" || cpu_measurement_invalid "initial snapshot request failed"
    jq -e '.cpu.logical_cpu_count == 4 and (.cpu.utilization_percent | numbers)' "$cpu_response" >/dev/null || \
        cpu_measurement_invalid "service CPU utilization is unavailable or logical CPU count is not four"

    output="$cpu_dir/cpu-samples.tsv"
    printf 'phase\tsample\tmonotonic_start_seconds\tmonotonic_end_seconds\tinterval_seconds\tbusy_delta\ttotal_delta\tslice_reference_percent\tservice_percent\tsnapshot_timestamp\n' >"$output"
    {
        echo "kernel=$(uname -a)"
        echo "online_cpus=$(cat /sys/devices/system/cpu/online)"
        echo "logical_cpus=$processor_count"
        echo "stress_ng_version=$(stress-ng --version 2>&1 | head -n 1)"
        echo "service_pid=$pid"
        echo "ambient_load=$(cat /proc/loadavg)"
        echo "half_workload=stress-ng --cpu 2 --cpu-load 100 --cpu-method loop"
        echo "full_workload=stress-ng --cpu 4 --cpu-load 100 --cpu-method loop"
    } >"$cpu_dir/cpu-environment.txt"

    echo "settling idle plateau for 5 seconds" >&2
    sleep 5
    index=1
    while [ "$index" -le 10 ]; do cpu_sample idle "$index" "$url" "$output"; index=$((index + 1)); done

    cpu_start_workload 2 "$cpu_dir/stress-half.log"
    sleep 3
    index=1
    while [ "$index" -le 10 ]; do cpu_sample half "$index" "$url" "$output"; index=$((index + 1)); done
    cpu_stop_workload

    cpu_start_workload 4 "$cpu_dir/stress-full.log"
    sleep 3
    index=1
    while [ "$index" -le 5 ]; do
        cpu_sample full "$index" "$url" "$output"
        index=$((index + 1))
    done
    cpu_stop_workload

    evaluator=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/cpu-evaluate.awk
    set +e
    awk -v comparisons="$cpu_dir/cpu-comparisons.tsv" \
        -v plateaus="$cpu_dir/cpu-plateaus.tsv" \
        -v all_core="$cpu_dir/cpu-all-core.tsv" \
        -v summary="$cpu_dir/cpu-summary.txt" \
        -f "$evaluator" "$output"
    evaluation_status=$?
    set -e
    case "$evaluation_status" in
        0) ;;
        1) echo "joy-pi-health: CPU product acceptance failed; see $cpu_dir/cpu-summary.txt" >&2; exit 1 ;;
        2) cpu_measurement_invalid "evaluator rejected the workload or environment evidence" ;;
        *) echo "joy-pi-health: CPU evaluator failed unexpectedly" >&2; exit 1 ;;
    esac
    trap - EXIT HUP INT TERM
    cpu_cleanup
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
