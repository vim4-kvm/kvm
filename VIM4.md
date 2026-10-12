# VIM4 integration

Status: initial integration branch; the full application is not yet runnable on
VIM4. The report encoder is tested in isolation and is not yet wired to RPC.
Keep the existing board services running until the integration passes hardware
acceptance. Preserve the license, copyright notices and existing authorship.

## Hardware boundaries

| Area | Existing application | VIM4 integration |
| --- | --- | --- |
| Video | `internal/native/cgo_linux.go` links `jknative` and LVGL; video uses Rockchip APIs | Separate native backend using `vfmcapsrc ! amlvenc ! h264parse ! appsink` |
| WebRTC | `native.go` submits encoded frames through `VideoTrack.WriteSample` | Deliver complete H.264 access units with buffer timing; repeat SPS/PPS at IDR for new viewers |
| Display | Native interface also controls local touchscreen | Headless backend; unsupported controls must not claim success |
| USB | `usb.go` creates and manages its own gadget | Board service owns configfs and PD; application only opens existing HID endpoints |
| Mouse | Absolute report includes an ID; relative report includes horizontal wheel | `internal/vim4hid` matches the installed six/four-byte descriptors without IDs |
| Network | `main.go` calls `initNetwork` | Ubuntu retains ownership of Ethernet and DHCP |
| Updates | Startup initializes OTA and periodic update checks; RPC also exposes updates | Disable firmware/system updates on this platform, including RPC routes |
| Storage | Configuration uses `/userdata/kvm_config.json` | Use a dedicated Ubuntu state directory with explicit ownership |
| Watchdog | Startup opens `/dev/watchdog` | Leave disabled until shutdown and restart behavior is verified |

Do not launch the unmodified main program as root on the board. Selecting ARM64
alone does not isolate hardware initialization or remove the native libraries.

## First usable milestone

1. Add a Linux ARM64 backend with explicit build selection. Exclude Rockchip and
   LVGL linkage. Keep existing web authentication, signaling and browser UI.
2. Read encoded buffers through appsink with access-unit alignment and valid
   timestamps. Handle EOS, signal loss and process shutdown; do not treat pipe
   reads as frame boundaries. Validate codec/profile negotiation in the browser.
3. Connect HID RPC to the installed gadget, including bounded writes, release on
   disconnect, reopen after re-enumeration, and keyboard LED feedback. Reject
   unsupported horizontal scrolling instead of appending extra report bytes.
4. Isolate system management before starting the web service. Disable unsupported
   UI actions as well as backend routes; a default config flag is insufficient.
5. Verify browser video and keyboard/mouse on the Mac, then disconnect/reconnect
   the browser and reboot the board. Capture frame rate and latency as measured
   results, not inherited performance claims.

## Current checks

`go test ./internal/vim4hid` checks wire layout and descriptor range boundaries.
This does not establish browser-to-host input or a working video backend.
