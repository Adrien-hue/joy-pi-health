#!/bin/sh
set -eu

service=joy-pi-health.service
base_url=http://127.0.0.1:8080

usage() {
    cat >&2 <<'EOF'
usage:
  test/release/pi3bplus/verify-readonly.sh verify-artifacts ARTIFACT_DIR EVIDENCE_DIR EXPECTED_COMMIT [video|acl]
  test/release/pi3bplus/verify-readonly.sh inspect-platform EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh package-baseline pre CANDIDATE_DEB EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh package-baseline post CANDIDATE_DEB EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh rootless ARTIFACT_DIR EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh managed EVIDENCE_DIR
  test/release/pi3bplus/verify-readonly.sh validate-model MODEL_FILE

The script never installs packages, changes permissions, edits service
configuration, or invokes sudo. It writes only below EVIDENCE_DIR and temporary
directories that it creates.
EOF
    exit 2
}

baseline_check() {
    name=$1
    status=$2
    detail=$3
    printf '%s\t%s\t%s\n' "$name" "$status" "$detail" >>"$baseline_checks"
}

package_baseline() {
    [ "$#" -eq 3 ] || usage
    stage=$1
    candidate_deb=$2
    case "$stage" in pre|post) ;; *) usage ;; esac
    [ -f "$candidate_deb" ] || {
        echo "joy-pi-health: candidate Debian package is missing: $candidate_deb" >&2
        exit 1
    }
    candidate_deb=$(CDPATH= cd -- "$(dirname -- "$candidate_deb")" && pwd)/$(basename -- "$candidate_deb")
    prepare_evidence "$3"
    [ "$(id -u)" -eq 0 ] || {
        echo 'joy-pi-health: package baseline inspection must run as root for complete read-only evidence' >&2
        exit 1
    }
    for command_name in awk basename dpkg dpkg-deb dpkg-divert dpkg-query dpkg-statoverride find getent getfacl grep id passwd pgrep sed ss stat systemctl; do
        require_command "$command_name"
    done

    baseline_report="$evidence_dir/raw/package-baseline-$stage.txt"
    baseline_checks="$evidence_dir/raw/package-baseline-$stage-checks.tsv"
    baseline_summary="$evidence_dir/raw/package-baseline-$stage-summary.txt"
    for output in "$baseline_report" "$baseline_checks" "$baseline_summary"; do
        [ ! -e "$output" ] || {
            echo "joy-pi-health: refusing to overwrite package baseline evidence: $output" >&2
            exit 1
        }
    done
    : >"$baseline_checks"

    {
        record_command package-query dpkg-query -W -f='${db:Status-Abbrev} ${binary:Package} ${Version} ${Architecture}\n' joy-pi-health || true
        record_command package-selection dpkg --get-selections joy-pi-health || true
        record_command package-payload dpkg-deb --contents "$candidate_deb" || true
        record_command systemd-show systemctl show joy-pi-health.service -p LoadState -p ActiveState -p SubState -p UnitFileState -p FragmentPath || true
        record_command systemd-enabled systemctl is-enabled joy-pi-health.service || true
        record_command systemd-active systemctl is-active joy-pi-health.service || true
        record_command systemd-paths find /etc/systemd /run/systemd -xdev \( -iname '*joy-pi-health*' -o \( -type l -lname '*joy-pi-health*' \) \) -ls || true
        record_command helper-system-state find /var/lib/systemd/deb-systemd-helper-enabled -iname '*joy-pi-health*' -ls || true
        record_command helper-system-content grep -FR joy-pi-health /var/lib/systemd/deb-systemd-helper-enabled || true
        record_command helper-user-state find /var/lib/systemd/deb-systemd-user-helper-enabled -iname '*joy-pi-health*' -ls || true
        record_command helper-user-content grep -FR joy-pi-health /var/lib/systemd/deb-systemd-user-helper-enabled || true
        record_command dpkg-info-state find /var/lib/dpkg/info -maxdepth 1 -name 'joy-pi-health.*' -ls || true
        record_command dpkg-statoverrides dpkg-statoverride --list || true
        record_command dpkg-diversions dpkg-divert --list || true
        record_command dpkg-triggers grep -FR joy-pi-health /var/lib/dpkg/triggers || true
        record_command dpkg-pending-updates grep -FR joy-pi-health /var/lib/dpkg/updates || true
        record_command retained-passwd getent passwd _joy-pi-health || true
        record_command retained-group getent group _joy-pi-health || true
        record_command retained-lock passwd -S _joy-pi-health || true
        record_command retained-memberships id _joy-pi-health || true
        record_command retained-processes pgrep -a -u _joy-pi-health || true
        record_command listeners ss -H -ltn || true
        printf '\n===== firmware ACLs\n'
        for device in /dev/vcio /dev/vcio_gencmd; do
            if [ -e "$device" ]; then
                getfacl -cp "$device" || true
            else
                echo "$device absent"
            fi
        done
        printf '\n===== policy-rc.d\n'
        if [ -e /usr/sbin/policy-rc.d ]; then
            stat -Lc '%n owner=%U group=%G mode=%a' /usr/sbin/policy-rc.d
        else
            echo absent
        fi
    } >"$baseline_report" 2>&1

    if [ "$stage" = pre ]; then
        printf '%s\n' 'classification=INVENTORY_ONLY' >"$baseline_summary"
        echo 'package baseline pre-reset inventory completed'
        return
    fi

    package_query=$(dpkg-query -W joy-pi-health 2>&1) && package_query_rc=0 || package_query_rc=$?
    case "$package_query_rc" in
        0) baseline_check package_query_absent BLOCKED 'dpkg-query still has a package record' ;;
        1) baseline_check package_query_absent PASS 'no dpkg-query package record' ;;
        *) baseline_check package_query_absent BLOCKED "dpkg-query inspection failed: $package_query" ;;
    esac

    package_selection=$(dpkg --get-selections joy-pi-health 2>&1) && package_selection_rc=0 || package_selection_rc=$?
    if [ "$package_selection_rc" -ne 0 ]; then
        baseline_check selection_absent BLOCKED "dpkg selection inspection failed: $package_selection"
    elif printf '%s\n' "$package_selection" | grep -E '^joy-pi-health[[:space:]]' >/dev/null; then
        baseline_check selection_absent BLOCKED 'dpkg selection remains'
    else
        baseline_check selection_absent PASS 'no dpkg selection remains'
    fi

    payload_residue=
    while IFS= read -r payload_path; do
        case "$payload_path" in ''|./) continue ;; esac
        absolute_path=/${payload_path#./}
        if [ -e "$absolute_path" ] || [ -L "$absolute_path" ]; then
            payload_residue="$payload_residue $absolute_path"
        fi
    done <<EOF
$(dpkg-deb --contents "$candidate_deb" | awk '$1 !~ /^d/ || $6 ~ /joy-pi-health/ {print $6}')
EOF
    if [ -n "$payload_residue" ]; then
        baseline_check payload_absent BLOCKED "candidate payload remains:$payload_residue"
    else
        baseline_check payload_absent PASS 'candidate payload is absent'
    fi

    if [ ! -d /var/lib/dpkg/info ] || [ ! -r /var/lib/dpkg/info ]; then
        baseline_check dpkg_info_absent BLOCKED 'dpkg info directory is missing or unreadable'
    else
        dpkg_info_residue=$(find /var/lib/dpkg/info -maxdepth 1 -name 'joy-pi-health.*' -print) && dpkg_info_rc=0 || dpkg_info_rc=$?
        if [ "$dpkg_info_rc" -ne 0 ]; then
            baseline_check dpkg_info_absent BLOCKED 'dpkg info inspection failed'
        elif [ -n "$dpkg_info_residue" ]; then
            baseline_check dpkg_info_absent BLOCKED 'dpkg info files remain'
        else
            baseline_check dpkg_info_absent PASS 'no dpkg info files remain'
        fi
    fi

    dpkg_residue=
    dpkg_state_error=
    if [ ! -r /var/lib/dpkg/status ]; then
        dpkg_state_error=' /var/lib/dpkg/status'
    fi
    for state_file in /var/lib/dpkg/status /var/lib/dpkg/statoverride /var/lib/dpkg/diversions /var/lib/dpkg/triggers/File /var/lib/dpkg/triggers/Unincorp; do
        if [ -f "$state_file" ]; then
            if [ ! -r "$state_file" ]; then
                dpkg_state_error="$dpkg_state_error $state_file"
            elif grep -F joy-pi-health "$state_file" >/dev/null 2>&1; then
                dpkg_residue="$dpkg_residue $state_file"
            fi
        fi
    done
    if [ -d /var/lib/dpkg/updates ]; then
        if [ ! -r /var/lib/dpkg/updates ]; then
            dpkg_state_error="$dpkg_state_error /var/lib/dpkg/updates"
        else
            update_residue=$(grep -FRl joy-pi-health /var/lib/dpkg/updates 2>/dev/null) && update_rc=0 || update_rc=$?
            case "$update_rc" in
                0) dpkg_residue="$dpkg_residue /var/lib/dpkg/updates" ;;
                1) ;;
                *) dpkg_state_error="$dpkg_state_error /var/lib/dpkg/updates" ;;
            esac
        fi
    fi
    if [ -n "$dpkg_state_error" ]; then
        baseline_check dpkg_state_absent BLOCKED "dpkg state is unreadable:$dpkg_state_error"
    elif [ -n "$dpkg_residue" ]; then
        baseline_check dpkg_state_absent BLOCKED "dpkg state remains:$dpkg_residue"
    else
        baseline_check dpkg_state_absent PASS 'no package-specific dpkg state remains'
    fi

    load_state=$(systemctl show joy-pi-health.service -p LoadState --value 2>/dev/null || true)
    if [ "$load_state" = not-found ]; then
        baseline_check unit_not_found PASS 'systemd reports LoadState=not-found'
    else
        baseline_check unit_not_found BLOCKED "systemd LoadState=$load_state"
    fi

    enabled_state=$(systemctl is-enabled joy-pi-health.service 2>&1) && enabled_rc=0 || enabled_rc=$?
    if [ "$enabled_rc" -ne 0 ] && [ "$enabled_state" = not-found ]; then
        baseline_check unit_not_enabled PASS "is-enabled rejected absent unit: $enabled_state"
    else
        baseline_check unit_not_enabled BLOCKED "unexpected is-enabled result rc=$enabled_rc: $enabled_state"
    fi

    if [ ! -d /etc/systemd ] || [ ! -r /etc/systemd ] || [ ! -d /run/systemd ] || [ ! -r /run/systemd ]; then
        baseline_check systemd_state_absent BLOCKED 'systemd state roots are missing or unreadable'
    else
        systemd_residue=$(find /etc/systemd /run/systemd -xdev \( -iname '*joy-pi-health*' -o \( -type l -lname '*joy-pi-health*' \) \) -print 2>/dev/null) && systemd_rc=0 || systemd_rc=$?
        if [ "$systemd_rc" -ne 0 ]; then
            baseline_check systemd_state_absent BLOCKED 'systemd state inspection failed'
        elif [ -n "$systemd_residue" ]; then
            baseline_check systemd_state_absent BLOCKED 'systemd link, mask, alias, drop-in, or override remains'
        else
            baseline_check systemd_state_absent PASS 'no package-specific systemd state remains'
        fi
    fi

    helper_system=/var/lib/systemd/deb-systemd-helper-enabled
    if [ ! -d "$helper_system" ] || [ ! -r "$helper_system" ]; then
        baseline_check helper_system_absent BLOCKED 'system helper state directory is missing or unreadable'
    else
        helper_system_residue=$(find "$helper_system" \( -iname '*joy-pi-health*' -o \( -type l -lname '*joy-pi-health*' \) \) -print) && helper_system_rc=0 || helper_system_rc=$?
        helper_system_content=$(grep -FRl joy-pi-health "$helper_system" 2>/dev/null) && helper_system_content_rc=0 || helper_system_content_rc=$?
        if [ "$helper_system_rc" -ne 0 ] || [ "$helper_system_content_rc" -gt 1 ]; then
            baseline_check helper_system_absent BLOCKED 'system helper state inspection failed'
        elif [ -n "$helper_system_residue" ] || [ -n "$helper_system_content" ]; then
            baseline_check helper_system_absent BLOCKED 'system helper state remains'
        else
            baseline_check helper_system_absent PASS 'no system helper state remains'
        fi
    fi
    helper_user=/var/lib/systemd/deb-systemd-user-helper-enabled
    if [ ! -e "$helper_user" ]; then
        baseline_check helper_user_absent PASS 'user helper state directory is absent'
    elif [ ! -d "$helper_user" ] || [ ! -r "$helper_user" ]; then
        baseline_check helper_user_absent BLOCKED 'user helper state path is not a readable directory'
    else
        helper_user_residue=$(find "$helper_user" \( -iname '*joy-pi-health*' -o \( -type l -lname '*joy-pi-health*' \) \) -print) && helper_user_rc=0 || helper_user_rc=$?
        helper_user_content=$(grep -FRl joy-pi-health "$helper_user" 2>/dev/null) && helper_user_content_rc=0 || helper_user_content_rc=$?
        if [ "$helper_user_rc" -ne 0 ] || [ "$helper_user_content_rc" -gt 1 ]; then
            baseline_check helper_user_absent BLOCKED 'user helper state inspection failed'
        elif [ -n "$helper_user_residue" ] || [ -n "$helper_user_content" ]; then
            baseline_check helper_user_absent BLOCKED 'user helper state remains'
        else
            baseline_check helper_user_absent PASS 'no user helper state remains'
        fi
    fi

    acl_residue=
    acl_error=
    for device in /dev/vcio /dev/vcio_gencmd; do
        if [ -e "$device" ]; then
            if device_acl=$(getfacl -cp "$device" 2>/dev/null); then
                if printf '%s\n' "$device_acl" | grep -F 'user:_joy-pi-health:' >/dev/null; then
                    acl_residue="$acl_residue $device"
                fi
            else
                acl_error="$acl_error $device"
            fi
        fi
    done
    if [ -n "$acl_error" ]; then
        baseline_check acl_absent BLOCKED "firmware ACL inspection failed:$acl_error"
    elif [ -n "$acl_residue" ]; then
        baseline_check acl_absent BLOCKED "service ACL remains:$acl_residue"
    else
        baseline_check acl_absent PASS 'no service ACL remains'
    fi

    identity_present=false
    if identity=$(getent passwd _joy-pi-health); then
        identity_present=true
        identity_home=$(printf '%s\n' "$identity" | awk -F: '{print $6}')
        identity_shell=$(printf '%s\n' "$identity" | awk -F: '{print $7}')
        identity_gid=$(printf '%s\n' "$identity" | awk -F: '{print $4}')
        identity_group=$(getent group "$identity_gid" | awk -F: '{print $1}')
        identity_lock=$(passwd -S _joy-pi-health 2>/dev/null | awk '{print $2}')
        identity_groups=$(id -Gn _joy-pi-health 2>/dev/null)
        if [ "$identity_home" = /nonexistent ] && [ "$identity_shell" = /usr/sbin/nologin ] && \
           [ "$identity_group" = _joy-pi-health ] && [ "$identity_lock" = L ] && \
           [ "$identity_groups" = _joy-pi-health ]; then
            baseline_check identity_valid PASS 'retained service identity matches policy'
        else
            baseline_check identity_valid BLOCKED 'retained service identity differs from policy'
        fi
    else
        baseline_check identity_valid PASS 'service identity is absent'
    fi

    if [ "$identity_present" = false ]; then
        baseline_check process_absent PASS 'service identity is absent, so it owns no process'
    else
        pgrep -u _joy-pi-health >/dev/null 2>&1 && process_rc=0 || process_rc=$?
        case "$process_rc" in
            0) baseline_check process_absent BLOCKED 'a process owned by the service identity remains' ;;
            1) baseline_check process_absent PASS 'no process owned by the service identity remains' ;;
            *) baseline_check process_absent BLOCKED 'service process inspection failed' ;;
        esac
    fi
    if ! listeners=$(ss -H -ltn 2>&1); then
        baseline_check port_available BLOCKED "listener inspection failed: $listeners"
    elif printf '%s\n' "$listeners" | awk '$4 ~ /:8080$/ {found=1} END {exit !found}'; then
        baseline_check port_available BLOCKED 'a listener already occupies port 8080'
    else
        baseline_check port_available PASS 'port 8080 has no listener'
    fi
    if [ -e /usr/sbin/policy-rc.d ]; then
        baseline_check policy_recorded PASS 'policy-rc.d is present; automatic-start result requires BLOCKED review if denied'
    else
        baseline_check policy_recorded PASS 'policy-rc.d is absent'
    fi

    evaluator=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/package-baseline-evaluate.awk
    if awk -f "$evaluator" "$baseline_checks" >"$baseline_summary"; then
        echo 'package baseline post-reset verification passed'
    else
        echo 'joy-pi-health: package baseline is BLOCKED; review the retained evidence' >&2
        return 1
    fi
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
    package-baseline) package_baseline "$@" ;;
    rootless) rootless "$@" ;;
    managed) managed "$@" ;;
    validate-model) validate_model_file "$@" ;;
    *) usage ;;
esac
