# JetKVM on VIM4

This page covers Ubuntu integration and operational limits; preserve upstream UI, licenses and authorship.
Read [build](#build-and-install), [operations](#privileged-operations), [features](#feature-boundaries),
[media](#shared-virtual-media), or [validation](#validation). Source links own parameters and defaults.

## Build and install

Use the public workspace [tools](https://github.com/vim4-kvm/vim4-kvm/blob/main/docs/tools.md)
and [package definitions](https://github.com/vim4-kvm/vim4-kvm/blob/main/packaging/ubuntu/package.py).
Install matching board, capture, encoder and GPU components first; application updates cannot supply platform dependencies.
Backend factories, ALSA routing and private runtime/plugin precedence belong to
[vim4-kvm-run](https://github.com/vim4-kvm/vim4-kvm/blob/main/board/vim4-kvm-run).
Both kernels retain application data and the shared image catalog.

The package does not select JetKVM automatically. Select it in the manager, which waits for the old application to stop.
Applications share capture, encoder, HID and the backend listener; systemd Conflicts alone does not ensure stop completion.
Use the same service account and temporary lock namespace for isolated encoder tests.
Never delete an active encoder lock or give the service a private temporary directory.

Installed identity and permissions come from [service](deploy/vim4/vim4-kvm.service);
state paths and listener values from [platform](internal/platform/vim4.go).
Gateway migration preserves other configuration and password hashes; startup also enforces the contract for old/missing files.
Public login and TLS belong to the manager. The appliance build retains its own behavior.

## Privileged operations

The fixed [helper](deploy/vim4/vim4-jetkvm) validates every request, with JSON on stdin so secrets stay out of process arguments.
Authorization is restricted by [sudoers](deploy/vim4/vim4-jetkvm.sudoers).
It owns media, receiver EDID/hotplug, NetworkManager changes, hostname/mDNS, reboot and application package installation.
A failed network reapply restores the previous profile. Wi-Fi settings must not become Ethernet static settings.

Update packages are hash-checked but have no JetKVM signature; use an administrator-controlled update source.
The helper simulates dependencies before install and verifies installed files.
Metadata fields and version order belong to [OTA implementation](internal/ota), not a second documentation schema.
Factory reset removes application state and its managed SSH key, then reboots;
Ubuntu network, hostname, accounts and the account's own SSH key remain.

## Feature boundaries

Capabilities come from [vim4Capabilities](main_vim4.go); rejected RPCs must stay rejected even if UI controls are hidden.
Cloud, device display/extensions, USB identity/device selection, serial console and HDMI sleep are unavailable.
Ubuntu owns clock, network and SSH. Tailscale belongs to the manager and retains existing package/login state.
MQTT, Wake-on-LAN and Prometheus remain application features.

Video behavior belongs to [backend_vim4.go](internal/native/backend_vim4.go),
[encoder](internal/native/vim4_encoder.go) and [HDR](video_hdr_vim4.go).
Output height preserves aspect ratio without upscaling. SDR and HDR use different shared scaling paths;
the newer height UI has not been qualified on the mainline kernel.
Bitrate edits apply on Save, persist across restart and restore the previous value on failure.
Higher targets need bandwidth and do not guarantee actual rate; older encoder plugins cannot accept the expanded range.

The shared HDR source probes capability without using a release name. Codec changes release the old capture before rebuilding.
H.264 stays SDR; native HEVC HDR requires explicit decoder negotiation, otherwise use H.264 SDR.
Safari remote HDR is unqualified. Offline capability checks do not qualify the application/GPU stack on hardware.
Vendor base frame rate stabilizes encoder budgets during VRR but does not qualify actual VRR.

Input monitoring rebuilds capture for stable source changes, and a missing-buffer watchdog recovers silent stalls.
Static pixels do not trigger restart. Reserved AVC workspace and bounded capture queues leave CMA for source changes;
HDMI resynchronization still interrupts frames. Joining viewers and feedback request keyframes;
numeric codec settings belong to source.

Horizontal scrolling is unavailable because board HID descriptors lack AC Pan.
URL media is unavailable because the kernel lacks NBD.
Idle-display hiding lowers hotplug and restores it when a viewer connects or the application stops.
EDID presets/custom selections persist, but the receiver cannot report its table: configured state is not readback.
RTK VRR declarations do not prove JetKVM VRR. Switching applications retains the last loaded table;
another application's saved choice may differ. For display recovery use the
[public contract](https://github.com/vim4-kvm/vim4-kvm/blob/main/docs/subsystems/video.md#hdmi-显示器行为).

## Shared virtual media

The catalog groups other applications' images and shared data disks. They cannot be deleted here and mount read-only;
foreign ISO uses CD-ROM mode. JetKVM's own mounts are also read-only.
Only a successful mount by the current process establishes ownership.
After restart, even an image in JetKVM's directory is external: JetKVM refuses replacement or eject.
Safely eject on the target computer, then use the manager.
An observed external eject clears stale ownership and UI state.

The helper checks expected LUN identity under the shared locks before any USB re-enumeration,
and refuses data disks being edited in the manager. Update application and helper together and install the matching
[board media module](https://github.com/vim4-kvm/vim4-kvm/blob/main/board/vim4media.py).
See [usb_mass_storage_vim4.go](usb_mass_storage_vim4.go) and [vim4media](internal/vim4media/media.go).
LUN attributes cannot detect replacement with the exact same path and flags between observations.

## Validation

Use the workspace builder for VIM4/default Go tests and vet, helper tests, UI typecheck/build and package verification.
Helper tests run without root: `python3 -m unittest discover -s deploy/vim4`.
Fake-LUN tests check ownership/refusal, not configfs or host behavior.
For isolated capture tests stop the web application first and use the service account;
options come from [vim4-capture](cmd/vim4-capture/main.go).

Historical board results are in the [integration record](https://github.com/vim4-kvm/vim4-kvm/blob/main/docs/bringup/2026-10-13-jetkvm-full-integration.md);
they do not qualify the current checkout.
Still unverified there: real Wi-Fi association, changing active Ethernet address, real tailnet/MQTT,
Wake-on-LAN against a sleeping target, sustained complex motion, HDMI hotplug, browser handover and cold power-on.
HDR/scaling qualification must name its configuration via [HDR evidence](https://github.com/vim4-kvm/vim4-kvm/blob/main/docs/hdr.md#相关记录).
Capture dimensions and EDID are separate controls; high input capability does not guarantee motion quality or encoder throughput.
