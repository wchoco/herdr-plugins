// Package herdr is yubi's thin adapter to herdr 0.9.5: the NDJSON socket
// client, the OSC 52 clipboard and the overlay pane's terminal. It holds no
// logic of its own; it implements the interfaces of the core session
// package.
package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
)

// APIError is an error response from herdr, e.g. code "pane_not_found".
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error returns the code, which is what yubi shows to the user.
func (e *APIError) Error() string { return e.Code }

// Client calls herdr's socket API. herdr answers one request per
// connection, so every call dials afresh.
type Client struct {
	path string
	seq  atomic.Int64
}

// NewClient returns a client for the socket at path ($HERDR_SOCKET_PATH).
func NewClient(path string) *Client { return &Client{path: path} }

// unreachableError is a failure to connect to herdr's socket.
type unreachableError struct{ err error }

func (e *unreachableError) Error() string { return "socket unreachable" }
func (e *unreachableError) Unwrap() error { return e.err }

// Call sends one request and decodes the response's result into result.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	conn, _, err := c.request(ctx, method, params, result)
	if conn != nil {
		conn.Close()
	}
	return err
}

// request dials, sends one request and reads the first response line,
// decoding its result into result. On success it returns the connection
// and its reader for a caller that keeps reading; otherwise the connection
// is closed.
func (c *Client) request(ctx context.Context, method string, params, result any) (net.Conn, *bufio.Reader, error) {
	if params == nil {
		params = struct{}{}
	}
	req, err := json.Marshal(struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{ID: "yubi:" + strconv.FormatInt(c.seq.Add(1), 10), Method: method, Params: params})
	if err != nil {
		return nil, nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.path)
	if err != nil {
		return nil, nil, &unreachableError{err}
	}
	fail := func(err error) (net.Conn, *bufio.Reader, error) {
		conn.Close()
		return nil, nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fail(fmt.Errorf("herdr socket: %w", err))
	}
	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return fail(fmt.Errorf("herdr socket: %w", err))
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *APIError       `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fail(fmt.Errorf("herdr socket: %w", err))
	}
	if resp.Error != nil {
		return fail(resp.Error)
	}
	if result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fail(fmt.Errorf("herdr socket: %w", err))
		}
	}
	return conn, r, nil
}

// ReadVisible implements session.PaneSource with pane.read, source visible.
func (c *Client) ReadVisible(ctx context.Context, pane string, f session.Format) (string, error) {
	format := "text"
	if f == session.FormatANSI {
		format = "ansi"
	}
	var res struct {
		Read struct {
			Text string `json:"text"`
		} `json:"read"`
	}
	err := c.Call(ctx, "pane.read", map[string]any{"pane_id": pane, "source": "visible", "format": format}, &res)
	return res.Read.Text, err
}

// OpenOverlay opens yubi's overlay pane over the focused pane with
// plugin.pane.open, handing it the Target Pane and mode, plus the extra
// environment variables, and focuses it. It returns the overlay's pane id.
func (c *Client) OpenOverlay(ctx context.Context, pluginID, target string, mode session.Mode, extra map[string]string) (string, error) {
	env := map[string]string{"YUBI_TARGET_PANE": target, "YUBI_MODE": string(mode)}
	for k, v := range extra {
		env[k] = v
	}
	var res struct {
		PluginPane struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"plugin_pane"`
	}
	err := c.Call(ctx, "plugin.pane.open", map[string]any{
		"plugin_id":  pluginID,
		"entrypoint": "ui",
		"env":        env,
		"focus":      true,
	}, &res)
	return res.PluginPane.Pane.PaneID, err
}
