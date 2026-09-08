#!/bin/sh
set -e

# rpm passes 0 on uninstall and 1 on upgrade; dpkg passes remove or upgrade.
case "$1" in
0 | remove | purge)
	if command -v systemctl >/dev/null 2>&1; then
		systemctl --quiet is-active solis-http-exporter.service && \
			systemctl stop solis-http-exporter.service || true
		systemctl --quiet is-enabled solis-http-exporter.service && \
			systemctl disable solis-http-exporter.service || true
	fi
	;;
esac

exit 0
