package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type SelectionType string

const (
	SelectionClipboard SelectionType = "clipboard"
	SelectionPrimary   SelectionType = "primary"
)

type Event struct {
	Content       string
	SelectionType SelectionType
}

// backend represents the underlying clipboard mechanism.
type backend int

const (
	backendX11     backend = iota
	backendWayland backend = iota
)

// activeBackend is resolved once at package init so both Monitor and Write use
// the same backend without re-checking on every call.
var activeBackend = detectBackend()

// detectBackend returns backendWayland when WAYLAND_DISPLAY is set and
// wl-paste is available; otherwise it falls back to X11/xclip.
// It prints a clear diagnostic line to stderr so the daemon log always
// records which path was chosen (or why a fallback occurred).
func detectBackend() backend {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			fmt.Fprintln(os.Stderr, "[clipboard] backend: Wayland (wl-clipboard)")
			return backendWayland
		}
		// Wayland session but wl-paste is missing — this will not work; tell the user.
		fmt.Fprintln(os.Stderr, "[clipboard] WARNING: WAYLAND_DISPLAY is set but wl-paste is not found.")
		fmt.Fprintln(os.Stderr, "[clipboard]          Install wl-clipboard:  sudo apt install wl-clipboard")
		fmt.Fprintln(os.Stderr, "[clipboard]          Falling back to X11/xclip (clipboard will NOT work until fixed).")
	}
	fmt.Fprintln(os.Stderr, "[clipboard] backend: X11 (xclip)")
	return backendX11
}

type Monitor struct {
	pollInterval time.Duration
	last         map[SelectionType]string
	events       chan Event
	monitorXSel  bool
}

func NewMonitor(pollIntervalMS int, monitorPrimary bool) *Monitor {
	return &Monitor{
		pollInterval: time.Duration(pollIntervalMS) * time.Millisecond,
		last:         map[SelectionType]string{},
		events:       make(chan Event, 64),
		monitorXSel:  monitorPrimary,
	}
}

func (m *Monitor) Events() <-chan Event {
	return m.events
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(SelectionClipboard)
			if m.monitorXSel {
				m.poll(SelectionPrimary)
			}
		}
	}
}

func (m *Monitor) poll(sel SelectionType) {
	content, err := readClipboard(sel)
	if err != nil || strings.TrimSpace(content) == "" {
		return
	}
	if content == m.last[sel] {
		return
	}
	m.last[sel] = content
	select {
	case m.events <- Event{Content: content, SelectionType: sel}:
	default:
	}
}

func readClipboard(sel SelectionType) (string, error) {
	var cmd *exec.Cmd
	switch activeBackend {
	case backendWayland:
		if sel == SelectionPrimary {
			cmd = exec.Command("wl-paste", "--no-newline", "--primary")
		} else {
			cmd = exec.Command("wl-paste", "--no-newline")
		}
	default: // backendX11
		if sel == SelectionPrimary {
			cmd = exec.Command("xclip", "-selection", "primary", "-o")
		} else {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Write pushes content to the system clipboard using the detected backend.
func Write(content string) error {
	switch activeBackend {
	case backendWayland:
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(content)
		return cmd.Run()
	default: // backendX11
		exec.Command("pkill", "-f", "xclip.*clipboard").Run()
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(content)
		return cmd.Start()
	}
}
