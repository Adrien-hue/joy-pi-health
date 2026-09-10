#!/bin/sh
set -eu

usage() {
    echo "usage: test/release/pi3bplus/derive-lifecycle-fixture.sh CANDIDATE_DEB OUTPUT_DIRECTORY" >&2
    exit 2
}

[ "$#" -eq 2 ] || usage
candidate=$1
output_dir=$2
fixture_version=0.1.0~classc1-1
fixture_name="joy-pi-health_${fixture_version}_arm64.deb"

for command_name in awk basename cmp diff dpkg-deb find grep mktemp mv sed sha256sum sort; do
    command -v "$command_name" >/dev/null 2>&1 || {
        echo "joy-pi-health: required lifecycle-fixture command is missing: $command_name" >&2
        exit 1
    }
done

[ -f "$candidate" ] || {
    echo "joy-pi-health: candidate package does not exist: $candidate" >&2
    exit 1
}
[ "$(dpkg-deb --field "$candidate" Package)" = joy-pi-health ]
[ "$(dpkg-deb --field "$candidate" Version)" = 0.1.0-1 ]
[ "$(dpkg-deb --field "$candidate" Architecture)" = arm64 ]

mkdir -p "$output_dir"
fixture="$output_dir/$fixture_name"
[ ! -e "$fixture" ] || {
    echo "joy-pi-health: refusing to overwrite lifecycle fixture: $fixture" >&2
    exit 1
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
original="$work/original"
rebuilt="$work/rebuilt"
mkdir -p "$original" "$rebuilt"
dpkg-deb --raw-extract "$candidate" "$original"

sed "s/^Version: 0\.1\.0-1$/Version: $fixture_version/" \
    "$original/DEBIAN/control" >"$work/control"
grep -Fx "Version: $fixture_version" "$work/control" >/dev/null
mv "$work/control" "$original/DEBIAN/control"

dpkg-deb --root-owner-group --uniform-compression --compression=gzip --compression-level=9 \
    --build "$original" "$fixture" >/dev/null

[ "$(dpkg-deb --field "$fixture" Package)" = joy-pi-health ]
[ "$(dpkg-deb --field "$fixture" Version)" = "$fixture_version" ]
[ "$(dpkg-deb --field "$fixture" Architecture)" = arm64 ]

dpkg-deb --raw-extract "$fixture" "$rebuilt"
diff -qr --exclude=DEBIAN "$original" "$rebuilt" >/dev/null

for script_name in postinst prerm postrm; do
    cmp "$original/DEBIAN/$script_name" "$rebuilt/DEBIAN/$script_name"
done
cmp "$original/usr/bin/joy-pi-health" "$rebuilt/usr/bin/joy-pi-health"

original_controls=$(find "$original/DEBIAN" -mindepth 1 -maxdepth 1 -type f ! -name control -printf '%f\n' | sort)
rebuilt_controls=$(find "$rebuilt/DEBIAN" -mindepth 1 -maxdepth 1 -type f ! -name control -printf '%f\n' | sort)
[ "$original_controls" = "$rebuilt_controls" ]

sed 's/^Version: .*/Version: @VERSION@/' "$original/DEBIAN/control" >"$work/original.control"
sed 's/^Version: .*/Version: @VERSION@/' "$rebuilt/DEBIAN/control" >"$work/rebuilt.control"
cmp "$work/original.control" "$work/rebuilt.control"

{
    echo "classification=Class C lifecycle fixture; not a release candidate"
    echo "source_package=$(basename "$candidate")"
    echo "source_sha256=$(sha256sum "$candidate" | awk '{print $1}')"
    echo "fixture_package=$fixture_name"
    echo "fixture_sha256=$(sha256sum "$fixture" | awk '{print $1}')"
    echo "changed_field=DEBIAN/control Version"
    echo "payload_identity=verified"
    echo "maintainer_script_identity=verified"
} >"$output_dir/${fixture_name}.provenance.txt"

echo "lifecycle fixture written to $fixture"
