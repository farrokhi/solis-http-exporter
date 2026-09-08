#!/bin/sh
# Runs before the files land, so the packaged ownership of /etc resolves.
set -e

user=solis-exporter

if getent passwd "$user" >/dev/null 2>&1; then
	exit 0
fi

if command -v systemd-sysusers >/dev/null 2>&1 &&
	systemd-sysusers --replace=/usr/lib/sysusers.d/solis-http-exporter.conf - <<-EOF
		u $user - "Solis HTTP exporter" - -
	EOF
then
	exit 0
fi

shell=/usr/sbin/nologin
[ -x "$shell" ] || shell=/sbin/nologin
[ -x "$shell" ] || shell=/bin/false

if command -v useradd >/dev/null 2>&1; then
	getent group "$user" >/dev/null 2>&1 || groupadd --system "$user"
	useradd --system --gid "$user" --no-create-home --home-dir /nonexistent \
		--shell "$shell" --comment "Solis HTTP exporter" "$user"
else
	getent group "$user" >/dev/null 2>&1 || addgroup -S "$user"
	adduser -S -D -H -G "$user" -s "$shell" "$user"
fi
