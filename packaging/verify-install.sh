#!/bin/sh
# Installs a built package and checks the service account can actually run the
# exporter. Run inside a throwaway container: verify-install.sh deb|rpm|apk DIR
set -e

format=${1:?usage: verify-install.sh deb|rpm|apk PACKAGE_DIR}
dir=${2:?usage: verify-install.sh deb|rpm|apk PACKAGE_DIR}
user=solis-exporter
confdir=/etc/solis-http-exporter

case "$(uname -m)" in
aarch64 | arm64) arch=arm64 ;;
x86_64) arch=amd64 ;;
*)
	echo "unsupported arch $(uname -m)"
	exit 1
	;;
esac

pkg=$(ls "$dir"/*_linux_"$arch"."$format")
echo "installing $pkg"
case "$format" in
deb) dpkg -i "$pkg" ;;
rpm) rpm -i "$pkg" ;;
apk) apk add --allow-untrusted --quiet "$pkg" ;;
esac

fail() {
	echo "FAIL: $1"
	exit 1
}

getent passwd "$user" >/dev/null || fail "$user was not created"
test -f /usr/lib/systemd/system/solis-http-exporter.service || fail "unit not installed"

group=$(stat -c '%G' "$confdir")
mode=$(stat -c '%a' "$confdir")
[ "$group" = "$user" ] || fail "$confdir group is $group, want $user"
[ "$mode" = "750" ] || fail "$confdir mode is $mode, want 750"

group=$(stat -c '%G' "$confdir/config.yml")
mode=$(stat -c '%a' "$confdir/config.yml")
[ "$group" = "$user" ] || fail "config.yml group is $group, want $user"
[ "$mode" = "640" ] || fail "config.yml mode is $mode, want 640"

# The point of the whole exercise: the service account reads its own secrets.
printf 'secret' >"$confdir/house.password"
chgrp "$user" "$confdir/house.password"
chmod 0640 "$confdir/house.password"
cat >"$confdir/config.yml" <<EOF
inverters:
  - name: house
    address: 127.0.0.1
    port: 9
    password_file: $confdir/house.password
    timeout: 1s
EOF
chgrp "$user" "$confdir/config.yml"

cmd="timeout 3 /usr/bin/solis_http_exporter --config.file=$confdir/config.yml --web.listen-address=127.0.0.1:9613"
if command -v su >/dev/null 2>&1; then
	su -s /bin/sh "$user" -c "$cmd" >/tmp/out 2>&1 || true
elif command -v runuser >/dev/null 2>&1; then
	runuser -u "$user" -- sh -c "$cmd" >/tmp/out 2>&1 || true
else
	chroot --userspec="$user:$user" / sh -c "$cmd" >/tmp/out 2>&1 || true
fi

head -1 /tmp/out
grep -q 'starting exporter' /tmp/out || fail "could not start as $user"
echo "OK: $format installs and runs as $user"
