#!/bin/sh
# Run as root on the configured Ubuntu VIM4, with an ARM64 vim4-tagged binary.
set -eu
if [ "$(id -u)" != 0 ] || [ "$#" != 1 ]; then
    echo "usage: sudo $0 /path/to/vim4-kvm" >&2
    exit 1
fi
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
"$1" --version
install -m 0755 "$1" /usr/local/bin/vim4-kvm.new
mv -f /usr/local/bin/vim4-kvm.new /usr/local/bin/vim4-kvm
install -m 0644 "$base/vim4-kvm.service" /etc/systemd/system/
install -m 0644 "$base/99-vim4-kvm-hid.rules" /etc/udev/rules.d/
udevadm control --reload-rules
udevadm trigger --subsystem-match=hidg --action=change
udevadm settle
systemctl daemon-reload
systemctl enable vim4-kvm.service
systemctl restart vim4-kvm.service
