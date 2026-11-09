//go:build !darwin

package midi

// Client is not yet implemented on this platform (Windows: winmm, planned).
type Client struct{}

func Open(onEvent func(Event), onChange func([]Source)) (*Client, error) {
	return nil, ErrUnsupportedPlatform
}
func (c *Client) Refresh() bool     { return false }
func (c *Client) Sources() []Source { return nil }
func (c *Client) Close() error      { return nil }
func RunLoop()                      {}
