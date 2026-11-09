# Testing

## Automated

CI (`.github/workflows/ci.yml`) runs on every push and pull request:

| Job | Runs |
|---|---|
| test (macos-latest, windows-latest) | `go vet`, `go test -race ./...`, build both binaries, install-script syntax check (bash / PowerShell parser) |
| lint (ubuntu) | gofmt, `go mod tidy` drift, shellcheck on the macOS script, Windows amd64/arm64 cross-builds with CGo off (as the install script builds) |

What the tests cover:

| Area | Tests |
|---|---|
| Core (`internal/app`) | selection, reconnection, supersession, enable/disable, rebinding (success / duplicate / unavailable / cancel / while disabled), the halfway level (offered and registered only for cameras that have one, overrides, levels while disconnected) |
| Shared shell logic (`internal/desktop/rebind.go`) | the rebind flow against a real core (success registers before persisting, Esc, unknown key, duplicate, key held elsewhere keeps the old binding, timeout, stale timeout, prompt failure, abort), hot key dispatch during a rebind, hot key table (replace only after success, ID wrap) |
| MIDI | decoder (`internal/midi`), MIDI bindings in `hotkey` and settings, core (key freed, press, gating), shared router against a real core (pad captured, knob and release ignored, press zooms without a UI hop, duplicate refused), menu line, probe summary |
| Menu model | every camera status line, per-camera zoom levels, hotkey problem, unavailable startup, tooltip |
| Camera models (`cmd/punchline`) | each real model's discovered levels and raw values (Insta360 halfway 2x, PowerConf none), and that every model is wired into the app |
| Settings | defaults, round trip, invalid fields, halfway key moved off a clash, bad per-camera entries, failed save leaves no temp files, clone independence |
| macOS | key table coverage, LaunchAgent write/remove (against a temp `$HOME`), script plist = app plist, temporary-build refusal, single-instance lock and signalling, icon pixel conversion, USB path parsing and IOKit error text, opening a missing device |
| Windows | key table coverage (runs in CI) |
| Boundaries | `internal/repocheck`: OS code only in `desktop`/`uvc`, no network imports, core never imports adapters |

Not unit tested: the native glue that only runs inside the app (Cocoa / Win32 calls, the keyboard hook, IOKit / DirectShow I/O). That is covered by the hardware test, the probe and the acceptance checklist below.

Coverage (`go test -coverpkg=./internal/... ./...`): 71% of internal code; the core 93%, the shared rebind and hotkey code 100%.

### Hardware test (opt-in)

```bash
PUNCHLINE_HW=1 go test -v -count=1 ./internal/camera/hwtest
```

Opens every supported camera connected, sets each level it offers, reads it back, checks each command takes under 100 ms, and leaves it at 1x. It skips without the variable, so CI never needs a camera. Last run (macOS, Link 2 Pro, with the installed app holding the camera at the same time): all levels exact, 0.2–0.3 ms per command.

## Hardware probe

Same commands on Windows and macOS.

```powershell
go run ./cmd/uvcprobe                       # ranges, current value, open time
go run ./cmd/uvcprobe -device anker -sweep    # watch the preview: note where the image visibly changes
go run ./cmd/uvcprobe -device anker -cycle 6  # per-command latency
```

Run the probe again while Teams or Discord shows the camera to confirm control
while the camera is streaming.

## Acceptance checklist (spec section 9)

| # | Check | Windows | macOS |
|---|---|---|---|
| 1 | Install with the platform script | | pass (install twice, uninstall twice, reinstall) |
| 2 | Tray icon, no window | | pass |
| 3 | Starts at login | | pending (log out and back in) |
| 4 | Detects and selects a supported camera (PowerConf C200 / Insta360 Link 2 Pro) | | pass (Link 2 Pro, auto-selected) |
| 5 | F14 = close-up | | pass (Link 2 Pro) |
| 6 | F13 = normal | | pass (Link 2 Pro) |
| 7 | Works while Teams / Discord has focus and is streaming | | pending |
| 8 | Rebind, restart, binding persists | | rebind pass; restart persistence pending |
| 9 | Duplicate binding refused; Esc cancels; timeout cancels | | pass |
| 10 | Disable / re-enable hotkeys from the tray | | pass |
| 11 | Unplug / replug: no crash, reconnects, status updates | | pass |
| 12 | Adapters exchangeable (Insta360 added without core changes) | pass (code) | pass (code: `internal/camera/insta360` + one line in `cmd/punchline/adapters.go`) |
| 13 | No Anker / Insta360 software installed or running | | pass |
| 14 | Quit ends the process | | pass (menu Quit and `-quit`, also with the menu open) |
| 15 | Start at Login off (tray or script) is respected | | plist removal verified; login check pending |
| 16 | Desktop isolated from camera / app logic (`internal/repocheck`) | auto | auto |
| 17 | Halfway (not in spec): Insta360 menu shows 3 levels and F15 = 2×; PowerConf menu shows 2 and F15 reaches other apps | | |
| 18 | MIDI (not in spec): Rebind a level to a RØDECaster pad, the pad zooms, the old key reaches other apps, a knob turned during Rebind is ignored | n/a (planned) | pass (pad binding and zoom, incl. rapid alternating 1×/4× presses) |
