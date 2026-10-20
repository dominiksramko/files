#!/bin/sh
# Starts the server as PUID:PGID (default 1000:1000) instead of root.
#
# It begins as root only to hand the app's own data to that user: the
# database, its folder, the thumbnail, video and conversion caches, and the
# users folder itself (where new users' folders are created) unless it's a
# mount. On a new volume there's nothing to do; after an upgrade from an image
# that ran as root it's a one-time fix. Folders mounted for users are never
# touched (they may sit on the same disk, so nothing under the data or users
# folder is changed recursively) — PUID:PGID must be able to read and write them.
set -eu

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# Already started as some user (e.g. `user:` in compose): nothing to hand over.
if [ "$(id -u)" != 0 ] || [ "$PUID" = 0 ]; then
	exec "$@"
fi

# give_to_user PATH [find options] — gives what root owns at PATH to PUID:PGID.
give_to_user() {
	path="$1"
	shift
	if [ -e "$path" ]; then
		find "$path" "$@" -user 0 -exec chown "$PUID:$PGID" {} +
	fi
}

db_path="${DB_PATH:-./data/files.db}"
give_to_user "$(dirname "$db_path")" -maxdepth 0
for db_file in "$db_path" "$db_path-wal" "$db_path-shm" "$db_path-journal"; do
	give_to_user "$db_file" -maxdepth 0
done
for cache_dir in "${THUMB_CACHE_DIR:-./data/thumbs}" "${HLS_CACHE_DIR:-./data/hls}" "${CONV_CACHE_DIR:-./data/conv}"; do
	give_to_user "$cache_dir" -xdev
done
users_root="${USERS_ROOT:-./data/users}"
if ! mountpoint -q "$users_root" 2>/dev/null; then
	give_to_user "$users_root" -maxdepth 0
fi

exec su-exec "$PUID:$PGID" "$@"
