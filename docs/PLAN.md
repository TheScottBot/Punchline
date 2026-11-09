# Plan and status

Spec: Disciplinary Close-Up v0.5. Development started on Windows (macOS second).

Spec correction: the primary camera is the **Anker PowerConf C200** (291a:3369),
not the Logitech StreamCam. The StreamCam adapter is kept but has not been
tested on hardware. Insta360 Link 2 Pro remains the second camera.

## Decisions

| Topic | Decision |
|---|---|
| Module | `github.com/thescottbot/punchline`; binary `punchline`; display name "Disciplinary Close-Up" |
| Targets | Windows amd64 + arm64; macOS Apple Silicon only (no Intel) |
| Windows camera control | DirectShow `IAMCameraControl` (`CameraControl_Zoom`) through the in-box UVC driver, pure Go COM, no CGo. All COM on one locked OS thread. |
| Device identity | `adapter:vvvv:pppp:SERIAL` from the parent USB device's instance ID; falls back to a port-specific ID when the camera has no serial |
| Zoom mapping | Magnification mapped linearly onto the raw UVC range: 1× = min, model max = max; step-aligned and clamped. Max magnification is known per model (UVC does not report it). |
| Latency | Camera opened once and kept open (open ≈ 100–270 ms; a command ≈ 1–26 ms). One worker per camera with a single-slot mailbox: a newer press supersedes an unsent one. Hotkey handling never waits on the camera. |
| On connect / reconnect | Apply normal framing. Quit leaves the zoom as it is. |
| Selection | Auto-select and persist the only camera; never switch away from a chosen camera that disconnects; reconnect when it returns. |
| Windows hotkeys | `RegisterHotKey`, no modifiers, `MOD_NOREPEAT`. A key held by another app is reported as unavailable. Disabled = unregistered. |
| Rebinding | Temporary `WH_KEYBOARD_LL` hook captures the next key system-wide (no window), swallows it, 15 s timeout, Esc cancels. Register new before releasing old; persist only after registration. |
| Bindable keys | Any single non-modifier key (user decision: hijacking ordinary keys is accepted). |
| Start at login | Windows: HKCU `Run` value; tray toggle and script manage the same value; Task Manager's `StartupApproved` override is honoured and cleared on enable. Refused when running from a temporary build. |
| Single instance | Named mutex; a second launch notifies the first and exits. `punchline -quit` closes a running instance cleanly. |
| Device changes | `WM_DEVICECHANGE` (debounced) plus a 10 s fallback poll. |
| Uninstall | Reverses every system change the app or script makes (listed in the script header). |

## Phases

| Phase | Status |
|---|---|
| 1. UVC proof of concept | Windows done. Validated on the PowerConf C200 (raw 100–400), including control while a second process holds the device. To do: confirm the C200's visible levels and true maximum magnification with `uvcprobe -sweep`. |
| 2. Shared core | Done: interfaces, mapping, mock adapter, settings, hotkey model, core with unit tests |
| 3. Desktop integration | Windows done (tray, menu, hotkeys, rebinding, enable/disable, startup toggle, single instance, logging). macOS pending. |
| 4. Insta360 Link 2 Pro | Pending. Try the generic UVC adapter first (the SDK's zoom calls are standard UVC); SDK via CGo wrapper only if needed. |
| 5. Deployment | Windows script done and tested for idempotency. macOS script pending. |
| 6. Cross-platform validation | Pending; checklist in TESTING.md |

## Still to verify on hardware (Windows)

- PowerConf C200: where the image visibly changes across 100–400, and whether 400 really is 4×; behaviour while Teams/Discord stream.
- Disconnect/reconnect during a call.
- Whether Teams or Discord themselves reset the zoom when they start the stream.

## macOS plan (next)

- **Camera:** IOKit `IOUSBDeviceInterface` control requests to the Video Control interface (SET_CUR / GET_MIN/MAX/RES/CUR on `CT_ZOOM_ABSOLUTE_CONTROL`), as in nf/logicam, through a small C file compiled with CGo. logicam uses the legacy IOUSBLib API because it still works while another app streams; verify on the current macOS release.
- **Menu bar:** `NSStatusItem` + `NSMenu` built from the same `desktop.BuildMenu` model. Objective-C via CGo, main thread only.
- **Hotkeys and capture: Input Monitoring (preferred).** A `CGEventTap` handles both the hotkeys and key capture for rebinding. The menu and a first-run notification report a missing permission, with a menu item that opens the Privacy & Security pane. Also: `CGPreflightListenEventAccess` / `CGRequestListenEventAccess`.
- **Bundle:** `~/Applications/Disciplinary Close-Up.app`, ad-hoc signed, `LSUIElement` (no Dock icon), bundle ID `com.thescottbot.punchline`.
- **Start at login:** LaunchAgent `~/Library/LaunchAgents/com.thescottbot.punchline.plist`.
- **Uninstall:** quit; `launchctl bootout`; remove the plist, the app bundle and `~/Library/Application Support/Punchline`; `tccutil reset ListenEvent com.thescottbot.punchline` (plus `Accessibility` if used) so the permission grants are removed too.

### Open questions for macOS

1. **Swallowing the key.** A listen-only event tap needs only Input Monitoring, but it cannot stop the key from also reaching Teams. That's harmless for F13–F20, but a bound letter would also type. Stopping the key needs an active tap, which macOS gates behind Accessibility instead. Options: Input Monitoring only (keys pass through), Accessibility (keys swallowed), or Carbon `RegisterEventHotKey`, which swallows keys with no permission but can't capture keys for rebinding.
2. **F21–F24:** macOS defines virtual key codes only up to F20. Check whether a macro pad's F21–F24 produce events at all.
3. Ad-hoc signing changes the code identity on every rebuild, so macOS can drop the Input Monitoring grant after each reinstall. A stable self-signed certificate avoids this.
