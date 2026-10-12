# VIM4 integration

The `vim4` build runs the device web UI, H.264 WebRTC video and keyboard/mouse
input on Ubuntu ARM64. Board services own HDMI setup, USB gadget creation and
USB-C PD negotiation. Preserve licenses and existing authorship.

## Build and install

Build the UI with Node 22.21.1 and npm, then copy the source and generated
`static/` directory to the board:

```sh
npm ci --prefix ui --ignore-scripts
npm --prefix ui run build:device
```

On the Ubuntu VIM4:

```sh
sudo apt-get install golang-go build-essential pkg-config \
  libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev libasound2-dev
go test -tags vim4 ./...
go build -tags vim4 -o /tmp/vim4-kvm ./cmd
sudo sh deploy/vim4/install.sh /tmp/vim4-kvm
```

Go selects the toolchain specified in go.mod. The service listens on port
**8080**. Open http://192.168.50.210:8080 and complete local authentication setup.
Configuration lives in `/var/lib/vim4-kvm/config.json`. The test board uses local
passwordless mode; the Ubuntu account and password are unchanged.

Prerequisites: working vfmcapsrc and amlvenc plugins, vim4-kvm-hdmi.service,
vim4-kvm-gadget.service, vim4-kvm-pd.service, and the eight/six/four-byte HID
descriptors installed by the board gadget script.

The application runs as **khadas**, with video/input group access. The udev rule
grants input group access to hidg0–2. Use the same account for encoder tests:
the vendor encoder uses `/tmp/vpu_mutex.map`. On this Ubuntu installation, root
opening the existing khadas-owned file with O_CREAT is denied by sticky
directory protection, and root capture crashes. Do not delete the lock while an
encoder is active or give the service a private /tmp.

## Implementation

| Area | VIM4 behavior |
| --- | --- |
| Video | `internal/native/backend_vim4.go` uses vfmcapsrc → amlvenc → h264parse → appsink, with complete H.264 access units and SPS/PPS repeated at IDR |
| WebRTC | Existing VideoTrack.WriteSample path; actual PTS spacing overrides the encoder's incorrect nominal 30 fps duration |
| Native libraries | vim4 build excludes Rockchip/LVGL linkage and uses an in-process headless backend |
| USB | Opens existing HID endpoints without creating, rebinding or rewriting the system-managed gadget |
| Input | Matches installed mouse descriptors; preserves position/buttons during wheel events; releases held buttons on last-client disconnect |
| System | Ubuntu retains network, clock, watchdog and system update ownership |
| API | RPC allowlist and HTTP/data-channel restrictions exclude system management, terminal, upload and extension operations |
| UI | Basic KVM capability hides virtual media, Wake-on-LAN and settings toolbar entries |

Capture runs while a WebRTC session needs it. Cancellation, EOS and errors
release the pipeline and DMA-bufs. Appsink does not arbitrarily drop encoded
buffers, which would break prediction within a GOP.

## Validation

On 2026-10-12:

- Full `go test -tags vim4 ./...` passed on the ARM64 board.
- UI TypeScript check and device build passed. Validation used Node 26; the
  package declares Node 22 and emitted an engine warning.
- Capture produced 300 decodable 1920×1080 H.264 frames in 5.017 seconds of media
  time. ffprobe reported 60 fps.
- Headless Chrome decoded 180 additional frames over three seconds, with one
  additional dropped frame during that interval. This is not a latency measurement.
- WebRTC keyboard RPC set and released Shift on the Mac: flags 0 → 131072 → 0.
  Absolute pointer reached the display center; relative motion moved it right.
- Browser reload re-established video and reopened the hardware encoder.
- Whole-board reboot started PD, HDMI, gadget and web services automatically.
  UDC returned to configured; browser video, UI Shift input and both mouse
  reports worked again without manual initialization. Post-reboot Chrome decoded
  181 frames over three seconds, with two additional dropped frames.

For isolated capture testing, first stop the web application to avoid opening
the encoder twice, then run as khadas:

```sh
go build -tags vim4 -o /tmp/vim4-capture ./cmd/vim4-capture
/tmp/vim4-capture -output /tmp/vim4-capture.h264
ffprobe -v error -count_frames -show_entries \
  stream=width,height,r_frame_rate,nb_read_frames /tmp/vim4-capture.h264
```

## Capture resolution

The board HDMI service defaults to 3840×2160 NV21 capture. The application uses
H.264 at 12 Mbps, the stock encoder plugin's maximum, with a 30-frame GOP.
Frame pacing follows input timestamps; selecting 4K does not manufacture 60 fps
from a slower HDMI source. The current test source supplies 2160p60.

To reduce bandwidth or decoder load, put the following in
`/etc/default/vim4-kvm-hdmi` on the board:

```sh
HDMI_WIDTH=1920
HDMI_HEIGHT=1080
```

Supported sizes are 3840×2160, 1920×1080 and 1280×720. Stop `vim4-kvm`, restart
`vim4-kvm-hdmi`, then start `vim4-kvm` and refresh the viewer. Remove the override
to restore 4K. Changing the service configuration persists across reboots.
The 1080p fallback and return to 4K were verified through the browser.

The [vendor specifications](https://www.khadas.com/vim4) list 4K60 HDMI input
but 4K50 encoding. On this board, a steady
600-frame 3840×2160 H.264 capture completed in 10.126 seconds. This is measured
behavior, not a guarantee of lossless 4K60 playback for all content or browsers.
12 Mbps can limit image quality during complex motion at 4K.

After a whole-board reboot, Chrome received 601 additional video frames over
10 seconds at 3840×2160 and dropped 30 during playback (about 57 presented fps).
Reload restored 4K video. PD, HDMI, gadget and web services were active, and the
USB controller was configured. This test did not benchmark complex moving content.

## Current limits

H.264 only; service-configured output size and 12 Mbps encoder setting.
Audio, virtual media, cloud access, system settings, EDID updates, horizontal
scrolling and automatic HDMI error recovery are not implemented. Reconnect the
viewer to restart capture after a pipeline error. HDMI hotplug, resolution changes
through WebRTC, browser handover, USB hotplug and cold boot need dedicated
end-to-end testing.
