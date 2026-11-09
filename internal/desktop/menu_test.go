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
	items := BuildMenu(base(), StartupState{Enabled: true, Available: true}, PermissionState{})
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

func TestMenuChooserWithTwoCameras(t *testing.T) {
	sn := base()
	sn.Cameras = append(sn.Cameras, app.CameraEntry{Info: camera.DeviceInfo{ID: "b", Name: "Insta360 Link 2 Pro"}, Present: true})
	cams := find(BuildMenu(sn, StartupState{}, PermissionState{}), CmdSelectCamera)
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
	items := BuildMenu(sn, StartupState{}, PermissionState{})
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
	rb := find(BuildMenu(sn, StartupState{}, PermissionState{}), CmdRebind)
	if len(rb) != 1 || !rb[0].Disabled {
		t.Errorf("%+v", rb)
	}
}

func TestPermissionItem(t *testing.T) {
	items := BuildMenu(base(), StartupState{}, PermissionState{Missing: true, Text: "Grant Input Monitoring…"})
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
