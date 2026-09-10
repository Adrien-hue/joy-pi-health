#!/bin/sh
set -eu

service=joy-pi-health.service
base_url=http://127.0.0.1:8080

usage() {
    cat >&2 <<'EOF'
usage:
  test/release/pi3bplus/verify-readonly.sh verify-artifacts ARTIFACT_DIR EVIDENCE_DIR EXPECTED_COMMIT [video|acl]
  test/release/pi3bplus/verify-readonly.sh inspect-platform EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh rootless ARTIFACT_DIR EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh managed EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh validate-model MODEL_FILE

The script never installs packages, changes permissions, edits service
configuration, or invokes sudo. It writes only below EVIDENCE_DIR and temporary
directories that it creates.
EOF
    exit 2
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "joy-pi-health: required acceptance command is missing: $1" >&2
        exit 1
    }
}

prepare_evidence() {
    evidence_dir=$1
    [ -n "$evidence_dir" ] || usage
    mkdir -p "$evidence_dir/raw"
    evidence_dir=$(CDPATH= cd -- "$evidence_dir" && pwd)
}

record_command() {
    description=$1
    shift
    {
        printf '\n===== %s\n' "$description"
        printf '$'
        printf ' %s' "$@"
        printf '\n'
        "$@"
    } 2>&1
}

release_gating_model='Raspberry Pi 3 Model B Plus'

validate_model_file() {
    [ "$#" -eq 1 ] || return 2
    model_file=$1
    [ -r "$model_file" ] || return 1

    byte_count=$(wc -c <"$model_file")
    model_with_sentinel=$(tr -d '\000' <"$model_file"; printf '\001')
    model=${model_with_sentinel%?}
    text_byte_count=$(printf '%s' "$model" | wc -c)
    nul_count=$((byte_count - text_byte_count))
    case "$nul_count" in
        0) ;;
        1)
            last_byte=$(tail -c 1 "$model_file" | od -An -tu1 | tr -d '[:space:]')
            [ "$last_byte" = 0 ] || return 1
            ;;
        *) return 1 ;;
    esac

    case "$model" in
        "$release_gating_model") return 0 ;;
        "$release_gating_model Rev "*)
            revision=${model#"$release_gating_model Rev "}
            printf '%s\n' "$revision" | grep -Eq '^[0-9]+([.][0-9]+)*$'
            ;;
        *) return 1 ;;
    esac
}

verify_artifacts() {
    [ "$#" -ge 3 ] && [ "$#" -le 4 ] || usage
    artifact_dir=$1
    prepare_evidence "$2"
    expected_commit=$3
    expected_profile=${4:-video}
    case "$expected_profile" in video|acl) ;; *) usage ;; esac
    artifact_dir=$(CDPATH= cd -- "$artifact_dir" && pwd)

    for command_name in cmp cp dpkg-deb file find jq readelf sha256sum sort tar; do
        require_command "$command_name"
    done

    actual=$(find "$artifact_dir" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | sort)
    expected=$(printf '%s\n' \
        SHA256SUMS \
        joy-pi-health-v0.1.0-linux-arm64.tar.gz \
        joy-pi-health-v0.1.0-release.json \
        joy-pi-health_0.1.0-1_arm64.deb | sort)
    [ "$actual" = "$expected" ] || {
        echo "joy-pi-health: artifact allow-list mismatch" >&2
        printf '%s\n' "$actual" >&2
        exit 1
    }
    if find "$artifact_dir" -mindepth 1 -maxdepth 1 ! -type f -print -quit | grep . >/dev/null; then
        echo "joy-pi-health: artifact directory contains a non-file entry" >&2
        exit 1
    fi

    (
        cd "$artifact_dir"
        sha256sum --check SHA256SUMS
        sha256sum SHA256SUMS joy-pi-health-v0.1.0-linux-arm64.tar.gz \
            joy-pi-health-v0.1.0-release.json joy-pi-health_0.1.0-1_arm64.deb \
            >"$evidence_dir/raw/artifact-hashes.txt"
    )

    metadata="$artifact_dir/joy-pi-health-v0.1.0-release.json"
    deb="$artifact_dir/joy-pi-health_0.1.0-1_arm64.deb"
    archive="$artifact_dir/joy-pi-health-v0.1.0-linux-arm64.tar.gz"
    tar -tzf "$archive" >"$evidence_dir/raw/archive-list.txt"
    if grep -E '(^/|(^|/)\.\.(/|$))' "$evidence_dir/raw/archive-list.txt" >/dev/null; then
        echo "joy-pi-health: portable archive contains an unsafe path" >&2
        exit 1
    fi
    tar -tvzf "$archive" >"$evidence_dir/raw/archive-verbose-list.txt"
    if awk 'substr($1,1,1) != "-" && substr($1,1,1) != "d" {found=1} END {exit !found}' \
        "$evidence_dir/raw/archive-verbose-list.txt"; then
        echo "joy-pi-health: portable archive contains a non-file, non-directory entry" >&2
        exit 1
    fi
    jq -e --arg commit "$expected_commit" --arg profile "$expected_profile" '
        .project == "joy-pi-health" and
        .version == "0.1.0" and
        .debian_version == "0.1.0-1" and
        .commit == $commit and
        .go_version == "go1.27.1" and
        .target == "linux/arm64/v8.0" and
        .cgo_enabled == false and
        .firmware_access == $profile and
        .module == "github.com/Adrien-hue/joy-pi-health" and
        (.third_party_dependencies | length) == 0
    ' "$metadata" >/dev/null
    cp "$metadata" "$evidence_dir/raw/metadata.json"

    {
        dpkg-deb --field "$deb" Package Version Architecture Depends
        file "$deb"
    } >"$evidence_dir/raw/package.txt"
    [ "$(dpkg-deb --field "$deb" Package)" = joy-pi-health ]
    [ "$(dpkg-deb --field "$deb" Version)" = 0.1.0-1 ]
    [ "$(dpkg-deb --field "$deb" Architecture)" = arm64 ]

    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT HUP INT TERM
    mkdir -p "$work/deb" "$work/tar"
    dpkg-deb --extract "$deb" "$work/deb"
    tar -xzf "$archive" -C "$work/tar"
    archive_directory="$work/tar/joy-pi-health-v0.1.0-linux-arm64"
    archive_files=$(cd "$archive_directory" && find . -type f -printf '%P\n' | sort)
    expected_archive_files=$(printf '%s\n' \
        LICENSE README.md \
        docs/architecture-v0.1.md docs/http-api-v0.1.md docs/requirements-v0.1.md \
        joy-pi-health release-metadata.json | sort)
    [ "$archive_files" = "$expected_archive_files" ] || {
        echo "joy-pi-health: portable archive allow-list mismatch" >&2
        exit 1
    }
    binary="$work/deb/usr/bin/joy-pi-health"
    portable="$archive_directory/joy-pi-health"
    cmp "$binary" "$portable"
    file "$binary" | grep -E 'ELF 64-bit.*(ARM aarch64|ARM64)' >/dev/null
    if readelf -l "$binary" | grep -F 'Requesting program interpreter' >/dev/null; then
        echo "joy-pi-health: candidate executable is dynamically linked" >&2
        exit 1
    fi
    sha256sum "$binary" "$portable" >"$evidence_dir/raw/executable-hashes.txt"
    echo "artifact identity verification completed"
}

inspect_platform() {
    [ "$#" -eq 1 ] || usage
    prepare_evidence "$1"
    for command_name in awk basename dpkg find findmnt getent grep nproc od sed stat systemctl tail tr uname wc; do
        require_command "$command_name"
    done

    output="$evidence_dir/raw/platform.txt"
    {
        record_command kernel uname -a
        record_command architecture uname -m
        record_command debian-architecture dpkg --print-architecture
        record_command os-release sed -n '1,200p' /etc/os-release
        if [ -r /proc/device-tree/model ]; then
            printf '\n===== Raspberry Pi model\n'
            tr -d '\000' </proc/device-tree/model
            printf '\n'
        fi
        printf '\n===== CPU identity without serial\n'
        awk -F: '/^(Hardware|Model|Revision)[[:space:]]*:/ {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $1 ": " $2}' /proc/cpuinfo
        record_command logical-cpus nproc
        if [ -r /sys/devices/system/cpu/online ]; then
            record_command online-topology sed -n 1p /sys/devices/system/cpu/online
        fi
        record_command memory sed -n '/^MemTotal:/p' /proc/meminfo
        record_command root-filesystem findmnt -no SOURCE,FSTYPE,SIZE,AVAIL,OPTIONS /
        record_command systemd-version systemctl --version
        record_command dpkg-version dpkg --version
        record_command video-group getent group video
        printf '\n===== firmware devices\n'
        for device in /dev/vcio /dev/vcio_gencmd; do
            if [ -e "$device" ]; then
                stat -Lc '%n type=%F owner=%U group=%G mode=%a major_minor=%t:%T' "$device"
            else
                echo "$device absent"
            fi
        done
        printf '\n===== thermal zones\n'
        for zone in /sys/class/thermal/thermal_zone*; do
            [ -e "$zone" ] || continue
            type=unreadable
            temperature=unreadable
            [ -r "$zone/type" ] && type=$(sed -n 1p "$zone/type")
            [ -r "$zone/temp" ] && temperature=$(sed -n 1p "$zone/temp")
            printf '%s type=%s temperature_millidegrees=%s\n' "$(basename "$zone")" "$type" "$temperature"
        done
        printf '\n===== existing package\n'
        dpkg-query -W -f='${Status} ${Version}\n' joy-pi-health 2>&1 || true
        printf '\n===== policy-rc.d\n'
        if [ -e /usr/sbin/policy-rc.d ]; then
            stat -Lc '%n owner=%U group=%G mode=%a' /usr/sbin/policy-rc.d
        else
            echo absent
        fi
        printf '\n===== reference firmware tool\n'
        if command -v vcgencmd >/dev/null 2>&1; then
            command -v vcgencmd
            vcgencmd version 2>&1 || true
        else
            echo 'vcgencmd absent'
        fi
        printf '\n===== acceptance tool versions\n'
        for tool in curl jq stress-ng pidstat mpstat file readelf ss; do
            if command -v "$tool" >/dev/null 2>&1; then
                printf '%s: ' "$tool"
                "$tool" --version 2>&1 | sed -n 1p || true
            else
                echo "$tool: absent"
            fi
        done
    } >"$output"

    [ "$(uname -m)" = aarch64 ]
    grep -Eq '^VERSION_CODENAME=(trixie|"trixie")$' /etc/os-release
    if [ ! -r /proc/device-tree/model ] || ! validate_model_file /proc/device-tree/model; then
        echo "joy-pi-health: platform is not a Raspberry Pi 3 Model B Plus" >&2
        exit 1
    fi
    echo "platform inspection completed"
}

rootless() {
    [ "$#" -eq 2 ] || usage
    artifact_dir=$(CDPATH= cd -- "$1" && pwd)
    prepare_evidence "$2"
    [ "$(id -u)" -ne 0 ] || {
        echo "joy-pi-health: rootless acceptance must not run as root" >&2
        exit 1
    }
    for command_name in curl date find jq ss tar; do require_command "$command_name"; done

    archive="$artifact_dir/joy-pi-health-v0.1.0-linux-arm64.tar.gz"
    [ -f "$archive" ] || { echo "joy-pi-health: portable archive is missing" >&2; exit 1; }
    work=$(mktemp -d)
    process=
    cleanup() {
        if [ -n "$process" ] && kill -0 "$process" 2>/dev/null; then
            kill -TERM "$process" 2>/dev/null || true
            wait "$process" 2>/dev/null || true
        fi
        rm -rf "$work"
    }
    trap cleanup EXIT HUP INT TERM
    tar -tzf "$archive" >"$evidence_dir/raw/rootless-tar-list.txt"
    if grep -E '(^/|(^|/)\.\.(/|$))' "$evidence_dir/raw/rootless-tar-list.txt" >/dev/null; then
        echo "joy-pi-health: unsafe portable archive path" >&2
        exit 1
    fi
    tar -xzf "$archive" -C "$work"
    binary="$work/joy-pi-health-v0.1.0-linux-arm64/joy-pi-health"
    "$binary" --help >"$evidence_dir/raw/rootless-help.txt"
    mkdir "$work/run"
    (
        cd "$work/run"
        exec "$binary"
    ) >"$evidence_dir/raw/rootless-stdout.log" 2>"$evidence_dir/raw/rootless-stderr.log" &
    process=$!

    attempt=0
    while ! curl --silent --show-error --fail --max-time 1 "$base_url/v1/snapshot" \
        >"$evidence_dir/raw/rootless-initial-snapshot.json"; do
        attempt=$((attempt + 1))
        [ "$attempt" -lt 40 ] || { echo "joy-pi-health: rootless listener did not become available" >&2; exit 1; }
        kill -0 "$process" 2>/dev/null || { echo "joy-pi-health: rootless process exited during startup" >&2; exit 1; }
        sleep 0.05
    done
    jq -e '.schema_version == "1.0" and (.host.hostname | type == "string")' \
        "$evidence_dir/raw/rootless-initial-snapshot.json" >/dev/null
    sleep 2
    curl --silent --show-error --fail --max-time 1 "$base_url/v1/snapshot" \
        >"$evidence_dir/raw/rootless-warm-snapshot.json"
    jq -e '.schema_version == "1.0" and (.cpu.utilization_percent | type == "number")' \
        "$evidence_dir/raw/rootless-warm-snapshot.json" >/dev/null
    ss -ltn >"$evidence_dir/raw/rootless-listeners.txt"
    grep -E '127\.0\.0\.1:8080[[:space:]]' "$evidence_dir/raw/rootless-listeners.txt" >/dev/null
    [ -z "$(find "$work/run" -mindepth 1 -print -quit)" ] || {
        echo "joy-pi-health: rootless process created files in its working directory" >&2
        exit 1
    }

    started=$(date +%s%N)
    kill -TERM "$process"
    wait "$process"
    process=
    ended=$(date +%s%N)
    elapsed_ms=$(((ended - started) / 1000000))
    printf 'shutdown_milliseconds\n%s\n' "$elapsed_ms" >"$evidence_dir/raw/rootless-shutdown.tsv"
    [ "$elapsed_ms" -le 2000 ]
    [ ! -s "$evidence_dir/raw/rootless-stdout.log" ]
    echo "rootless acceptance capture completed"
}

http_probe() {
    label=$1
    expected_status=$2
    shift 2
    headers="$evidence_dir/raw/http-$label.headers"
    body="$evidence_dir/raw/http-$label.body"
    status=$(curl --silent --show-error --max-time 2 --dump-header "$headers" --output "$body" --write-out '%{http_code}' "$@")
    printf '%s\t%s\t%s\n' "$label" "$expected_status" "$status" >>"$evidence_dir/raw/http-contract.tsv"
    [ "$status" = "$expected_status" ]
}

managed() {
    [ "$#" -eq 1 ] || usage
    prepare_evidence "$1"
    for command_name in curl getent jq sleep ss stat systemctl; do require_command "$command_name"; done
    systemctl is-active --quiet "$service"
    pid=$(systemctl show --property=MainPID --value "$service")
    case "$pid" in ''|0|*[!0-9]*) echo "joy-pi-health: managed service has no main PID" >&2; exit 1;; esac
    service_uid=$(getent passwd _joy-pi-health | awk -F: '{print $3}')
    [ -n "$service_uid" ]

    {
        systemctl show "$service" --property=Type,NotifyAccess,User,Group,SupplementaryGroups,ActiveState,SubState,MainPID,NRestarts,Restart,RestartUSec,RestartSteps,RestartMaxDelayUSec,TimeoutStopUSec,DevicePolicy,CapabilityBoundingSet,AmbientCapabilities
        systemctl cat "$service"
        printf '\n/proc/%s/status selected fields\n' "$pid"
        sed -n '/^Name:/p;/^Uid:/p;/^Gid:/p;/^Groups:/p;/^Threads:/p;/^CapEff:/p' "/proc/$pid/status"
    } >"$evidence_dir/raw/systemd.txt"
    process_uid=$(awk '/^Uid:/ {print $2}' "/proc/$pid/status")
    [ "$process_uid" = "$service_uid" ]
    [ "$(awk '/^CapEff:/ {print $2}' "/proc/$pid/status")" = 0000000000000000 ]
    grep -Fx 'Type=notify' "$evidence_dir/raw/systemd.txt" >/dev/null
    grep -Fx 'NotifyAccess=main' "$evidence_dir/raw/systemd.txt" >/dev/null
    [ "$(grep -c '^DeviceAllow=/dev/vcio\(_gencmd\)\? r$' "$evidence_dir/raw/systemd.txt")" -eq 2 ]

    ss -ltn >"$evidence_dir/raw/managed-listeners.txt"
    grep -E '127\.0\.0\.1:8080[[:space:]]' "$evidence_dir/raw/managed-listeners.txt" >/dev/null
    sleep 2
    curl --silent --show-error --fail --max-time 2 --dump-header "$evidence_dir/raw/managed-snapshot.headers" \
        "$base_url/v1/snapshot" >"$evidence_dir/raw/managed-snapshot.json"
    jq -e '
        .schema_version == "1.0" and
        (.observed_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T.*Z$")) and
        (.host.hostname | type == "string" and length > 0) and
        (.cpu.utilization_percent | type == "number") and
        (.cpu.logical_cpu_count | type == "number") and
        (.load.one_minute | type == "number") and
        (.load.five_minutes | type == "number") and
        (.load.fifteen_minutes | type == "number") and
        (.memory.total_bytes | type == "number") and
        (.memory.available_bytes | type == "number") and
        (.memory.used_bytes == (.memory.total_bytes - .memory.available_bytes)) and
        (.root_filesystem.total_bytes | type == "number") and
        (.root_filesystem.available_bytes | type == "number") and
        (.root_filesystem.used_bytes == (.root_filesystem.total_bytes - .root_filesystem.available_bytes)) and
        (.uptime_seconds | type == "number") and
        (.network.interfaces | type == "array") and
        (.raspberry_pi.soc_temperature_celsius | type == "number") and
        (.raspberry_pi.thermal_throttling_active | type == "boolean") and
        (.raspberry_pi.thermal_throttling_occurred_since_boot | type == "boolean") and
        (.raspberry_pi.undervoltage_active | type == "boolean") and
        (.raspberry_pi.undervoltage_occurred_since_boot | type == "boolean") and
        (.issues | type == "array") and
        (.issues | length == 0) and
        (all(.issues[]; (.code == "unsupported" or .code == "permission_denied" or .code == "temporarily_unavailable"))) and
        (all(.issues[]; .code != "internal_error"))
    ' "$evidence_dir/raw/managed-snapshot.json" >/dev/null
    tr -d '\r' <"$evidence_dir/raw/managed-snapshot.headers" >"$evidence_dir/raw/managed-snapshot-normalized.headers"
    grep -i '^Content-Type: application/json$' "$evidence_dir/raw/managed-snapshot-normalized.headers" >/dev/null
    grep -i '^Cache-Control: no-store$' "$evidence_dir/raw/managed-snapshot-normalized.headers" >/dev/null
    if grep -Ei '^(ETag|Last-Modified|Content-Encoding):' "$evidence_dir/raw/managed-snapshot-normalized.headers" >/dev/null; then
        echo "joy-pi-health: snapshot response contains a forbidden cache validator or encoding" >&2
        exit 1
    fi
    [ "$(wc -c <"$evidence_dir/raw/managed-snapshot.json")" -le 65536 ]

    printf 'case\texpected\tactual\n' >"$evidence_dir/raw/http-contract.tsv"
    http_probe get 200 "$base_url/v1/snapshot"
    http_probe post 405 --request POST "$base_url/v1/snapshot"
    tr -d '\r' <"$evidence_dir/raw/http-post.headers" | grep -i '^Allow: GET$' >/dev/null
    http_probe head 405 --head "$base_url/v1/snapshot"
    tr -d '\r' <"$evidence_dir/raw/http-head.headers" | grep -i '^Allow: GET$' >/dev/null
    http_probe options 405 --request OPTIONS "$base_url/v1/snapshot"
    tr -d '\r' <"$evidence_dir/raw/http-options.headers" | grep -i '^Allow: GET$' >/dev/null
    http_probe content_length_zero 200 --header 'Content-Length: 0' "$base_url/v1/snapshot"
    http_probe content_type_ignored 200 --header 'Content-Type: application/octet-stream' "$base_url/v1/snapshot"
    http_probe accept_ignored 200 --header 'Accept: application/xml' "$base_url/v1/snapshot"
    http_probe compression_ignored 200 --header 'Accept-Encoding: gzip' "$base_url/v1/snapshot"
    if tr -d '\r' <"$evidence_dir/raw/http-compression_ignored.headers" | grep -i '^Content-Encoding:' >/dev/null; then
        echo "joy-pi-health: response compression was unexpectedly enabled" >&2
        exit 1
    fi
    http_probe query 400 "$base_url/v1/snapshot?x=1"
    http_probe body 400 --request GET --data-binary x "$base_url/v1/snapshot"
    http_probe transfer_encoding 400 --request GET --header 'Transfer-Encoding: chunked' --data-binary x "$base_url/v1/snapshot"
    http_probe unknown 404 "$base_url/not-found"
    http_probe health 404 "$base_url/health"
    http_probe ready 404 "$base_url/ready"
    http_probe metrics 404 "$base_url/metrics"
    for label in post options query body transfer_encoding unknown health ready metrics; do
        jq -e '.error.code == "invalid_request"' "$evidence_dir/raw/http-$label.body" >/dev/null
    done
    echo "managed-service acceptance capture completed"
}

[ "$#" -ge 1 ] || usage
mode=$1
shift
case "$mode" in
    verify-artifacts) verify_artifacts "$@" ;;
    inspect-platform) inspect_platform "$@" ;;
    rootless) rootless "$@" ;;
    managed) managed "$@" ;;
    validate-model) validate_model_file "$@" ;;
    *) usage ;;
esac
