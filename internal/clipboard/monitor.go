package clipboard

import (
	"context"
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
	content, err := read(sel)
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

func read(sel SelectionType) (string, error) {
	var cmd *exec.Cmd
	switch sel {
	case SelectionPrimary:
		cmd = exec.Command("xclip", "-selection", "primary", "-o")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func Write(content string) error {
	exec.Command("pkill", "-f", "xclip.*clipboard").Run()
	cmd := exec.Command("xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(content)
	return cmd.Start()
}
