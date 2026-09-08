#!/bin/sh
set -e

# The solis-exporter user is left behind on purpose; files may still own to it.
command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload >/dev/null 2>&1 || true

exit 0
