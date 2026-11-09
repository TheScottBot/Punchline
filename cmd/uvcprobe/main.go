// Command uvcprobe lists video devices and exercises their UVC zoom control.
// It is a diagnostic for hardware verification, not part of the app.
//
//	uvcprobe                          list devices and zoom ranges
//	uvcprobe -sweep                   step through the range to find visible levels
//	uvcprobe -cycle 4                 alternate min/max, timing each command
//	uvcprobe -set 200                 set a raw zoom value
//	uvcprobe -device streamcam -sweep choose a device by list number or name
//
// Without -device, the only camera that has a zoom control is used.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/thescottbot/punchline/internal/uvc"
)

func main() {
	dev := flag.String("device", "", "device: list number ([0], [1], ...) or part of its name; default: the only camera with zoom")
	set := flag.Int("set", -1, "raw zoom value to set")
	cycle := flag.Int("cycle", 0, "alternate min/max this many times")
	sweep := flag.Bool("sweep", false, "step through the range one step per second")
	pause := flag.Duration("pause", 2*time.Second, "pause between cycle commands")
	reopen := flag.Bool("reopen", false, "with -cycle: open and close the device for every command")
	flag.Parse()

	b := uvc.System()
	nodes, err := b.Enumerate()
	if err != nil {
		fail(err)
	}
	if *set < 0 && *cycle == 0 && !*sweep {
		list(b, nodes)
		return
	}
	n := pick(b, nodes, *dev)
	t0 := time.Now()
	c, err := b.Open(n.Path)
	if err != nil {
		fail(err)
	}
	fmt.Printf("opened %s in %v\n", n.Name, time.Since(t0))
	r, err := c.ZoomRange()
	if err != nil {
		fail(err)
	}
	fmt.Printf("range %+v\n", r)
	switch {
	case *set >= 0:
		timed(c, int32(*set))
	case *cycle > 0:
		for i := 0; i < *cycle; i++ {
			v := r.Max
			if i%2 == 1 {
				v = r.Min
			}
			if *reopen {
				c.Close()
				t := time.Now()
				if c, err = b.Open(n.Path); err != nil {
					fail(err)
				}
				fmt.Printf("  reopen %v\n", time.Since(t))
			}
			timed(c, v)
			time.Sleep(*pause)
		}
		timed(c, r.Min)
	case *sweep:
		step := r.Step
		if step <= 0 {
			step = 1
		}
		if n := (r.Max - r.Min) / step; n > 40 {
			step = (r.Max - r.Min) / 40
		}
		for v := r.Min; ; v += step {
			if v > r.Max {
				v = r.Max // always finish exactly at the maximum
			}
			timed(c, v)
			time.Sleep(time.Second)
			if v == r.Max {
				break
			}
		}
		timed(c, r.Min)
	}
	c.Close()
}

func list(b uvc.Backend, nodes []uvc.Node) {
	if len(nodes) == 0 {
		fmt.Println("no video capture devices")
	}
	for i, n := range nodes {
		fmt.Printf("[%d] %s\n    usb %s  stable %s\n    path %s\n", i, n.Name, n.USBID(), n.StableID, n.Path)
		t0 := time.Now()
		c, err := b.Open(n.Path)
		if err != nil {
			fmt.Printf("    open: %v\n", err)
			continue
		}
		open := time.Since(t0)
		r, err := c.ZoomRange()
		if err != nil {
			fmt.Printf("    zoom: %v (open %v)\n", err, open)
		} else {
			cur, cerr := c.Zoom()
			fmt.Printf("    zoom min %d max %d step %d default %d current %d (%v)  open %v\n", r.Min, r.Max, r.Step, r.Default, cur, cerr, open)
		}
		c.Close()
	}
}

// pick selects a device by index, by case-insensitive name fragment, or (when
// sel is empty) the only device exposing a zoom control.
func pick(b uvc.Backend, nodes []uvc.Node, sel string) uvc.Node {
	if i, err := strconv.Atoi(sel); err == nil {
		if i < 0 || i >= len(nodes) {
			fail(fmt.Errorf("no device [%d]; run uvcprobe with no flags to list devices", i))
		}
		return nodes[i]
	}
	var matches []uvc.Node
	for _, n := range nodes {
		if sel != "" {
			if strings.Contains(strings.ToLower(n.Name), strings.ToLower(sel)) {
				matches = append(matches, n)
			}
			continue
		}
		if c, err := b.Open(n.Path); err == nil {
			if r, err := c.ZoomRange(); err == nil && r.Max > r.Min {
				matches = append(matches, n)
			}
			c.Close()
		}
	}
	switch len(matches) {
	case 1:
		return matches[0]
	case 0:
		if sel == "" {
			fail(fmt.Errorf("no camera with a zoom control found"))
		}
		fail(fmt.Errorf("no device name contains %q; run uvcprobe with no flags to list devices", sel))
	}
	var names []string
	for _, m := range matches {
		names = append(names, m.Name)
	}
	fail(fmt.Errorf("several devices match (%s); use -device with a list number", strings.Join(names, ", ")))
	return uvc.Node{}
}

func timed(c uvc.Control, v int32) {
	t := time.Now()
	err := c.SetZoom(v)
	d := time.Since(t)
	got, gerr := c.Zoom()
	fmt.Printf("  set %d in %v err=%v  readback %d (%v)\n", v, d, err, got, gerr)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "uvcprobe:", err)
	os.Exit(1)
}
