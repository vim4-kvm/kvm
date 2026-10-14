# VIM4 integration

The `vim4` build runs the JetKVM application and its unchanged device UI on
Khadas VIM4 Ubuntu ARM64. Board services own HDMI setup, USB gadget creation and
USB-C PD negotiation; Ubuntu owns the network stack, clock and SSH. Preserve
licenses and existing authorship.

## Build and install

The workspace builds and packages this application as the `vim4-jetkvm` Ubuntu
component (see the workspace `docs/board-tools.md`):

```sh
python3 tools/workspace.py build jetkvm     # on Linux ARM64 or in the builder
```

The build embeds the package version (`PACKAGE_VERSION`) as the app version;
the update page compares it with the release server's version.

The package installs `/opt/vim4-jetkvm/jetkvm`, `vim4-kvm.service`, the root
helper `/usr/libexec/vim4-jetkvm` and its sudoers rule, but does not enable the
service. Select JetKVM in the vim4-manager dashboard (port 80): it disables and
stops `nanokvm.service`, then enables and starts this one. The units declare
Conflicts= with each other because both use HDMI capture, the encoder, the HID
endpoints and port **8080**. The HID udev rule comes from the vim4-nanokvm package.

Configuration lives in `/var/lib/jetkvm/config.json` (systemd StateDirectory);
images, TLS certificates and downloaded updates live beside it.

Prerequisites: the vim4-vfmcap and vim4-amlvenc packages, vim4-kvm-hdmi.service,
vim4-kvm-gadget.service, vim4-kvm-pd.service, and the eight/six/four-byte HID
descriptors and `mass_storage.disk0` function created by the board gadget script.

The application runs as **khadas** with video, input, audio and systemd-journal
groups and CAP_NET_BIND_SERVICE for the optional HTTPS listener on 443. The
vendor encoder uses `/tmp/vpu_mutex.map`: use the same account for encoder
tests and do not delete the lock while an encoder is active or give the service
a private /tmp.

## Privileged operations

Root changes go through `sudo -n /usr/libexec/vim4-jetkvm OPERATION` with a
JSON request on stdin, so credentials never appear in process arguments. The
helper validates every request itself; sudoers allows only this program.

| Operation | Effect |
| --- | --- |
| media-mount, media-eject | Opens a validated image from `/var/lib/jetkvm/images` without following links and hands its descriptor to the LUN; exchanges NCM for mass storage and back (six IN endpoints fail on this controller). Images are always read-only, as on JetKVM |
| usb-emulation | Binds or unbinds the board gadget's UDC |
| edid-apply, hotplug | Loads an EDID into HDMI RX port B (both 1.4 and 2.0 slots, after tvserver's boot load); skips a table already loaded in this tvserver run. Hot-plug low hides the display |
| network-get, network-set, dhcp-renew | Reads and changes the active eth0 NetworkManager profile; a failed reapply restores the previous profile |
| hostname-set, mdns-set | hostnamectl; Avahi enable/disable and IPv4/IPv6 families |
| wifi-get, wifi-scan, wifi-save, wifi-connect, wifi-remove | NetworkManager Wi-Fi profiles; secrets go to a root-only keyfile |
| tailscale-login | Starts `tailscale login --login-server=…`; Ubuntu's tailscale has no `set --login-server` |
| reboot | Delayed `systemctl reboot` after sync |
| update-install | Installs a downloaded vim4-jetkvm package after an APT dependency simulation, then `dpkg --verify` |

`python3 -m unittest discover -s deploy/vim4` tests the helper without root.

## Feature mapping

VIM4's video settings also expose a 1000–60000 kbps custom bitrate input.
Existing quality presets remain 12/6/1.2 Mbps. Custom values use the existing
persisted quality factor (kbps / 12000, at most 5) and survive restart; the
appliance backend retains its original range. Deploy the matching vim4-amlvenc
plugin alongside the application; older plugins accept at most 12000 kbps.
Higher targets require more bandwidth and do not guarantee the actual bitrate.

The UI hides features through `getDeviceCapabilities`. The appliance build keeps
reporting all of its features; new capabilities `cloud`, `display`,
`usb_config`, `time_sync`, `url_media` and `update_source` let a platform hide
or add them.

| Feature | VIM4 behavior |
| --- | --- |
| Video | vfmcapsrc → amlvenc → parser → appsink. H.264 and H.265 (codec preference and browser support decide). Quality factor 1/0.5/0.1 = 12/6/1.2 Mbps. GOP 120 with forced IDR on PLI/FIR and for viewers joining a running stream. The encoder budget uses the HDMI frame rate and reopens when it changes; pipeline errors retry every second |
| Audio | HDMI receiver `hw:AMLAUGESOUND,0` (48 kHz stereo) through JetKVM's G.722/PCMU encoder. The board HDMI service routes TDM-B to hdmirx |
| Keyboard, mouse, macros, paste, jiggler | Existing HID endpoints; horizontal scrolling is unavailable because the board's descriptors have no AC Pan |
| Virtual media | Upload (HTTP and data channel), image list, CD-ROM and Disk mounting, eject. Mounting from a URL is hidden (`url_media`): the kernel has no NBD driver |
| EDID | Defaults to the vendor VIM4 3840×2160 60 Hz table on first use and UI reset. The VIM4 1080p table and JetKVM presets remain selectable. Saved selections and custom EDIDs are preserved. The receiver cannot report its table; the configured one is returned |
| Hide host display when idle | Hot-plug low through the hdmirx debug interface; restored when a viewer connects and when the application stops |
| Network | eth0 IPv4 DHCP/static and IPv6 modes through NetworkManager, DHCP lease details, renew, hostname, mDNS, HTTP proxy (application setting). A new static address redirects the browser. Domain, LLDP and JetKVM's time sources are not offered; chrony owns the clock |
| Wi-Fi | NetworkManager profiles; Ethernet stays the managed interface, so static IPv4 settings never belong to a Wi-Fi network |
| Access | Local password, HTTPS (self-signed or custom), loopback-only. JetKVM Cloud is not offered |
| Advanced | Developer mode, web terminal (khadas shell), SSH key in `~khadas/.ssh/authorized_keys2`, USB emulation, log level, diagnostics (journal), factory reset |
| Updates | The administrator sets an update server; `GET {server}/releases` returns JetKVM's metadata format (`appVersion`, `appUrl`, `appHash`; no system component). Packages are installed without a JetKVM signature after their hash matches, then the board reboots. Version order includes the build number |
| Tailscale | Status, login URL and control server when Tailscale is installed |
| MQTT, Wake-on-LAN, Prometheus | Unchanged application features |
| Factory reset | Removes the JetKVM configuration, images, TLS store and the managed SSH key file, then reboots. Ubuntu network, hostname and accounts are kept |
| Not available | Built-in display, extension port (ATX, DC power, serial console), USB identity and device selection, CDC-ACM console, JetKVM Cloud, HDMI sleep. Their RPCs are rejected |

## Validation

On 2026-10-13, ARM64 builder: `go test -tags vim4 ./...`, `go vet` for both
targets, helper unit tests, UI type check and device build, package verify.
On the board (dev build, then package 1.0.3+vim4.20261013.33 through the web
updater), with a Mac as HDMI source and USB host:

- H.264 and H.265 at 1920×1080 and 3840×2160, about 60 decoded fps over 10 s,
  no drops. Quality factor reopened the encoder at 6000 and 12000 kbps, 60 fps.
- HDMI audio: 50 G.722 packets/s; energy 0.079 with a 440 Hz tone, about 1e-7 without.
- VIM4 4K60 preset switched the Mac to 3840×2160; idle mode removed the display
  without a viewer and restored it at 4K with one.
- Uploaded ISO mounted as CD-ROM and Disk; the Mac read a file from it; eject
  restored NCM. Absolute and relative pointer worked after re-enumeration.
  USB emulation off removed the device; on restored it and input.
- Network settings round trip with mDNS IPv4-only and back, Avahi resolution,
  Wi-Fi scan, save/connect failure/remove with a test network.
- SSH key file, self-signed HTTPS on 443, terminal, diagnostics, local password,
  Tailscale login URL and control server, jiggler.
- Web update downloaded, verified, installed and rebooted; after boot the
  service, gadget and 4K video, audio and pointer worked. Factory reset rebooted
  and kept `~khadas/.ssh/authorized_keys`.

Not tested: real Wi-Fi association, changing the active Ethernet address,
a real tailnet or MQTT broker, Wake-on-LAN on a sleeping host, long-running or
complex moving video, HDMI hotplug, browser handover and cold power-on.

For isolated capture testing, first stop the web application to avoid opening
the encoder twice, then run as khadas:

```sh
go build -tags vim4 -o /tmp/vim4-capture ./cmd/vim4-capture
/tmp/vim4-capture -output /tmp/vim4-capture.h264
```

## Capture resolution

The board HDMI service sets the capture size (`/etc/default/vim4-kvm-hdmi`,
3840×2160, 1920×1080 or 1280×720) and the EDID decides what the source sends.
The vendor specifications list 4K60 HDMI input but 4K50 encoding; 12 Mbps can
limit image quality during complex motion at 4K.

## Switching applications

The last loaded EDID remains when vim4-manager switches to NanoKVM. NanoKVM
restores only its own custom selection; `vim4-edid preset VIM4-4K60` reloads the
vendor default.
