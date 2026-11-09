package desktop

// Platform-neutral menu and tooltip model. Each platform renders these items
// natively; keeping the content here gives macOS and Windows the same menu.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/product"
)

// ItemKind is a menu item's type.
type ItemKind int

const (
	KindLabel ItemKind = iota
	KindCommand
	KindSeparator
	KindSubmenu
)

// CommandKind identifies what a menu item does.
type CommandKind int

const (
	CmdNone CommandKind = iota
	CmdToggleHotkeys
	CmdRebind
	CmdSelectCamera
	CmdToggleStartup
	CmdOpenFolder
	CmdOpenPermissions
	CmdQuit
)

// Command is the action attached to a menu item.
type Command struct {
	Kind   CommandKind
	Action hotkey.Action
	Camera camera.DeviceID
}

// Item is one menu entry.
type Item struct {
	Kind     ItemKind
	Text     string
	Checked  bool
	Disabled bool
	Cmd      Command
	Children []Item
}

// StartupState describes start-at-login for the menu.
type StartupState struct {
	Enabled   bool
	Available bool
	Note      string // why unavailable
}

// PermissionState describes an OS permission the app needs (macOS Input
// Monitoring). Zero value means nothing to show.
type PermissionState struct {
	Missing bool
	Text    string
}

func sep() Item { return Item{Kind: KindSeparator} }

func label(s string) Item { return Item{Kind: KindLabel, Text: s, Disabled: true} }

// BuildMenu returns the tray / menu bar menu for a snapshot.
func BuildMenu(sn app.Snapshot, st StartupState, perm PermissionState, m MIDIState) []Item {
	items := []Item{label(product.DisplayName), label(CameraLine(sn))}
	if l := midiLine(sn.Bindings, m); l != "" {
		items = append(items, label(l))
	}
	if perm.Missing {
		items = append(items, Item{Kind: KindCommand, Text: perm.Text, Cmd: Command{Kind: CmdOpenPermissions}})
	}
	if sn.HotkeyProblem != "" {
		items = append(items, label("Hotkey problem: "+truncate(sn.HotkeyProblem, 70)))
	}
	if showChooser(sn) {
		var sub []Item
		for _, c := range sn.Cameras {
			t := c.Info.Name
			if !c.Present {
				t += " (disconnected)"
			}
			sub = append(sub, Item{
				Kind: KindCommand, Text: t, Checked: c.Selected, Disabled: !c.Present,
				Cmd: Command{Kind: CmdSelectCamera, Camera: c.Info.ID},
			})
		}
		items = append(items, Item{Kind: KindSubmenu, Text: "Select Camera", Children: sub})
	}
	items = append(items, sep(),
		Item{Kind: KindCommand, Text: "Hotkeys Enabled", Checked: sn.Enabled,
			Disabled: sn.Rebinding, Cmd: Command{Kind: CmdToggleHotkeys}})
	for _, l := range zoomLevels(sn) {
		a := l.Action
		if sn.Rebinding && sn.RebindAction == a {
			items = append(items, label(fmt.Sprintf("%s: press a key now (Esc cancels)", a)))
			continue
		}
		items = append(items, Item{
			Kind: KindCommand, Text: fmt.Sprintf("Rebind %s%s (%s)…", a, magText(l.Magnification), sn.Bindings.Get(a)),
			Disabled: sn.Rebinding, Cmd: Command{Kind: CmdRebind, Action: a},
		})
	}
	startText := "Start at Login"
	if !st.Available && st.Note != "" {
		startText += " (" + st.Note + ")"
	}
	items = append(items, sep(),
		Item{Kind: KindCommand, Text: startText, Checked: st.Enabled, Disabled: !st.Available,
			Cmd: Command{Kind: CmdToggleStartup}},
		Item{Kind: KindCommand, Text: "Open Settings Folder", Cmd: Command{Kind: CmdOpenFolder}},
		sep(),
		Item{Kind: KindCommand, Text: "Quit", Cmd: Command{Kind: CmdQuit}},
	)
	return items
}

// zoomLevels is the selected camera's zoom actions; before the core reports
// any, Normal and Close-Up.
func zoomLevels(sn app.Snapshot) []app.ZoomLevel {
	if len(sn.Zoom) > 0 {
		return sn.Zoom
	}
	return []app.ZoomLevel{{Action: hotkey.Normal}, {Action: hotkey.CloseUp}}
}

// magText formats a magnification for the menu: " 2×", " 1.5×", or ""
// when unknown.
func magText(m float64) string {
	if m <= 0 {
		return ""
	}
	return fmt.Sprintf(" %s×", strconv.FormatFloat(m, 'f', -1, 64))
}

func showChooser(sn app.Snapshot) bool {
	return len(sn.Cameras) > 1 || (len(sn.Cameras) == 1 && !sn.Cameras[0].Selected)
}

// CameraLine describes the camera status in one line.
func CameraLine(sn app.Snapshot) string {
	name := sn.Selected.Name
	switch sn.Status {
	case app.StatusConnected:
		return "Camera: " + name + " (connected)"
	case app.StatusConnecting:
		return "Camera: " + name + " (connecting…)"
	case app.StatusWaiting:
		return "Camera: " + name + " (disconnected)"
	case app.StatusError:
		if name == "" {
			return "Camera error: " + truncate(sn.Detail, 60)
		}
		return "Camera: " + name + " (error: " + truncate(sn.Detail, 50) + ")"
	case app.StatusNoCamera:
		return "Camera: no supported camera connected"
	case app.StatusSelectCamera:
		return "Camera: choose one below"
	}
	return "Camera: searching…"
}

// Tooltip summarises state for the tray icon's hover text.
func Tooltip(sn app.Snapshot) string {
	keys := "Hotkeys disabled"
	if sn.Enabled {
		var parts []string
		for _, l := range zoomLevels(sn) {
			name := map[hotkey.Action]string{hotkey.Normal: "normal", hotkey.Halfway: "halfway", hotkey.CloseUp: "close-up"}[l.Action]
			parts = append(parts, fmt.Sprintf("%s %s", sn.Bindings.Get(l.Action), name))
		}
		keys = strings.Join(parts, " · ")
	}
	return truncate(product.DisplayName+"\n"+strings.TrimPrefix(CameraLine(sn), "Camera: ")+"\n"+keys, 127)
}

// Armed reports whether a press would zoom now (drives the icon style).
func Armed(sn app.Snapshot) bool {
	return sn.Enabled && sn.Status == app.StatusConnected && sn.HotkeyProblem == ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
