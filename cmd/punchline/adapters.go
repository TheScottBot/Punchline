package main

import (
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/insta360"
	"github.com/thescottbot/punchline/internal/camera/powerconf"
	"github.com/thescottbot/punchline/internal/camera/streamcam"
	"github.com/thescottbot/punchline/internal/uvc"
)

// adapters lists the camera adapters compiled into the app.
func adapters() []camera.Adapter {
	b := uvc.System()
	return []camera.Adapter{powerconf.New(b), streamcam.New(b), insta360.New(b)}
}
