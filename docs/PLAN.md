# Plan and status

Spec: Disciplinary Close-Up v0.5. Development started on Windows; macOS second.

Spec correction: the primary camera is the **Anker PowerConf C200** (291a:3369),
not the Logitech StreamCam. The StreamCam adapter is kept but has not been
tested on hardware. Insta360 Link 2 Pro remains the second camera.

## Decisions

| Topic | Decision |
|---|---|
| Module | `github.com/thescottbot/punchline`; binary `punchline`; display name "Disciplinary Close-Up" |
| Targets | Windows amd64 + arm64; macOS Apple Silicon only (no Intel) |
| Windows camera control | DirectShow `IAMCameraControl` (`CameraControl_Zoom`) through the in-box UVC driver, pure Go COM, no CGo. All COM on one locked OS thread. |
| Device identity | `adapter:vvvv:pppp:SERIAL` (Windows: from the parent USB device's instance ID; macOS: the `USB Serial Number` property). A camera without a serial is `adapter:vvvv:pppp`, so it survives a port change; the port is added only while two identical serial-less cameras are connected. |
| Zoom mapping | Magnification mapped linearly onto the raw UVC range: 1× = min, model max = max; step-aligned and clamped. Max magnification is known per model (UVC does not report it). |
| Latency | Camera opened once and kept open (open ≈ 100–270 ms; a command ≈ 1–26 ms). One worker per camera with a single-slot mailbox: a newer press supersedes an unsent one. Hotkey handling never waits on the camera. |
| On connect / reconnect | Apply normal framing. Quit leaves the zoom as it is. |
| Selection | Auto-select and persist the only camera; never switch away from a chosen camera that disconnects; reconnect when it returns. |
| Windows hotkeys | `RegisterHotKey`, no modifiers, `MOD_NOREPEAT`. A key held by another app is reported as unavailable. Disabled = unregistered. |
| Rebinding | Temporary `WH_KEYBOARD_LL` hook captures the next key system-wide (no window), swallows it, 15 s timeout, Esc cancels. Register new before releasing old; persist only after registration. |
| Bindable keys | Any single non-modifier key (user decision: hijacking ordinary keys is accepted). |
| Zoom levels | Normal and Close-Up on every camera; a third **Halfway** level (default key F15) on models that define one: the Insta360 Link 2 Pro at 2× (user request: 4× is too much for some shots). Not on the PowerConf or StreamCam. The level is part of the model (`uvcadapter.Model.HalfwayMagnification`) and reported at discovery (`DeviceInfo.Zoom`), so the menu shows the right levels even while the camera is disconnected. Settings: `halfwayKey`, `halfwayZoom` (0 = model's level), `cameraZoom[id].halfway`. |
| Shared shell logic | The rebind flow (`rebinder`) and hot key bookkeeping (`hotkeyTable`, `dispatchHotkey`) live in `internal/desktop/rebind.go`, shared by both shells and unit tested. Each shell supplies only its native parts: macOS a capture panel and Carbon hot keys; Windows a low-level keyboard hook and `RegisterHotKey`. |
| CI | GitHub Actions on macOS and Windows runners (race tests, vet, builds, script syntax) plus a lint job; see TESTING.md. |
| Hotkey registration | `Core.SyncHotkeys` registers exactly the keys the selected camera offers. The shells call it on their UI thread from the state-changed handler, because registration is thread-bound on both platforms and the change is detected on the camera monitor goroutine. A key that fails is not retried until bindings, levels or the enabled state change. |
| Start at login | Windows: HKCU `Run` value; tray toggle and script manage the same value; Task Manager's `StartupApproved` override is honoured and cleared on enable. Refused when running from a temporary build. |
| Single instance | Named mutex; a second launch notifies the first and exits. `punchline -quit` closes a running instance cleanly. |
| Device changes | `WM_DEVICECHANGE` (debounced) plus a 10 s fallback poll. |
| Uninstall | Reverses every system change the app or script makes (listed in the script header). |

## Phases

| Phase | Status |
|---|---|
| 1. UVC proof of concept | Windows done. Validated on the PowerConf C200 (raw 100–400), including control while a second process holds the device. To do: confirm the C200's visible levels and true maximum magnification with `uvcprobe -sweep`. macOS backend written (IOKit, `internal/uvc/uvc_darwin.go`); builds and `uvcprobe` runs, but not yet tried with a camera attached. |
| 2. Shared core | Done: interfaces, mapping, mock adapter, settings, hotkey model, core with unit tests |
| 3. Desktop integration | Windows done (tray, menu, hotkeys, rebinding, enable/disable, startup toggle, single instance, logging). macOS done (menu bar, Carbon hotkeys, capture panel, LaunchAgent toggle, single instance, USB notifications); no permissions needed. Hands-on checks pending. |
| 4. Insta360 Link 2 Pro | Adapter added (`internal/camera/insta360`), standard UVC, no SDK or native dependency. Probed on macOS: USB 2e1a:4c06, zoom 100–400 step 1, every value accepted and read back, command < 1 ms. One readback straight after a set returned kIOReturnAborted; reads now retry once. No USB serial number: ID is `insta360:2e1a:4c06` while it is the only one connected (port added only when two are). Sweep checked visually: smooth zoom across the range, 400 = 4×. To do: test while Teams/Discord streams; test on Windows. |
| 5. Deployment | Windows and macOS scripts done and tested for idempotency (install twice, startup toggles twice, uninstall twice, reinstall). macOS: login launch from the LaunchAgent still to be confirmed by logging out and back in. |
| 6. Cross-platform validation | Pending; checklist in TESTING.md |

## Still to verify on hardware (Windows)

- PowerConf C200: where the image visibly changes across 100–400, and whether 400 really is 4×; behaviour while Teams/Discord stream.
- Disconnect/reconnect during a call.
- Whether Teams or Discord themselves reset the zoom when they start the stream.

## macOS

- **Camera:** `internal/uvc/uvc_darwin.go`. Finds `IOUSBHostDevice`s with a Video Control interface from registry properties (no I/O to unrelated devices), reads the configuration descriptor to get the VC interface number, camera terminal ID and whether it advertises absolute zoom (`desc.go`, unit tested), then sends UVC class requests (GET_MIN/MAX/RES/DEF/CUR, SET_CUR on `CT_ZOOM_ABSOLUTE_CONTROL`) on the default pipe with `IOUSBDeviceInterface182::DeviceRequestTO` (1 s timeout). No `USBDeviceOpen`, as in nf/logicam. The legacy IOUSBLib API is still in the macOS 27 SDK, not marked deprecated. CGo, system frameworks only. Verified with the Link 2 Pro. No camera (TCC) permission is involved because no video is opened. To verify: control while Teams/Discord streams; the C200 on macOS.
- **Menu bar (done):** `NSStatusItem` + `NSMenu` rebuilt from `desktop.BuildMenu` each time it opens, so the menu matches Windows. Objective-C in `shell_darwin.m` (ARC), Go in `shell_darwin.go`. Accessory activation policy: no Dock icon even when run unbundled. Notices that Windows shows as balloons appear as a line in the menu until the next menu action, and rebind results appear in the capture panel. Work queued for the main thread uses `CFRunLoopPerformBlock` in the common modes so it still runs while the menu is open.
- **Hotkeys (done): Carbon `RegisterEventHotKey`**, no modifiers, `kEventHotKeyExclusive`. Needs **no permission** (no Input Monitoring, no Accessibility) and consumes the key, like `RegisterHotKey` on Windows. A key held exclusively by another app (`eventHotKeyExistsErr`) is reported as unavailable. Register new before releasing old.
- **Rebinding (done):** a temporary panel ("Press the new key…") is activated and an `NSEvent` local monitor takes the next key down and swallows it, also with no permission. Esc, closing the panel, clicking away, or 15 s cancel. A key that is currently bound is consumed by its hot key first, so the hot key handler feeds it to the capture.
- **Keys:** macOS has no virtual key codes beyond F20, so F21–F24 are refused as "not available on macOS". Print Screen / Scroll Lock / Pause (which arrive as F13–F15) and volume/media keys (system-defined events) are refused too. Every other key in `hotkey.Supported` is mapped (`keys_darwin.go`, test checks coverage).
- **Single instance (done):** `flock` on `~/Library/Application Support/Punchline/punchline.lock`, which holds the PID. A second launch sends SIGUSR1 (the running instance opens its menu); `punchline -quit` sends SIGTERM, which shuts down cleanly (unregister, release camera, remove the menu bar item). Signals instead of distributed notifications, which proved unreliable from a sandboxed shell.
- **Device changes (done):** IOKit first-match / terminated notifications for `IOUSBHostDevice`, debounced 700 ms, plus the 10 s fallback poll.
- **Start at login (done in app):** LaunchAgent `~/Library/LaunchAgents/com.thescottbot.punchline.plist` (`RunAtLoad`, `ProcessType Interactive`, `LimitLoadToSessionType Aqua`). The menu toggle writes / deletes it and never boots out the running job. Refused from temporary builds (`$TMPDIR`, `/tmp`, `go-build`).
- **Logout / restart:** `applicationShouldTerminate:` runs the same shutdown.
- **Bundle (done, `scripts/macos/punchline.sh`):** `~/Applications/Disciplinary Close-Up.app`, built into a temporary folder, ad-hoc signed, then swapped in after the running instance quits. `LSUIElement`, bundle ID `com.thescottbot.punchline`, arm64 only. With no TCC permissions in use, ad-hoc signing's changing code identity doesn't matter. No app icon (`.icns`) yet; the menu bar icon is drawn at run time.
- **Uninstall (done):** `punchline -quit` (forced after 5 s); remove the plist; `launchctl bootout` the job if loaded; remove the bundle and `~/Library/Application Support/Punchline` (kept with `--keep-settings`). No `tccutil` step needed. `disable-startup` only removes the plist and never boots out the job, which would kill the running app. A test checks that the script's plist matches the app's byte for byte.
- **Known gap:** switching the item off in System Settings > Login Items leaves the plist in place, so the menu still shows Start at Login as on. Reading that Background Task Management state needs admin (`sfltool`), so it isn't reflected.

### Still to verify on macOS

- Hotkeys while Teams / Discord has focus and is streaming.
- Whether a VIA/QMK macro pad's F13–F20 register as expected (they should; F21–F24 can't).
- Unplug / replug of the camera while running.
- Start at login: log out and back in after installing.
