# Testing

## Automated

`go test ./...` covers zoom mapping, settings validation, hotkey model, the
core (selection, reconnection, supersession, enable/disable, rebinding success /
duplicate / unavailable / cancel / while disabled), the menu model, the icon
and the import boundaries. The race detector needs CGo and a C compiler, which
this machine lacks; run `go test -race ./...` where one is available.

## Hardware probe

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
| 1 | Install with the platform script | | |
| 2 | Tray icon, no window | | |
| 3 | Starts at login | | |
| 4 | Detects and selects the PowerConf C200 | | |
| 5 | F14 = close-up | | |
| 6 | F13 = normal | | |
| 7 | Works while Teams / Discord has focus and is streaming | | |
| 8 | Rebind, restart, binding persists | | |
| 9 | Duplicate binding refused; Esc cancels; timeout cancels | | |
| 10 | Disable / re-enable hotkeys from the tray | | |
| 11 | Unplug / replug: no crash, reconnects, status updates | | |
| 12 | Adapters exchangeable (Insta360 added without core changes) | | |
| 13 | No Anker / Insta360 software installed or running | | |
| 14 | Quit ends the process | | |
| 15 | Start at Login off (tray or script) is respected | | |
| 16 | Desktop isolated from camera / app logic (`internal/repocheck`) | auto | auto |
