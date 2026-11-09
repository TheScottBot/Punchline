# MIDI triggers: plan

Status: **built on macOS** (phases 0–3); Windows backend (phase 4) to do. Goal: let a MIDI message trigger a zoom level
as an alternative to a key, specifically the SMART Pads on a RØDECaster Pro II.

## What the RØDECaster Pro II sends

From RØDE's user guide ([Using the SMART Pads](https://rode.com/en/user-guides/rodecaster-pro-ii/using-the-smart-pads),
"Setting Up Custom MIDI Triggers") and release notes; **all to be confirmed
with the probe (phase 0)**:

- A pad set up as a **MIDI trigger** sends, by default, a **Control Change on
  channel 1**. Bank 1 pads are CC 0–7, bank 2 CC 8–15, and so on (8 banks give
  CC 0–63), numbered down column 1 then column 2.
- A custom trigger can send **CC or Note**, with chosen number, channel and
  value. It can send On only, Off only, or both (On on press, Off on release).
- On/Off values: probably 127 / 0. That comes from a third-party guide, not RØDE.
- Firmware 1.0.8 moved pad MIDI to start at channel 1 and fixed duplicate
  messages. Older firmware may use a different channel.
- Fixed CCs on channel 1: pad bank select CC 0 (receive), Record button CC 17,
  pad colour CC 37 ([RØDE help](https://help.rode.com/hc/en-us/articles/8565481010063)).
  These could clash with a pad's CC 0 as seen by the host, which the probe needs to check.
- **Confirmed on this Mac:** with the unit connected over USB, CoreMIDI lists
  one source named **"RODECaster Pro II"**. RØDE's help article calls it
  "MIDI Function", which turned out not to be its CoreMIDI name.
- MIDI must be enabled under
  System settings on the unit or in RØDE Central. A pad is probably either a
  MIDI trigger or a sound pad, not both. It's probably available on USB 1 only.
  None of this is confirmed.

### Probed on this unit (2026-11-09, macOS, `cmd/midiprobe`)

Three pads set up as MIDI triggers, each pressed many times (quick taps and
holds of about a second):

| Pad | Press | Release |
|---|---|---|
| 1 | `B0 20 7F`: CC ch 1 #32, value 127 | `B0 20 00`: CC ch 1 #32, value 0 |
| 2 | `B0 21 7F`: CC ch 1 #33, value 127 | `B0 21 00`: value 0 |
| 3 | `B0 24 7F`: CC ch 1 #36, value 127 | `B0 24 00`: value 0 |

- **Momentary:** exactly one press message when the pad goes down and one
  release when it comes up, even on a held pad. No repeats, no velocity,
  always 127 / 0. This confirms On = 127 and Off = 0.
- The numbers fit RØDE's default layout for **bank 5** (CC 32–39, numbered
  down column 1 then column 2: 32, 33 in the first column, 36 at the top of
  the second).
- **Something else on the unit sent `B5 0F 04`, then `B5 0F 08`** (CC ch 6 #15)
  during the test: a continuous control, not a pad. Since its values are
  non-zero, `IsPress` counts them as presses. So **Rebind must not grab the
  first CC it sees**. During capture, only accept a CC with value 127 (or a
  Note On); intermediate values (knobs, faders, encoders) are ignored. At run
  time a binding matches its exact channel and number and fires on the press
  (value > 0) only, so a stray control can't trigger it.
- Fires about 0.1–0.2 s apart when tapped quickly; each press and release
  arrives as its own message, so the "a newer press supersedes an unsent one"
  rule handles fast taps.

Design consequence: don't hard-code RØDE numbers. Capture whatever message
the pad sends, the same way key rebinding captures the next key. That also
works for any other MIDI controller.

## Design

### Binding model

- **Decided: each action is bound to a key *or* a MIDI trigger, whichever
  was set last** (user decision, 2026-11-09). Binding one replaces the other;
  extend later if needed.
- A trigger is `{type: cc|note, channel 1–16, number 0–127}` plus an optional
  source port name. It fires on **CC with value > 0** or **Note On with
  velocity > 0**. Value 0 and Note Off are releases and are ignored, so
  "On and Off" pads work.
- **Port:** matching is by message alone by default, from any MIDI input. The
  source port name is stored and shown, but only used to tell two devices apart
  if both send the same message. This avoids losing bindings when the port is
  renamed (USB port, firmware).
- Settings: the existing `normalKey` / `halfwayKey` / `closeUpKey` fields hold
  either a key name or a MIDI trigger string such as `"midi cc 1 3"` or
  `"midi note 10 36"`. A binding is one thing, so the value is one field.
  Parsing, formatting and duplicate checks go in the pure `hotkey` package,
  which generalises `Key` to a binding.
- Hot keys are registered only for actions bound to keys. A MIDI-bound action
  frees its key.

### Code

| Piece | Where | Notes |
|---|---|---|
| Message parsing (status byte → CC / Note On / Off, running status) | `internal/midi/msg.go`, pure Go | unit tested; no library needed |
| Input backend interface: list sources, deliver messages, report hot-plug | `internal/midi` | OS-facing, added to `repocheck`'s list alongside `desktop` and `uvc` |
| macOS backend: CoreMIDI via CGo | `internal/midi/coremidi_darwin.go` | `MIDIClientCreateWithBlock` (hot-plug via object added/removed), `MIDIInputPortCreateWithProtocol` (MIDI 1.0), `MIDISourceConnect` to every source. Multi-client, so RØDE Central or a DAW doesn't conflict. Callback thread → channel. About 150 lines. |
| Windows backend: winmm via `x/sys/windows`, pure Go | `internal/midi/winmm_windows.go` | `midiInOpen` with `CALLBACK_WINDOW` into the existing tray window's message loop, then `midiInStart`. Re-enumerate on `WM_DEVICECHANGE`. Classic winmm is **single-client**: if a DAW holds the port, `MMSYSERR_ALLOCATED` is shown in the menu as "MIDI port in use". Windows MIDI Services (rolling out on Windows 11 in 2026) makes WinMM multi-client. |
| Core | `app.Core.Trigger(midi.Message)` | Looks up the action and calls the same non-blocking path as `Press`. Disabled and rebinding gate it the same way. |
| Rebinding | shared `rebinder` | The existing "Rebind …" accepts whichever comes first: the next key or the next MIDI press. Esc and the timeout cancel. It's validated and persisted the same way. |
| Menu | `BuildMenu` | Unchanged shape: `Rebind Close-Up 4× (MIDI CC 1 #3)…`. Adds a "MIDI: n inputs" status line, or the error. |

Not using `gitlab.com/gomidi/midi/v2`: its drivers need RtMidi (C++ via
CGo), PortMidi or a helper binary. None of those fits pure Go on Windows, and
the parsing it would replace is small.

Hotkeys are swallowed; MIDI isn't. CoreMIDI is multi-client, so other apps
also see the message. That's harmless for a pad dedicated to a MIDI trigger.

### Latency

CoreMIDI and winmm deliver within about 1 ms. The handoff to `Core.Trigger`
is a channel send, and the camera command takes under 1 ms on the Link 2 Pro.
That should stay well inside "the punchline must never be delayed".

## Testing

- Message parser: table tests (CC, Note On/Off, velocity-0 Note On, running
  status, SysEx and realtime bytes ignored).
- Trigger parsing, formatting and duplicates: unit tests in `hotkey`. Settings
  round trip and invalid values.
- A fake MIDI source drives the core and the rebinder in tests:
  - trigger → zoom
  - disabled → ignored
  - release ignored
  - Bind MIDI success / duplicate / Esc / timeout
- Opt-in hardware test (`PUNCHLINE_HW=1`, like the camera test): asks you to
  press a pad and checks a trigger arrives.
- CI is unchanged. Backends compile on both runners; no device needed.

## Phases

0. **Probe (built):** `go run ./cmd/midiprobe` lists MIDI sources, prints
   every message with PRESS / release, and on Ctrl+C prints a summary per
   control (presses, releases, values seen). Also `-list`, `-source rode`, and
   `-all` to include clock and other realtime messages. It's built on
   `internal/midi`: a pure decoder (tested) and the CoreMIDI backend. Run it
   with the RØDECaster and press pads in each mode to confirm channel,
   numbers, values and what release sends.
1. **Done:** `internal/midi` has the decoder (tested) and the CoreMIDI backend.
2. **Done:** `hotkey.Key` holds a key or a `MIDITrigger` ("MIDI CC 1/32").
   Settings accept either. The core registers OS hot keys only for key-bound
   actions, and `Core.PressBinding` presses a MIDI-bound one. Tests cover
   freeing the key, pressing, and gating while disabled or rebinding.
3. **Done:** shared `midiRouter` (`internal/desktop/midi.go`):
   - presses go to the core straight from CoreMIDI's thread;
   - during a rebind, capturable presses (CC 127 or Note On) go to the
     rebinder on the UI thread;
   - knobs and releases are ignored.
   The rebind prompt says "key or MIDI pad". The menu has a MIDI status line
   and shows the binding, e.g. `Rebind Close-Up 4× (MIDI CC 1/32)…`. The
   macOS shell opens CoreMIDI on the main thread. Tests run the router against
   a real core.
4. Windows winmm backend: CI compile and unit tests, then a hardware check on Windows.
5. Docs: README, TESTING checklist rows, spec deviation note in PLAN.

## Decisions

1. Key **or** MIDI per action, whichever was set: decided.
2. Match by message from any port: proposed default, revisit if needed.
3. MIDI follows "Hotkeys Enabled": proposed default, revisit if needed.
