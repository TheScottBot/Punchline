# Disciplinary Close-Up (punchline)

Keyboard-triggered webcam zoom from the system tray, for delivering an
unannounced close-up during Teams or Discord calls. Press **F14** for the
close-up, **F13** to return to normal framing. No companion software, no
window, no network.

Status: Windows implemented; macOS to follow (see [docs/PLAN.md](docs/PLAN.md)).

## Supported cameras

| Camera | USB ID | Integration | Zoom |
|---|---|---|---|
| Anker PowerConf C200 | 291a:3369 | standard UVC | 1×–4× (raw 100–400; magnification to be confirmed) |
| Logitech StreamCam | 046d:0893 | standard UVC | 1×–2× (not yet tested on hardware) |
| Insta360 Link 2 Pro | — | planned (phase 4) | 1×–4× expected |

## Install (Windows)

Needs Go 1.27+ to build from source. No administrator rights.

```powershell
powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1                         # build, install, start at login, launch
powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1 -Action DisableStartup
powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1 -Action EnableStartup
powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1 -Action Status
powershell -ExecutionPolicy Bypass -File scripts\windows\punchline.ps1 -Action Uninstall       # add -KeepSettings to keep settings
```

`-FromExe path\to\punchline.exe` installs a prebuilt binary; `-Arch arm64`
cross-builds. The program goes to `%LOCALAPPDATA%\Programs\Punchline`; start at
login is the per-user `Run` registry value. The script header lists every
change made to the system, and `Uninstall` reverses all of them.

## Using it

Click (or right-click) the magnifier in the notification area. It is neon pink
on a dark tile (Thyme's palette) when a press would zoom, muted otherwise.
Windows 11 puts new tray icons in the overflow (^) area at first; drag it onto
the taskbar to keep it visible.

- **Camera** line: the selected camera and its connection state. With more than
  one supported camera, **Select Camera** chooses; the choice is remembered and
  the app never switches to another camera on its own.
- **Hotkeys Enabled**: turns both hotkeys off (the keys then reach other apps
  as normal).
- **Rebind Normal Zoom / Close-Up**: press the new key within 15 seconds. `Esc`
  cancels. The key is captured system-wide and not passed to the focused app.
  A key already used by the other action, or held by another application, is
  refused and the old binding stays.
- **Start at Login**: the same setting the script manages.
- **Open Settings Folder**, **Quit**.

Any single key without modifiers can be bound (F1–F24, letters, digits, numpad,
navigation, punctuation, media keys). Binding an ordinary key takes it away from
every other application while hotkeys are enabled; F13–F24 avoid that.

## Settings

`%LOCALAPPDATA%\Punchline\settings.json`, written when something changes:

```json
{
  "version": 1,
  "hotkeysEnabled": true,
  "normalKey": "F13",
  "closeUpKey": "F14",
  "preferredCamera": "powerconf:291a:3369:ACNV9P1F20606968",
  "normalZoom": 1,
  "closeUpZoom": 0,
  "cameraZoom": { "powerconf:291a:3369:ACNV9P1F20606968": { "closeUp": 2 } }
}
```

Zoom values are magnifications (1–16). `closeUpZoom: 0` means the camera's
maximum. `cameraZoom` overrides per camera ID (0 or omitted = use the global
value). Values beyond a camera's range are clamped. Invalid fields fall back to
defaults and are logged; an unreadable file is renamed `settings.json.invalid`.
Edits take effect at next start.

Log: `%LOCALAPPDATA%\Punchline\punchline.log` (1 MiB, one previous file kept).
Each zoom logs its command time.

## Development

```powershell
go test ./...                       # includes the import-boundary check (internal/repocheck)
go run ./cmd/uvcprobe               # list cameras and their raw UVC zoom ranges
go run ./cmd/uvcprobe -sweep                # find a camera's visibly distinct zoom levels
go run ./cmd/uvcprobe -cycle 6              # time absolute zoom commands
go run ./cmd/uvcprobe -device anker -sweep  # pick by name (or list number) when several cameras have zoom
```

| Path | Holds |
|---|---|
| `cmd/punchline` | entry point, wiring only |
| `cmd/uvcprobe` | hardware diagnostic |
| `internal/app` | application core: selection, reconnection, zoom dispatch, hotkeys, rebinding |
| `internal/camera` | the camera interface; `mock`, `uvcadapter`, `powerconf`, `streamcam` implement it |
| `internal/uvc` | native UVC zoom access (Windows: DirectShow `IAMCameraControl`, pure Go) |
| `internal/zoom` | magnification ↔ raw value mapping |
| `internal/hotkey` | key names, actions, binding validation |
| `internal/settings` | settings file: defaults, validation, atomic save |
| `internal/desktop` | tray/menu bar, hotkey registration, key capture, startup; the only GUI/OS shell code |
| `internal/icon` | tray icon drawn into a pixel buffer |
| `internal/eventlog` | capped local log |
| `internal/repocheck` | enforces package boundaries and no network imports |
| `scripts/windows` | install / startup / uninstall |
