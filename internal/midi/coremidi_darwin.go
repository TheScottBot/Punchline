//go:build darwin

package midi

// CoreMIDI backend. CoreMIDI is multi-client, so listening never takes a
// device away from RØDE Central, a DAW or anything else.

/*
#cgo CFLAGS: -fblocks
#cgo LDFLAGS: -framework CoreMIDI -framework CoreFoundation
#include "coremidi_darwin.h"
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
)

// Client listens to every MIDI source, including ones connected later.
type Client struct {
	mu        sync.Mutex
	onEvent   func(Event)
	onChange  func([]Source)
	connected map[int32]Source
	sources   []Source
	closed    bool
}

var (
	activeMu sync.Mutex
	active   *Client
)

// Open starts listening. onEvent runs on a CoreMIDI thread for every message
// and must not block. onChange (may be nil) runs when the set of sources
// changes. Source changes are reported on the run loop of the thread that
// called Open (the app's main thread; command-line tools call RunLoop).
// Only one Client may be open at a time.
func Open(onEvent func(Event), onChange func([]Source)) (*Client, error) {
	activeMu.Lock()
	defer activeMu.Unlock()
	if active != nil {
		return nil, fmt.Errorf("midi: already open")
	}
	c := &Client{onEvent: onEvent, connected: map[int32]Source{}}
	active = c
	if st := C.pl_midi_open(); st != 0 {
		active = nil
		return nil, fmt.Errorf("midi: CoreMIDI client: OSStatus %d", int(st))
	}
	c.Refresh() // the initial set is not a change
	c.mu.Lock()
	c.onChange = onChange
	c.mu.Unlock()
	return c, nil
}

// Refresh connects any new sources and reports whether the set changed.
// Safe from any goroutine; also called on CoreMIDI's setup-changed notice.
func (c *Client) Refresh() bool {
	n := int(C.pl_midi_source_count())
	var now []Source
	var name [256]C.char
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	seen := map[int32]bool{}
	for i := 0; i < n; i++ {
		var uid C.int32_t
		if C.pl_midi_source(C.int(i), &uid, &name[0], C.int(len(name))) != 0 {
			continue
		}
		s := Source{ID: int32(uid), Name: C.GoString(&name[0])}
		now = append(now, s)
		seen[s.ID] = true
		if _, ok := c.connected[s.ID]; !ok {
			if C.pl_midi_connect(C.int(i), uid) == 0 {
				c.connected[s.ID] = s
			}
		} else {
			c.connected[s.ID] = s // name may change
		}
	}
	for id := range c.connected {
		if !seen[id] {
			delete(c.connected, id) // CoreMIDI drops the connection itself
		}
	}
	changed := !sameSources(now, c.sources)
	c.sources = now
	cb := c.onChange
	c.mu.Unlock()
	if changed && cb != nil {
		cb(append([]Source(nil), now...))
	}
	return changed
}

func sameSources(a, b []Source) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Sources returns the current sources.
func (c *Client) Sources() []Source {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Source(nil), c.sources...)
}

// Close stops listening.
func (c *Client) Close() error {
	activeMu.Lock()
	defer activeMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	C.pl_midi_close()
	if active == c {
		active = nil
	}
	return nil
}

// RunLoop runs the calling thread's run loop forever, for command-line tools
// that need source-change notifications. Call it from the thread that called
// Open, locked with runtime.LockOSThread.
func RunLoop() { C.pl_midi_run() }

func current() *Client {
	activeMu.Lock()
	defer activeMu.Unlock()
	return active
}

//export goMIDIEvent
func goMIDIEvent(src C.uint32_t, status, d1, d2 C.uint8_t) {
	c := current()
	if c == nil {
		return
	}
	at := time.Now()
	c.mu.Lock()
	s, ok := c.connected[int32(src)]
	cb := c.onEvent
	closed := c.closed
	c.mu.Unlock()
	if closed || cb == nil {
		return
	}
	if !ok {
		s = Source{ID: int32(src)}
	}
	cb(Event{Source: s, Msg: Decode(byte(status), byte(d1), byte(d2)), At: at})
}

//export goMIDISetupChanged
func goMIDISetupChanged() {
	if c := current(); c != nil {
		c.Refresh()
	}
}
