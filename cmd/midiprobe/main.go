// Command midiprobe lists MIDI input sources and prints every message they
// send, to find out what a controller (such as RØDECaster Pro II pads) puts
// on the wire before binding it. It is a diagnostic, not part of the app.
//
//	midiprobe                 print messages until Ctrl+C, then a summary per control
//	midiprobe -list           list sources and exit
//	midiprobe -source rode    only sources whose name contains "rode"
//	midiprobe -all            include realtime messages (clock, active sensing)
//
// Listening is shared (CoreMIDI is multi-client): RØDE Central or a DAW can
// stay open.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/thescottbot/punchline/internal/midi"
)

// CoreMIDI reports source changes on the run loop of the thread that opened
// the client; keep main on the main thread and run its loop there.
func init() { runtime.LockOSThread() }

func main() {
	list := flag.Bool("list", false, "list sources and exit")
	all := flag.Bool("all", false, "include realtime messages (clock, active sensing)")
	only := flag.String("source", "", "only sources whose name contains this (case-insensitive)")
	flag.Parse()

	var (
		mu     sync.Mutex
		tl     = newTally()
		events = make(chan midi.Event, 1024)
		start  = time.Now()
	)
	match := func(name string) bool {
		return *only == "" || strings.Contains(strings.ToLower(name), strings.ToLower(*only))
	}
	c, err := midi.Open(
		func(e midi.Event) {
			select {
			case events <- e:
			default: // never block CoreMIDI's thread
			}
		},
		func(ss []midi.Source) {
			fmt.Printf("%s  sources changed:\n", stamp(start))
			printSources(ss)
		})
	if err != nil {
		fmt.Fprintln(os.Stderr, "midiprobe:", err)
		os.Exit(1)
	}
	fmt.Println("MIDI sources:")
	printSources(c.Sources())
	if *list {
		c.Close()
		return
	}
	fmt.Println("listening; press pads now (Ctrl+C for the summary)")

	go func() {
		for e := range events {
			if !match(e.Source.Name) || (!*all && e.Msg.IsRealtime()) {
				continue
			}
			mu.Lock()
			tl.add(e)
			mu.Unlock()
			edge := "release"
			if e.Msg.IsPress() {
				edge = "PRESS"
			}
			if e.Msg.Kind == midi.Other {
				edge = ""
			}
			fmt.Printf("%s  %-32s %-40s %-7s [% X]\n",
				stamp(start), e.Source.Name, e.Msg, edge, e.Msg.Raw[:rawLen(e.Msg)])
		}
	}()
	go func() { // fallback in case a setup-changed notice is missed
		for range time.Tick(2 * time.Second) {
			c.Refresh()
		}
	}()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		time.Sleep(50 * time.Millisecond) // let queued events print
		mu.Lock()
		fmt.Print("\n" + tl.String())
		mu.Unlock()
		c.Close()
		os.Exit(0)
	}()
	midi.RunLoop()
}

func printSources(ss []midi.Source) {
	if len(ss) == 0 {
		fmt.Println("  (none) — is the device connected, and is MIDI enabled on it?")
	}
	for _, s := range ss {
		fmt.Printf("  %-40s id %d\n", s.Name, s.ID)
	}
}

func stamp(start time.Time) string {
	return fmt.Sprintf("%8.3fs", time.Since(start).Seconds())
}

func rawLen(m midi.Message) int {
	switch {
	case m.Kind == midi.ProgramChange || m.Kind == midi.ChannelPressure:
		return 2
	case m.Kind == midi.Other:
		return 1
	}
	return 3
}
