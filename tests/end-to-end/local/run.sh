#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../../.." && pwd -P)
utc_date=$(date -u +%Y-%m-%d)
evidence_root=${RUNGRID_EVIDENCE_ROOT:-"$repository_root/tmp/$utc_date/rungrid-headless-e2e"}
output_limit=${RUNGRID_EVIDENCE_LIMIT_BYTES:-67108864}
helper=${RUNGRID_EVIDENCE_HELPER:-}
temporary_helper=
helper_pid=

cleanup() {
	if [ -n "$temporary_helper" ]; then
		rm -f "$temporary_helper"
	fi
}

forward_signal() {
	signal=$1
	status=$2
	if [ -n "$helper_pid" ]; then
		kill -"$signal" "$helper_pid" 2>/dev/null || true
		wait "$helper_pid" 2>/dev/null || true
	fi
	cleanup
	exit "$status"
}

trap 'forward_signal HUP 129' HUP
trap 'forward_signal INT 130' INT
trap 'forward_signal TERM 143' TERM
trap cleanup EXIT

if [ -z "$helper" ]; then
	temporary_helper=${TMPDIR:-/tmp}/rungrid-evidence-helper.$$
	(cd "$repository_root" && go build -o "$temporary_helper" ./tests/evidencecapture)
	helper=$temporary_helper
fi

"$helper" \
	--repository-root "$repository_root" \
	--evidence-root "$evidence_root" \
	--output-limit-bytes "$output_limit" \
	-- env RUNGRID_E2E=1 RUNGRID_E2E_SUSTAINED=1 go test -run 'Test((Headless|TabOnly)Lifecycle|RepositoryMaintenance|ResourceGuard(Sustained)?)EndToEnd' -count=1 -v ./tests/end-to-end/local &
helper_pid=$!
set +e
wait "$helper_pid"
exit_code=$?
set -e
helper_pid=
exit "$exit_code"
