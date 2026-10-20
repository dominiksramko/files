#!/bin/sh
# Tests docker-entrypoint.sh; run as root inside the image (the build does).
set -eu

entrypoint=/usr/local/bin/docker-entrypoint.sh
failures=0
check() { # check DESCRIPTION EXPECTED ACTUAL
	if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: want '$2', got '$3'"; failures=$((failures + 1)); fi
}
owner() { stat -c %u:%g "$1"; }

# A volume as a root-run image left it: everything owned by root.
setup() {
	rm -rf /tmp/entrypoint-test
	mkdir -p /tmp/entrypoint-test/data/thumbs/ab /tmp/entrypoint-test/data/hls /tmp/entrypoint-test/data/users/ann
	touch /tmp/entrypoint-test/data/files.db /tmp/entrypoint-test/data/files.db-wal \
		/tmp/entrypoint-test/data/thumbs/ab/1.jpg /tmp/entrypoint-test/data/users/ann/root-owned.txt
}
run() { # run COMMAND... with the test volume's paths
	DB_PATH=/tmp/entrypoint-test/data/files.db \
		THUMB_CACHE_DIR=/tmp/entrypoint-test/data/thumbs \
		HLS_CACHE_DIR=/tmp/entrypoint-test/data/hls \
		CONV_CACHE_DIR=/tmp/entrypoint-test/data/conv \
		USERS_ROOT=/tmp/entrypoint-test/data/users \
		"$entrypoint" "$@"
}

setup
check "server runs as 1000:1000 by default" "1000:1000" "$(run sh -c 'echo $(id -u):$(id -g)')"
check "database handed over" "1000:1000" "$(owner /tmp/entrypoint-test/data/files.db)"
check "database WAL handed over" "1000:1000" "$(owner /tmp/entrypoint-test/data/files.db-wal)"
check "data folder handed over" "1000:1000" "$(owner /tmp/entrypoint-test/data)"
check "cache contents handed over" "1000:1000" "$(owner /tmp/entrypoint-test/data/thumbs/ab/1.jpg)"
check "user folders untouched" "0:0" "$(owner /tmp/entrypoint-test/data/users/ann/root-owned.txt)"
check "users folder handed over (new users' folders go there)" "1000:1000" "$(owner /tmp/entrypoint-test/data/users)"
check "a user's folder inside untouched" "0:0" "$(owner /tmp/entrypoint-test/data/users/ann)"
check "server can write its data" "written" "$(run sh -c 'touch /tmp/entrypoint-test/data/files.db-shm /tmp/entrypoint-test/data/thumbs/new.jpg && mkdir /tmp/entrypoint-test/data/conv && echo written')"

setup
check "PUID and PGID respected" "1234:2345" "$(PUID=1234 PGID=2345 run sh -c 'echo $(id -u):$(id -g)')"
check "...and used for the data" "1234:2345" "$(owner /tmp/entrypoint-test/data/files.db)"

setup
check "started as a user: runs as that user" "1500" "$(su-exec 1500:1500 "$entrypoint" id -u)"

rm -rf /tmp/entrypoint-test
[ "$failures" = 0 ] || { echo "$failures failed"; exit 1; }
