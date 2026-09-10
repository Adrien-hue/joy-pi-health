#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
validator="$script_dir/verify-readonly.sh"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
fixture="$work/model"

accept() {
    label=$1
    format=$2
    printf '%b' "$format" >"$fixture"
    if ! sh "$validator" validate-model "$fixture"; then
        echo "joy-pi-health: expected accepted model: $label" >&2
        exit 1
    fi
}

reject() {
    label=$1
    format=$2
    printf '%b' "$format" >"$fixture"
    if sh "$validator" validate-model "$fixture"; then
        echo "joy-pi-health: expected rejected model: $label" >&2
        exit 1
    fi
}

accept canonical 'Raspberry Pi 3 Model B Plus'
accept canonical-nul 'Raspberry Pi 3 Model B Plus\000'
accept known-reference 'Raspberry Pi 3 Model B Plus Rev 1.3\000'

reject pi-3b 'Raspberry Pi 3 Model B\000'
reject pi-4 'Raspberry Pi 4 Model B Rev 1.4\000'
reject pi-5 'Raspberry Pi 5 Model B Rev 1.0\000'
reject empty ''
reject malformed-revision 'Raspberry Pi 3 Model B Plus Rev 1.x\000'
reject missing-revision 'Raspberry Pi 3 Model B Plus Rev \000'
reject arbitrary-suffix 'Raspberry Pi 3 Model B Plus unexpected\000'
reject leading-space ' Raspberry Pi 3 Model B Plus\000'
reject trailing-space 'Raspberry Pi 3 Model B Plus \000'
reject trailing-newline 'Raspberry Pi 3 Model B Plus\n\000'
reject embedded-nul 'Raspberry Pi 3 Model B\000 Plus'

if sh "$validator" validate-model "$work/missing-model"; then
    echo 'joy-pi-health: expected missing model file to be rejected' >&2
    exit 1
fi

grep -F 'platform is not a Raspberry Pi 3 Model B Plus' "$validator" >/dev/null

echo 'Raspberry Pi 3B+ model validation tests passed'
