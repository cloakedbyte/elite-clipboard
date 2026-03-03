package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/yourusername/elite-clipboard/internal/classifier"
	"github.com/yourusername/elite-clipboard/internal/clipboard"
	"github.com/yourusername/elite-clipboard/internal/config"
	"github.com/yourusername/elite-clipboard/internal/db"
	"github.com/yourusername/elite-clipboard/internal/ipc"
)

const banner = `
  +-----------------------------------------+
  |  elite-clipboard  ::  daemon starting   |
  +-----------------------------------------+`

func main() {
	fmt.Println(banner)

	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".config", "elite-clipboard", "config.json")
	dbPath  := filepath.Join(home, ".local", "share", "elite-clipboard", "store.db")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("[fatal] mkdir db dir: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("[fatal] load config: %v", err)
	}
	log.Printf("[init]  config loaded  :: items_cap=%d  poll=%dms", cfg.MaxItems, cfg.PollIntervalMS)

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("[fatal] open db: %v", err)
	}
	defer store.Close()
	log.Printf("[init]  db ready       :: %s", dbPath)

	mon := clipboard.NewMonitor(cfg.PollIntervalMS, cfg.MonitorPrimary)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// -- clipboard monitor goroutine --
	go mon.Run(ctx)
	go func() {
		for event := range mon.Events() {
			content := strings.TrimSpace(event.Content)
			if len(content) < cfg.MinContentLength {
				continue
			}
			if store.Exists(content) {
				continue
			}

			res := classifier.Classify(content)

			item := &db.Item{
				Content:       content,
				MimeType:      "text/plain",
				Category:      res.Category,
				WorkspaceID:   res.WorkspaceID,
				Tags:          strings.Join(res.Tags, ","),
				SelectionType: string(event.SelectionType),
				Sensitive:     boolInt(res.Sensitive),
			}

			id, err := store.Insert(item)
			if err != nil {
				log.Printf("[error] insert: %v", err)
				continue
			}

			wsLimits := map[string]int{}
			for k, v := range cfg.Workspaces {
				wsLimits[k] = v.MaxItems
			}
			store.EnforceLimits(cfg.MaxItems, wsLimits)

			if res.Sensitive {
				log.Printf("[clip]  #%-5d  ws=%-2d  cat=%-10s  [SENSITIVE REDACTED]",
					id, res.WorkspaceID, res.Category)
			} else {
				preview := content
				if len(preview) > 60 {
					preview = preview[:60] + "..."
				}
				log.Printf("[clip]  #%-5d  ws=%-2d  cat=%-10s  %q",
					id, res.WorkspaceID, res.Category, preview)
			}
		}
	}()
	log.Printf("[init]  monitor ready  :: primary=%v", cfg.MonitorPrimary)

	// -- IPC server goroutine --
	srv := ipc.NewServer(func(req ipc.Request) ipc.Response {
		switch req.Action {

		case "search":
			limit := req.Limit
			if limit <= 0 {
				limit = 50
			}
			items, err := store.Search(req.Query, req.WorkspaceID, limit, req.PinnedOnly)
			if err != nil {
				return ipc.Err(err)
			}
			return ipc.OK(items)

		case "pin":
			if err := store.Pin(req.ID, true); err != nil {
				return ipc.Err(err)
			}
			return ipc.OK("pinned")

		case "unpin":
			if err := store.Pin(req.ID, false); err != nil {
				return ipc.Err(err)
			}
			return ipc.OK("unpinned")

		case "delete":
			if err := store.Delete(req.ID); err != nil {
				return ipc.Err(err)
			}
			return ipc.OK("deleted")

		case "clear":
			if err := store.Clear(req.WorkspaceID); err != nil {
				return ipc.Err(err)
			}
			return ipc.OK("cleared")

		case "workspace":
			wsID, err := strconv.Atoi(req.Query)
			if err != nil {
				return ipc.Err(fmt.Errorf("invalid workspace id: %s", req.Query))
			}
			items, err := store.Search("", &wsID, 100, false)
			if err != nil {
				return ipc.Err(err)
			}
			return ipc.OK(items)

		case "ping":
			return ipc.OK("pong")

		default:
			return ipc.Err(fmt.Errorf("unknown action: %s", req.Action))
		}
	})

	go func() {
		if err := srv.Listen(); err != nil {
			log.Printf("[error] ipc: %v", err)
		}
	}()
	log.Printf("[init]  ipc ready      :: %s", ipc.SocketPath)
	log.Printf("[ready] daemon running  -- send SIGINT or SIGTERM to stop")

	// -- graceful shutdown --
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("[exit]  signal received -- shutting down cleanly")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
