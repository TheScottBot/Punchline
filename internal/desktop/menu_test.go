package desktop

import (
	"strings"
	"testing"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/hotkey"
)

func find(items []Item, k CommandKind) []Item {
	var out []Item
	for _, it := range items {
		if it.Cmd.Kind == k && it.Kind == KindCommand {
			out = append(out, it)
		}
		out = append(out, find(it.Children, k)...)
	}
	return out
}

func base() app.Snapshot {
	info := camera.DeviceInfo{ID: "a", Name: "Logitech StreamCam"}
	return app.Snapshot{
		Cameras:  []app.CameraEntry{{Info: info, Present: true, Selected: true}},
		Selected: info, HasSelection: true,
		Status: app.StatusConnected, Bindings: hotkey.DefaultBindings(), Enabled: true,
	}
}

func TestMenuRequiredItems(t *testing.T) {
	items := BuildMenu(base(), StartupState{Enabled: true, Available: true}, PermissionState{}, MIDIState{})
	if items[1].Text != "Camera: Logitech StreamCam (connected)" {
		t.Errorf("status %q", items[1].Text)
	}
	if h := find(items, CmdToggleHotkeys); len(h) != 1 || !h[0].Checked {
		t.Error("hotkeys toggle")
	}
	rb := find(items, CmdRebind)
	if len(rb) != 2 || !strings.Contains(rb[0].Text, "F13") || !strings.Contains(rb[1].Text, "F14") {
		t.Errorf("rebind items %+v", rb)
	}
	if len(find(items, CmdQuit)) != 1 || len(find(items, CmdToggleStartup)) != 1 {
		t.Error("quit / startup missing")
	}
	if len(find(items, CmdSelectCamera)) != 0 {
		t.Error("chooser shown for a single selected camera")
	}
}

func TestMenuShowsTheCamerasZoomLevels(t *testing.T) {
	sn := base()
	sn.Zoom = []app.ZoomLevel{{Action: hotkey.Normal, Magnification: 1}, {Action: hotkey.CloseUp, Magnification: 4}}
	rb := find(BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{}), CmdRebind)
	if len(rb) != 2 || rb[0].Text != "Rebind Normal Zoom 1× (F13)…" || rb[1].Text != "Rebind Close-Up 4× (F14)…" {
		t.Fatalf("two-level camera: %+v", rb)
	}

	sn.Zoom = []app.ZoomLevel{
		{Action: hotkey.Normal, Magnification: 1},
		{Action: hotkey.Halfway, Magnification: 2},
		{Action: hotkey.CloseUp, Magnification: 4},
	}
	rb = find(BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{}), CmdRebind)
	if len(rb) != 3 || rb[1].Text != "Rebind Halfway 2× (F15)…" || rb[1].Cmd.Action != hotkey.Halfway {
		t.Fatalf("three-level camera: %+v", rb)
	}
	if tt := Tooltip(sn); !strings.Contains(tt, "F13 normal · F15 halfway · F14 close-up") {
		t.Error(tt)
	}
}

func TestMenuChooserWithTwoCameras(t *testing.T) {
	sn := base()
	sn.Cameras = append(sn.Cameras, app.CameraEntry{Info: camera.DeviceInfo{ID: "b", Name: "Insta360 Link 2 Pro"}, Present: true})
	cams := find(BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{}), CmdSelectCamera)
	if len(cams) != 2 || !cams[0].Checked || cams[1].Checked {
		t.Errorf("%+v", cams)
	}
}

func TestMenuDisconnectedSelection(t *testing.T) {
	sn := base()
	sn.Status = app.StatusWaiting
	sn.Cameras = []app.CameraEntry{
		{Info: camera.DeviceInfo{ID: "b", Name: "Other"}, Present: true},
		{Info: sn.Selected, Selected: true},
	}
	items := BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{})
	if !strings.Contains(items[1].Text, "disconnected") {
		t.Error(items[1].Text)
	}
	cams := find(items, CmdSelectCamera)
	if len(cams) != 2 || !cams[1].Disabled || cams[0].Disabled {
		t.Errorf("%+v", cams)
	}
}

func TestMenuWhileRebinding(t *testing.T) {
	sn := base()
	sn.Rebinding, sn.RebindAction = true, hotkey.CloseUp
	rb := find(BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{}), CmdRebind)
	if len(rb) != 1 || !rb[0].Disabled {
		t.Errorf("%+v", rb)
	}
}

func TestPermissionItem(t *testing.T) {
	items := BuildMenu(base(), StartupState{}, PermissionState{Missing: true, Text: "Grant Input Monitoring…"}, MIDIState{})
	if p := find(items, CmdOpenPermissions); len(p) != 1 {
		t.Fatal("permission item missing")
	}
}

func TestTooltipAndArmed(t *testing.T) {
	sn := base()
	if tt := Tooltip(sn); !strings.Contains(tt, "F13 normal") || len([]rune(tt)) > 127 {
		t.Error(tt)
	}
	if !Armed(sn) {
		t.Error("should be armed")
	}
	sn.Enabled = false
	if Armed(sn) || !strings.Contains(Tooltip(sn), "disabled") {
		t.Error("disabled state")
	}
}

func TestCameraLineForEveryStatus(t *testing.T) {
	sn := base()
	cases := []struct {
		status app.Status
		name   string
		detail string
		want   string
	}{
		{app.StatusConnected, "Cam", "", "Camera: Cam (connected)"},
		{app.StatusConnecting, "Cam", "", "Camera: Cam (connecting…)"},
		{app.StatusWaiting, "Cam", "", "Camera: Cam (disconnected)"},
		{app.StatusError, "Cam", "boom", "Camera: Cam (error: boom)"},
		{app.StatusError, "", "no backend", "Camera error: no backend"},
		{app.StatusNoCamera, "", "", "Camera: no supported camera connected"},
		{app.StatusSelectCamera, "", "", "Camera: choose one below"},
		{app.StatusSearching, "", "", "Camera: searching…"},
	}
	for _, c := range cases {
		sn.Status, sn.Selected.Name, sn.Detail = c.status, c.name, c.detail
		if got := CameraLine(sn); got != c.want {
			t.Errorf("%v: got %q want %q", c.status, got, c.want)
		}
	}
	sn.Status, sn.Selected.Name, sn.Detail = app.StatusError, "Cam", strings.Repeat("x", 200)
	if got := []rune(CameraLine(sn)); len(got) > 80 || got[len(got)-2] != '…' {
		t.Errorf("long error not truncated: %q", string(got))
	}
}

func TestMenuHotkeyProblemAndUnavailableStartup(t *testing.T) {
	sn := base()
	sn.HotkeyProblem = "F14 is held by another application"
	items := BuildMenu(sn, StartupState{Available: false, Note: "unavailable from a temporary build"}, PermissionState{}, MIDIState{})
	found := false
	for _, it := range items {
		found = found || strings.HasPrefix(it.Text, "Hotkey problem: F14")
	}
	if !found {
		t.Error("hotkey problem not shown")
	}
	st := find(items, CmdToggleStartup)
	if len(st) != 1 || !st[0].Disabled || !strings.Contains(st[0].Text, "temporary build") {
		t.Errorf("%+v", st)
	}
	if Armed(sn) {
		t.Error("armed despite a hotkey problem")
	}
}

func TestMenuBeforeCoreReportsLevels(t *testing.T) {
	sn := base()
	sn.Zoom = nil // e.g. a snapshot taken before the first scan
	rb := find(BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{}), CmdRebind)
	if len(rb) != 2 || rb[0].Text != "Rebind Normal Zoom (F13)…" {
		t.Fatalf("%+v", rb)
	}
	if magText(1.5) != " 1.5×" || magText(0) != "" {
		t.Error("magText")
	}
}
