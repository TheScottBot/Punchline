// Package uvcfake is an in-memory uvc.Backend for tests of adapters and
// camera models.
package uvcfake

import (
	"sync"

	"github.com/thescottbot/punchline/internal/uvc"
)

// Backend serves Nodes; every node reports Range. Sets records every raw
// zoom value written, across all controls.
type Backend struct {
	mu    sync.Mutex
	Nodes []uvc.Node
	Range uvc.Range
	sets  []int32
}

func (b *Backend) Enumerate() ([]uvc.Node, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]uvc.Node(nil), b.Nodes...), nil
}

func (b *Backend) Open(path string) (uvc.Control, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, n := range b.Nodes {
		if n.Path == path {
			return &control{b: b, cur: b.Range.Default}, nil
		}
	}
	return nil, uvc.ErrNotFound
}

// Sets returns the raw values written so far.
func (b *Backend) Sets() []int32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]int32(nil), b.sets...)
}

type control struct {
	b   *Backend
	cur int32
}

func (c *control) ZoomRange() (uvc.Range, error) { return c.b.Range, nil }
func (c *control) Zoom() (int32, error)          { return c.cur, nil }
func (c *control) SetZoom(v int32) error {
	c.b.mu.Lock()
	defer c.b.mu.Unlock()
	c.cur = v
	c.b.sets = append(c.b.sets, v)
	return nil
}
func (c *control) Close() error { return nil }
