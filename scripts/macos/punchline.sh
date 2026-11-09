#!/bin/bash
#
# Install, uninstall and manage start-at-login for Disciplinary Close-Up
# (Punchline) on macOS (Apple Silicon).
#
# Per-user only; never needs administrator rights (no sudo). Every action is
# idempotent.
#
# Changes this script (and the app) can make to the system, all reversed by
# "uninstall":
#   1. ~/Applications/Disciplinary Close-Up.app                 (app bundle, ad-hoc signed)
#   2. ~/Library/LaunchAgents/com.thescottbot.punchline.plist   (start at login)
#   3. launchd job gui/<uid>/com.thescottbot.punchline          (loaded by launchd at login)
#   4. ~/Library/Application Support/Punchline/                 (settings.json, punchline.log,
#                                                                punchline.lock; kept with
#                                                                --keep-settings)
# No daemons, kernel extensions, system-wide files or privacy permissions.
#
# Permissions: none. Hotkeys use Carbon hot keys, which need neither Input
# Monitoring nor Accessibility; zoom uses USB control requests, which need no
# Camera permission because no video is opened. macOS shows a "Background
# item added" notice when start at login is first enabled; the item appears
# in System Settings > General > Login Items & Extensions, where it can also
# be switched off.
#
# Usage:
#   scripts/macos/punchline.sh [install]        build, install, start at login, launch
#   scripts/macos/punchline.sh disable-startup
#   scripts/macos/punchline.sh enable-startup
#   scripts/macos/punchline.sh status
#   scripts/macos/punchline.sh uninstall        add --keep-settings to keep settings
#
# Install options:
#   --from-app PATH   install a prebuilt "Disciplinary Close-Up.app" instead of building
#   --no-launch       don't start the app now
#   --no-startup      don't register start at login

set -euo pipefail

NAME="Disciplinary Close-Up"
LABEL="com.thescottbot.punchline"
EXE_NAME="punchline"
APP="$HOME/Applications/$NAME.app"
EXE="$APP/Contents/MacOS/$EXE_NAME"
DATA_DIR="$HOME/Library/Application Support/Punchline"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GUI_DOMAIN="gui/$(id -u)"

action="install"
from_app=""
no_launch=0
no_startup=0
keep_settings=0

say() { printf '  %s\n' "$*"; }
die() { printf 'punchline: %s\n' "$*" >&2; exit 1; }

usage() {
	sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

while [ $# -gt 0 ]; do
	case "$1" in
		install|uninstall|enable-startup|disable-startup|status) action="$1" ;;
		--from-app) [ $# -ge 2 ] || die "--from-app needs a path"; from_app="$2"; shift ;;
		--no-launch) no_launch=1 ;;
		--no-startup) no_startup=1 ;;
		--keep-settings) keep_settings=1 ;;
		-h|--help) usage 0 ;;
		*) printf 'unknown argument: %s\n\n' "$1" >&2; usage 2 ;;
	esac
	shift
done

[ "$(uname -s)" = "Darwin" ] || die "this script is for macOS"
[ "$(id -u)" -ne 0 ] || die "run as your own user, not root"

running_pids() { pgrep -x -U "$(id -u)" "$EXE_NAME" || true; }

# signal_all sends a signal to every running instance.
signal_all() {
	local pid
	for pid in $(running_pids); do
		kill "-$1" "$pid" 2>/dev/null || true
	done
}

one_line() { tr '\n' ' ' | sed 's/ $//'; }

# stop_app asks a running instance to quit (it unregisters hotkeys and
# releases the camera), then forces it after 5 seconds.
stop_app() {
	local quitter="${1:-$EXE}"
	[ -n "$(running_pids)" ] || return 0
	if [ -x "$quitter" ]; then
		"$quitter" -quit >/dev/null 2>&1 || true
	else
		signal_all TERM
	fi
	local i=0
	while [ -n "$(running_pids)" ] && [ $i -lt 25 ]; do
		sleep 0.2
		i=$((i + 1))
	done
	if [ -n "$(running_pids)" ]; then
		say "forcing stop of process $(running_pids | one_line)"
		signal_all KILL
	fi
	say "stopped running instance"
}

xml_escape() { printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'; }

# Must match launchAgentPlist in internal/desktop/startup_darwin.go, so the
# script and the menu's Start at Login toggle manage the same file.
write_plist() {
	mkdir -p "$(dirname "$PLIST")"
	cat > "$PLIST.tmp" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL</string>
	<key>ProgramArguments</key>
	<array>
		<string>$(xml_escape "$EXE")</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
EOF
	mv "$PLIST.tmp" "$PLIST"
}

enable_startup() {
	[ -x "$EXE" ] || die "not installed ($EXE missing); run install first"
	write_plist
	say "start at login enabled -> $EXE"
}

# disable_startup removes the LaunchAgent. It does not boot out a loaded job:
# that would kill a running instance, and without KeepAlive launchd won't
# start it again anyway.
disable_startup() {
	if [ -f "$PLIST" ]; then
		rm -f "$PLIST"
		say "removed $PLIST"
	fi
	say "start at login disabled"
}

build_binary() {
	local out="$1"
	command -v go >/dev/null 2>&1 || die "Go is not on PATH. Install Go, or pass --from-app <path to $NAME.app>."
	xcrun --find clang >/dev/null 2>&1 || die "the Xcode command line tools are needed to build (xcode-select --install)"
	local ver="dev"
	if command -v git >/dev/null 2>&1; then
		ver="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
	fi
	(cd "$REPO_ROOT" && CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
		go build -trimpath -ldflags "-s -w -X main.version=$ver" -o "$out" ./cmd/punchline) \
		|| die "go build failed"
	say "built $ver" >&2
	printf '%s' "$ver"
}

write_info_plist() {
	local contents="$1" ver="$2"
	cat > "$contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>$EXE_NAME</string>
	<key>CFBundleIdentifier</key>
	<string>$LABEL</string>
	<key>CFBundleName</key>
	<string>$NAME</string>
	<key>CFBundleDisplayName</key>
	<string>$NAME</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$(xml_escape "$ver")</string>
	<key>CFBundleVersion</key>
	<string>$(xml_escape "$ver")</string>
	<key>LSUIElement</key>
	<true/>
	<key>LSMinimumSystemVersion</key>
	<string>14.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
EOF
}

STAGING=""
cleanup() { if [ -n "$STAGING" ]; then rm -rf "$STAGING"; fi; }
trap cleanup EXIT

install_app() {
	STAGING="$(mktemp -d "${TMPDIR:-/tmp}/punchline-build.XXXXXX")"
	local new="$STAGING/$NAME.app"
	if [ -n "$from_app" ]; then
		[ -x "$from_app/Contents/MacOS/$EXE_NAME" ] || die "$from_app is not a $NAME app bundle"
		ditto "$from_app" "$new"
	else
		mkdir -p "$new/Contents/MacOS"
		local ver
		ver="$(build_binary "$new/Contents/MacOS/$EXE_NAME")"
		write_info_plist "$new/Contents" "$ver"
		codesign --force --sign - --timestamp=none "$new" >/dev/null 2>&1 || die "codesign failed"
		say "bundled and ad-hoc signed"
	fi

	stop_app "$new/Contents/MacOS/$EXE_NAME"
	mkdir -p "$(dirname "$APP")"
	rm -rf "$APP"
	mv "$new" "$APP"
	say "installed $APP"

	if [ "$no_startup" -eq 1 ]; then disable_startup; else enable_startup; fi
	if [ "$no_launch" -eq 0 ]; then
		open "$APP"
		say "started (look for the magnifier in the menu bar)"
	fi
}

uninstall_app() {
	stop_app
	disable_startup
	if launchctl print "$GUI_DOMAIN/$LABEL" >/dev/null 2>&1; then
		launchctl bootout "$GUI_DOMAIN/$LABEL" 2>/dev/null || true
		say "unloaded launchd job $LABEL"
	fi
	if [ -d "$APP" ]; then
		rm -rf "$APP"
		say "removed $APP"
	fi
	if [ "$keep_settings" -eq 1 ]; then
		say "kept $DATA_DIR"
	elif [ -d "$DATA_DIR" ]; then
		rm -rf "$DATA_DIR"
		say "removed $DATA_DIR"
	fi
}

yesno() { if "$@"; then echo yes; else echo no; fi; }

show_status() {
	local ver=""
	[ -f "$APP/Contents/Info.plist" ] && ver="$(defaults read "$APP/Contents/Info" CFBundleShortVersionString 2>/dev/null || true)"
	say "installed:        $(yesno test -x "$EXE")  ($APP${ver:+, $ver})"
	say "start at login:   $(if [ -f "$PLIST" ]; then echo enabled; else echo disabled; fi)  ($PLIST)"
	say "launchd job:      $(if launchctl print "$GUI_DOMAIN/$LABEL" >/dev/null 2>&1; then echo loaded; else echo not loaded; fi)"
	local pids
	pids="$(running_pids)"
	say "running:          $(if [ -n "$pids" ]; then echo "yes (pid $(printf '%s' "$pids" | one_line))"; else echo no; fi)"
	say "settings and log: $DATA_DIR  (exists: $(yesno test -d "$DATA_DIR"))"
}

echo "Punchline: $action"
case "$action" in
	install) install_app ;;
	uninstall) uninstall_app ;;
	enable-startup) enable_startup ;;
	disable-startup) disable_startup ;;
	status) show_status ;;
esac
