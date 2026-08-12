#!/bin/sh
set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
	printf 'usage: %s <workspace> [duration]\n' "$0" >&2
	exit 2
fi

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd -P)
workspace=$(CDPATH='' cd -- "$1" && pwd -P)
duration=${2:-24h}
config=$workspace/.rungrid.yaml
utc_date=$(date -u +%Y-%m-%d)
evidence_root=${RUNGRID_SOAK_EVIDENCE_ROOT:-"$repository_root/tmp/$utc_date/rungrid-resource-guard-soak"}
temporary_directory=$(mktemp -d /tmp/rungrid-soak.XXXXXX)

cleanup() {
	rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$evidence_root"
run=1
while ! mkdir "$evidence_root/$run" 2>/dev/null; do
	run=$((run + 1))
done
evidence=$evidence_root/$run

(cd "$repository_root" && go build -o "$temporary_directory/rungrid" .)
(cd "$repository_root" && go build -o "$temporary_directory/soak" ./tests/live-integration/resource-guard-soak)

set -- \
	--rungrid "$temporary_directory/rungrid" \
	--config "$config" \
	--duration "$duration" \
	--evidence "$evidence"
if [ -n "${RUNGRID_SOAK_STATE_DIR:-}" ]; then
	set -- "$@" --state-dir "$RUNGRID_SOAK_STATE_DIR"
fi
if [ "${RUNGRID_SOAK_OPEN_WORKSPACE:-0}" = "1" ]; then
	set -- "$@" --open-workspace
fi
"$temporary_directory/soak" "$@"
