#!/bin/sh
set -eu

version=0.1.0-1
upstream_version=0.1.0
firmware_access=video

usage() {
    echo "usage: debian/build-release.sh [--firmware-access video|acl]" >&2
    exit 2
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --firmware-access)
            [ "$#" -ge 2 ] || usage
            firmware_access=$2
            shift 2
            ;;
        *) usage ;;
    esac
done

case "$firmware_access" in
    video|acl) ;;
    *) usage ;;
esac

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

if [ -n "$(git status --porcelain)" ]; then
    echo "joy-pi-health: release builds require a clean worktree" >&2
    exit 1
fi

actual_go_version=$(go env GOVERSION)
if [ "$actual_go_version" != go1.27.1 ]; then
    echo "joy-pi-health: Go 1.27.1 is required; found $actual_go_version" >&2
    exit 1
fi

for command_name in git go dpkg-deb tar gzip sha256sum sed find touch cmp install date mktemp; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        echo "joy-pi-health: required build command is missing: $command_name" >&2
        exit 1
    fi
done

export LC_ALL=C
export TZ=UTC
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=arm64
export GOARM64=v8.0
export GOENV=off
export GOFLAGS=
export GOTOOLCHAIN=local

commit=$(git rev-parse HEAD)
source_date_epoch=${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct HEAD)}
export SOURCE_DATE_EPOCH="$source_date_epoch"
created_at=$(date -u -d "@$source_date_epoch" '+%Y-%m-%dT%H:%M:%SZ')
go_version=$actual_go_version

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$root/dist" "$work/build-one" "$work/build-two"

go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$work/build-one/joy-pi-health" ./cmd/joy-pi-health
go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$work/build-two/joy-pi-health" ./cmd/joy-pi-health
if ! cmp -s "$work/build-one/joy-pi-health" "$work/build-two/joy-pi-health"; then
    echo "joy-pi-health: consecutive release builds differ" >&2
    exit 1
fi

metadata="$work/release-metadata.json"
cat >"$metadata" <<EOF
{
  "project": "joy-pi-health",
  "version": "$upstream_version",
  "debian_version": "$version",
  "commit": "$commit",
  "go_version": "$go_version",
  "target": "linux/arm64/v8.0",
  "cgo_enabled": false,
  "build_mode": "pure-go static executable",
  "created_at": "$created_at",
  "firmware_access": "$firmware_access",
  "module": "github.com/Adrien-hue/joy-pi-health",
  "third_party_dependencies": [],
  "licenses": ["MIT"]
}
EOF

package_root="$work/package"
mkdir -p "$package_root/DEBIAN" \
    "$package_root/usr/bin" \
    "$package_root/usr/lib/systemd/system" \
    "$package_root/usr/lib/sysusers.d" \
    "$package_root/usr/share/lintian/overrides" \
    "$package_root/usr/share/doc/joy-pi-health/docs"

acl_depends=
supplementary_groups=SupplementaryGroups=video
if [ "$firmware_access" = acl ]; then
    acl_depends=', acl, udev'
    supplementary_groups=
    mkdir -p "$package_root/usr/lib/udev/rules.d"
    install -m 0644 debian/firmware-acl/70-joy-pi-health-vcio-acl.rules \
        "$package_root/usr/lib/udev/rules.d/70-joy-pi-health-vcio-acl.rules"
fi

sed -e "s/@VERSION@/$version/g" -e "s/@ACL_DEPENDS@/$acl_depends/g" \
    debian/control.in >"$package_root/DEBIAN/control"
sed "s/@SUPPLEMENTARY_GROUPS@/$supplementary_groups/" \
    debian/joy-pi-health.service.in >"$package_root/usr/lib/systemd/system/joy-pi-health.service"

install -m 0755 "$work/build-one/joy-pi-health" "$package_root/usr/bin/joy-pi-health"
sed "s/@FIRMWARE_ACCESS@/$firmware_access/g" debian/postinst >"$package_root/DEBIAN/postinst"
install -m 0755 debian/prerm "$package_root/DEBIAN/prerm"
sed "s/@FIRMWARE_ACCESS@/$firmware_access/g" debian/postrm >"$package_root/DEBIAN/postrm"
chmod 0755 "$package_root/DEBIAN/postinst" "$package_root/DEBIAN/postrm"
install -m 0644 debian/joy-pi-health.sysusers "$package_root/usr/lib/sysusers.d/joy-pi-health.conf"
install -m 0644 debian/copyright "$package_root/usr/share/doc/joy-pi-health/copyright"
gzip -n -9 -c debian/changelog >"$package_root/usr/share/doc/joy-pi-health/changelog.Debian.gz"
chmod 0644 "$package_root/usr/share/doc/joy-pi-health/changelog.Debian.gz"
install -m 0644 debian/lintian-overrides "$package_root/usr/share/lintian/overrides/joy-pi-health"
install -m 0644 README.md LICENSE "$metadata" "$package_root/usr/share/doc/joy-pi-health/"
install -m 0644 docs/requirements-v0.1.md docs/http-api-v0.1.md docs/architecture-v0.1.md \
    "$package_root/usr/share/doc/joy-pi-health/docs/"

find "$package_root" -exec touch -d "@$source_date_epoch" {} +
deb_path="$root/dist/joy-pi-health_${version}_arm64.deb"
dpkg-deb --root-owner-group --uniform-compression --compression=gzip --compression-level=9 \
    --build "$package_root" "$deb_path"

archive_root="$work/joy-pi-health-v${upstream_version}-linux-arm64"
mkdir -p "$archive_root/docs"
install -m 0755 "$work/build-one/joy-pi-health" "$archive_root/joy-pi-health"
install -m 0644 README.md LICENSE "$metadata" "$archive_root/"
install -m 0644 docs/requirements-v0.1.md docs/http-api-v0.1.md docs/architecture-v0.1.md "$archive_root/docs/"
find "$archive_root" -exec touch -d "@$source_date_epoch" {} +

tar_path="$root/dist/joy-pi-health-v${upstream_version}-linux-arm64.tar.gz"
tar --sort=name --mtime="@$source_date_epoch" --owner=0 --group=0 --numeric-owner \
    -C "$work" -cf - "$(basename "$archive_root")" | gzip -n >"$tar_path"

extracted="$work/extracted"
mkdir -p "$extracted"
dpkg-deb --extract "$deb_path" "$extracted"
if ! cmp -s "$extracted/usr/bin/joy-pi-health" "$archive_root/joy-pi-health"; then
    echo "joy-pi-health: Debian and archive executables differ" >&2
    exit 1
fi

metadata_path="$root/dist/joy-pi-health-v${upstream_version}-release.json"
install -m 0644 "$metadata" "$metadata_path"
(
    cd "$root/dist"
    sha256sum \
        "$(basename "$deb_path")" \
        "$(basename "$tar_path")" \
        "$(basename "$metadata_path")" >SHA256SUMS
)

echo "release artifacts written to $root/dist"
