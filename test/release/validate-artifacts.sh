#!/bin/sh
set -eu

artifact_dir=${1:-dist}
version=0.1.0-1
upstream_version=0.1.0
deb="$artifact_dir/joy-pi-health_${version}_arm64.deb"
archive="$artifact_dir/joy-pi-health-v${upstream_version}-linux-arm64.tar.gz"
metadata="$artifact_dir/joy-pi-health-v${upstream_version}-release.json"
checksums="$artifact_dir/SHA256SUMS"

for command_name in cmp dash dpkg-deb file find grep jq lintian readelf sha256sum sort stat systemd-analyze tar; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        echo "joy-pi-health: required validation command is missing: $command_name" >&2
        exit 1
    fi
done

actual_files=$(find "$artifact_dir" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | sort)
expected_files=$(printf '%s\n' \
    SHA256SUMS \
    "joy-pi-health-v${upstream_version}-linux-arm64.tar.gz" \
    "joy-pi-health-v${upstream_version}-release.json" \
    "joy-pi-health_${version}_arm64.deb" | sort)
if [ "$actual_files" != "$expected_files" ]; then
    echo "joy-pi-health: unexpected release artifact set" >&2
    printf '%s\n' "$actual_files" >&2
    exit 1
fi

(
    cd "$artifact_dir"
    sha256sum --check SHA256SUMS
)

test "$(dpkg-deb --field "$deb" Package)" = joy-pi-health
test "$(dpkg-deb --field "$deb" Version)" = "$version"
test "$(dpkg-deb --field "$deb" Architecture)" = arm64
depends=$(dpkg-deb --field "$deb" Depends)
printf '%s\n' "$depends" | grep -F 'init-system-helpers (>= 1.60)' >/dev/null
printf '%s\n' "$depends" | grep -F 'systemd (>= 254)' >/dev/null
if printf '%s\n' "$depends" | grep -E '(^|, )(acl|udev)(,|$)' >/dev/null; then
    echo "joy-pi-health: default package unexpectedly enables ACL dependencies" >&2
    exit 1
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
package_root="$work/package"
control_root="$work/control"
archive_root="$work/archive"
mkdir -p "$package_root" "$control_root" "$archive_root"
dpkg-deb --extract "$deb" "$package_root"
dpkg-deb --control "$deb" "$control_root"
tar -xzf "$archive" -C "$archive_root"

package_files=$(cd "$package_root" && find . -type f -printf '%P\n' | sort)
expected_package_files=$(printf '%s\n' \
    usr/bin/joy-pi-health \
    usr/lib/systemd/system/joy-pi-health.service \
    usr/lib/sysusers.d/joy-pi-health.conf \
    usr/share/lintian/overrides/joy-pi-health \
    usr/share/doc/joy-pi-health/LICENSE \
    usr/share/doc/joy-pi-health/README.md \
    usr/share/doc/joy-pi-health/changelog.Debian.gz \
    usr/share/doc/joy-pi-health/copyright \
    usr/share/doc/joy-pi-health/docs/architecture-v0.1.md \
    usr/share/doc/joy-pi-health/docs/http-api-v0.1.md \
    usr/share/doc/joy-pi-health/docs/requirements-v0.1.md \
    usr/share/doc/joy-pi-health/release-metadata.json | sort)
if [ "$package_files" != "$expected_package_files" ]; then
    echo "joy-pi-health: unexpected Debian payload" >&2
    printf '%s\n' "$package_files" >&2
    exit 1
fi

test ! -e "$package_root/etc"
test ! -e "$package_root/var"
test ! -e "$control_root/conffiles"
test "$(stat -c %a "$package_root/usr/bin/joy-pi-health")" = 755
for script_name in postinst prerm postrm; do
    test "$(stat -c %a "$control_root/$script_name")" = 755
    dash -n "$control_root/$script_name"
done
if dpkg-deb --contents "$deb" | awk '$2 != "root/root" { exit 1 }'; then
    :
else
    echo "joy-pi-health: Debian payload contains non-root ownership" >&2
    exit 1
fi

unit="$package_root/usr/lib/systemd/system/joy-pi-health.service"
for setting in \
    Type=notify NotifyAccess=main ExecStart=/usr/bin/joy-pi-health \
    User=_joy-pi-health Group=_joy-pi-health SupplementaryGroups=video \
    Restart=on-failure RestartPreventExitStatus=2 RestartSec=1s RestartSteps=5 \
    RestartMaxDelaySec=30s TimeoutStopSec=2s NoNewPrivileges=yes \
    ProtectSystem=strict ProtectHome=yes MemoryDenyWriteExecute=yes \
    CapabilityBoundingSet= AmbientCapabilities= DevicePolicy=closed \
    'DeviceAllow=/dev/vcio r' 'DeviceAllow=/dev/vcio_gencmd r' UMask=0077; do
    grep -Fx "$setting" "$unit" >/dev/null
done
test "$(grep -c '^DeviceAllow=' "$unit")" = 2
test ! -e "$package_root/usr/lib/udev/rules.d/70-joy-pi-health-vcio-acl.rules"

grep -F 'deb-systemd-helper enable' "$control_root/postinst" >/dev/null
grep -F 'deb-systemd-invoke start' "$control_root/postinst" >/dev/null
grep -F 'deb-systemd-invoke try-restart' "$control_root/postinst" >/dev/null
grep -F 'deb-systemd-invoke stop' "$control_root/prerm" >/dev/null
grep -F 'deb-systemd-helper disable' "$control_root/prerm" >/dev/null
grep -F 'deb-systemd-helper purge' "$control_root/postrm" >/dev/null
grep -F '[ "${1:-}" = purge ]' "$control_root/postrm" >/dev/null
if grep -E 'systemctl( --system)? (start|stop|restart|try-restart|enable|disable)( |$)' "$control_root"/postinst "$control_root"/prerm "$control_root"/postrm >/dev/null; then
    echo "joy-pi-health: maintainer script bypasses Debian service policy helpers" >&2
    exit 1
fi

grep -Fx 'u _joy-pi-health - "Joy Pi Health service" /nonexistent /usr/sbin/nologin' \
    "$package_root/usr/lib/sysusers.d/joy-pi-health.conf" >/dev/null

verify_units="$work/verify-units"
mkdir -p "$verify_units"
sed "s#^ExecStart=/usr/bin/joy-pi-health\$#ExecStart=$package_root/usr/bin/joy-pi-health#" \
    "$unit" >"$verify_units/joy-pi-health.service"
SYSTEMD_UNIT_PATH="$verify_units:" systemd-analyze verify joy-pi-health.service
lintian --fail-on error "$deb"
file "$package_root/usr/bin/joy-pi-health" | grep -E 'ELF 64-bit.*(ARM aarch64|ARM64)' >/dev/null
if readelf -l "$package_root/usr/bin/joy-pi-health" | grep -F 'Requesting program interpreter' >/dev/null; then
    echo "joy-pi-health: release executable is dynamically linked" >&2
    exit 1
fi

archive_directory="$archive_root/joy-pi-health-v${upstream_version}-linux-arm64"
archive_files=$(cd "$archive_directory" && find . -type f -printf '%P\n' | sort)
expected_archive_files=$(printf '%s\n' \
    LICENSE README.md \
    docs/architecture-v0.1.md docs/http-api-v0.1.md docs/requirements-v0.1.md \
    joy-pi-health release-metadata.json | sort)
if [ "$archive_files" != "$expected_archive_files" ]; then
    echo "joy-pi-health: unexpected portable archive payload" >&2
    printf '%s\n' "$archive_files" >&2
    exit 1
fi
cmp "$package_root/usr/bin/joy-pi-health" "$archive_directory/joy-pi-health"
cmp "$package_root/usr/share/doc/joy-pi-health/release-metadata.json" "$archive_directory/release-metadata.json"
cmp "$metadata" "$archive_directory/release-metadata.json"

test "$(jq -r .project "$metadata")" = joy-pi-health
test "$(jq -r .version "$metadata")" = "$upstream_version"
test "$(jq -r .debian_version "$metadata")" = "$version"
test "$(jq -r .commit "$metadata")" = "$(git rev-parse HEAD)"
test "$(jq -r .go_version "$metadata")" = go1.27.1
test "$(jq -r .target "$metadata")" = linux/arm64/v8.0
test "$(jq -r .cgo_enabled "$metadata")" = false
test "$(jq -r .firmware_access "$metadata")" = video
test "$(jq -r .module "$metadata")" = github.com/Adrien-hue/joy-pi-health
test "$(jq -r '.third_party_dependencies | length' "$metadata")" = 0
test "$(jq -r '.licenses | join(",")' "$metadata")" = MIT
