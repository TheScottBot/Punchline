# Disciplinary Close-Up (punchline)

Keyboard-triggered webcam zoom from the system tray, for delivering an
unannounced close-up during Teams or Discord calls. Press **F14** for the
close-up, **F13** to return to normal framing, and on cameras that offer it
**F15** for a halfway zoom. No companion software, no window, no network.

Status: implemented on Windows and macOS (Apple Silicon). See [docs/PLAN.md](docs/PLAN.md).

## Supported cameras

| Camera | USB ID | Integration | Zoom levels |
|---|---|---|---|
| Anker PowerConf C200 | 291a:3369 | standard UVC | normal 1×, close-up 4× (raw 100–400; magnification to be confirmed) |
| Logitech StreamCam | 046d:0893 | standard UVC | normal 1×, close-up 2× (not yet tested on hardware) |
| Insta360 Link 2 Pro | 2e1a:4c06 | standard UVC (no SDK) | normal 1×, halfway 2×, close-up 4× (raw 100–400, confirmed on hardware) |

Each camera model defines its levels; a halfway level is offered only where the
model has one (`HalfwayMagnification` in its adapter).

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

## Install (macOS)

Needs Go 1.27+ and the Xcode command line tools (`xcode-select --install`) to
build from source. No administrator rights.

```bash
scripts/macos/punchline.sh                     # build, install, start at login, launch
scripts/macos/punchline.sh disable-startup
scripts/macos/punchline.sh enable-startup
scripts/macos/punchline.sh status
scripts/macos/punchline.sh uninstall           # add --keep-settings to keep settings
```

`--from-app path/to/Disciplinary\ Close-Up.app` installs a prebuilt bundle;
`--no-launch` and `--no-startup` skip those steps. The app goes to
`~/Applications/Disciplinary Close-Up.app` (ad-hoc signed, no Dock icon); start
at login is the LaunchAgent `~/Library/LaunchAgents/com.thescottbot.punchline.plist`,
the same file the menu's Start at Login toggle manages. The script header lists
every change made to the system, and `uninstall` reverses all of them.

**Permissions: none.** Hotkeys use Carbon hot keys (no Input Monitoring or
Accessibility), and zoom uses USB control requests (no Camera permission, since
no video is opened). macOS shows a "Background item added" notice when start at
login is first enabled. The item is listed under System Settings > General >
Login Items & Extensions; switching it off there stops it starting at login
even though the menu still shows Start at Login as on.

No native libraries are bundled: the app links only to macOS system frameworks
(Cocoa, Carbon, IOKit), and the Insta360 SDK isn't needed.

## Using it

Click (or right-click) the magnifier in the notification area (Windows) or the
menu bar (macOS). It is neon pink
on a dark tile (Thyme's palette) when a press would zoom, muted otherwise.
Windows 11 puts new tray icons in the overflow (^) area at first; drag it onto
the taskbar to keep it visible.

- **Camera** line: the selected camera and its connection state. With more than
  one supported camera, **Select Camera** chooses; the choice is remembered and
  the app never switches to another camera on its own.
- **Hotkeys Enabled**: turns the hotkeys off (the keys then reach other apps
  as normal).
- **Rebind …**: one entry per zoom level the selected camera offers, with its
  magnification and key, e.g. `Rebind Halfway 2× (F15)…`. A PowerConf shows two
  (Normal Zoom, Close-Up); an Insta360 Link 2 Pro shows three. Press the new key
  within 15 seconds; `Esc` cancels. The key is captured and not passed to the
  focused app. A key already used by another action, or held by another
  application, is refused and the old binding stays. The halfway key is only
  registered while the selected camera has a halfway level, so with other
  cameras it stays free for other apps.
- **Start at Login**: the same setting the script manages.
- **Open Settings Folder**, **Quit**.

**MIDI (macOS):** any zoom level can be bound to a MIDI pad or button instead
of a key. Choose Rebind and press the pad: a CC that goes to 127, or a Note
On, is captured; knobs and faders are ignored. A level bound to MIDI frees its
key for other apps. Tested with RØDECaster Pro II SMART Pads set up as MIDI
triggers; CoreMIDI is shared, so RØDE Central can stay open. The menu shows
the MIDI inputs it hears. In settings the binding reads e.g.
`"closeUpKey": "MIDI CC 1/32"`. Windows MIDI is planned; see
[docs/MIDI.md](docs/MIDI.md).

Any single key without modifiers can be bound (F1–F24, letters, digits, numpad,
navigation, punctuation, media keys). Binding an ordinary key takes it away from
every other application while hotkeys are enabled; F13–F24 avoid that.
On macOS the limit is F20 (the OS has no key codes for F21–F24), and Print
Screen, Scroll Lock, Pause and media/volume keys can't be bound. Rebinding on
macOS shows a small "press a key" panel. The Mac app needs no Input Monitoring,
Accessibility or camera permission.

## Settings

`%LOCALAPPDATA%\Punchline\settings.json` (Windows) or
`~/Library/Application Support/Punchline/settings.json` (macOS), written when
something changes:

```json
{
  "version": 1,
  "hotkeysEnabled": true,
  "normalKey": "F13",
  "halfwayKey": "F15",
  "closeUpKey": "F14",
  "preferredCamera": "insta360:2e1a:4c06",
  "normalZoom": 1,
  "halfwayZoom": 0,
  "closeUpZoom": 0,
  "cameraZoom": { "insta360:2e1a:4c06": { "halfway": 2.5 } }
}
```

Zoom values are magnifications (1–16). `closeUpZoom: 0` means the camera's
maximum; `halfwayZoom: 0` means the camera model's halfway level. Setting
`halfwayZoom` doesn't add a halfway level to cameras that don't have one. An
older file without `halfwayKey` gets F15, or the next free F-key if F15 is
already bound. `cameraZoom` overrides per camera ID (0 or omitted = use the global
value). Values beyond a camera's range are clamped. Invalid fields fall back to
defaults and are logged; an unreadable file is renamed `settings.json.invalid`.
Edits take effect at next start.

Log: `punchline.log` in the same folder (1 MiB, one previous file kept).
Each zoom logs its command time.

## Development

```powershell
go test ./...                       # includes the import-boundary check (internal/repocheck)
PUNCHLINE_HW=1 go test -v ./internal/camera/hwtest   # opt-in: exercise the connected camera(s)
go run ./cmd/midiprobe              # macOS: list MIDI sources, print messages, summary on Ctrl+C
go run ./cmd/uvcprobe               # list cameras and their raw UVC zoom ranges
go run ./cmd/uvcprobe -sweep                # find a camera's visibly distinct zoom levels
go run ./cmd/uvcprobe -cycle 6              # time absolute zoom commands
go run ./cmd/uvcprobe -device anker -sweep  # pick by name (or list number) when several cameras have zoom
```

| Path | Holds |
|---|---|
| `cmd/punchline` | entry point, wiring only |
| `cmd/uvcprobe` | hardware diagnostic |
| `cmd/midiprobe` | MIDI diagnostic (macOS for now) |
| `cmd/mkicon` | writes the macOS app icon set from `internal/icon` (used by the install script) |
| `internal/midi` | MIDI input (macOS CoreMIDI via CGo) and message decoding; groundwork for MIDI triggers, see [docs/MIDI.md](docs/MIDI.md) |
| `internal/app` | application core: selection, reconnection, zoom dispatch, hotkeys, rebinding |
| `internal/camera` | the camera interface; `mock`, `uvcadapter`, `powerconf`, `streamcam`, `insta360` implement it |
| `internal/uvc` | native UVC zoom access (Windows: DirectShow `IAMCameraControl`, pure Go; macOS: IOKit USB class requests via CGo) |
| `internal/zoom` | magnification ↔ raw value mapping |
| `internal/hotkey` | key names, actions, binding validation |
| `internal/settings` | settings file: defaults, validation, atomic save |
| `internal/desktop` | tray/menu bar, hotkey registration, key capture, startup; the only GUI/OS shell code (Windows: Win32 in pure Go; macOS: Cocoa + Carbon hot keys via CGo) |
| `internal/icon` | tray icon drawn into a pixel buffer |
| `internal/eventlog` | capped local log |
| `internal/repocheck` | enforces package boundaries and no network imports |
| `scripts/windows`, `scripts/macos` | install / startup / uninstall |
