package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloakedbyte/elite-clipboard/internal/clipboard"
	"github.com/cloakedbyte/elite-clipboard/internal/db"
	"github.com/cloakedbyte/elite-clipboard/internal/ipc"
)

const (
	colReset  = "\033[0m"
	colBold   = "\033[1m"
	colDim    = "\033[2m"
	colGreen  = "\033[32m"
	colYellow = "\033[33m"
	colCyan   = "\033[36m"
	colRed    = "\033[31m"
	colWhite  = "\033[97m"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}

	switch os.Args[1] {

	case "search", "s":
		query := ""
		if len(os.Args) >= 3 {
			query = strings.Join(os.Args[2:], " ")
		}
		resp := must(ipc.Send(ipc.Request{Action: "search", Query: query, Limit: 50}))
		printItems(resp.Data)

	case "ws":
		if len(os.Args) < 3 {
			die("usage: ecb ws <0-4>")
		}
		resp := must(ipc.Send(ipc.Request{Action: "workspace", Query: os.Args[2]}))
		printItems(resp.Data)

	case "pin":
		id := mustID(2)
		resp := must(ipc.Send(ipc.Request{Action: "pin", ID: id}))
		ok(resp)

	case "unpin":
		id := mustID(2)
		resp := must(ipc.Send(ipc.Request{Action: "unpin", ID: id}))
		ok(resp)

	case "del", "delete":
		id := mustID(2)
		resp := must(ipc.Send(ipc.Request{Action: "delete", ID: id}))
		ok(resp)

	case "clear":
		var wsID *int
		if len(os.Args) >= 3 {
			n, err := strconv.Atoi(os.Args[2])
			if err != nil {
				die("workspace id must be integer")
			}
			wsID = &n
		}
		resp := must(ipc.Send(ipc.Request{Action: "clear", WorkspaceID: wsID}))
		ok(resp)

	case "pinned":
		resp := must(ipc.Send(ipc.Request{Action: "search", PinnedOnly: true, Limit: 100}))
		printItems(resp.Data)

	case "copy", "c":
		id := mustID(2)
		resp := must(ipc.Send(ipc.Request{Action: "search", Query: "", Limit: 500}))
		data, _ := json.Marshal(resp.Data)
		var items []db.Item
		json.Unmarshal(data, &items)
		for _, item := range items {
			if item.ID == id {
				if err := clipboard.Write(item.Content); err != nil {
					die("xclip write failed: " + err.Error())
				}
				fmt.Printf("%s  copied #%d to clipboard%s\n", colGreen+"[+]"+colReset, id, colReset)
				return
			}
		}
		die("id not found")

	case "ping":
		resp := must(ipc.Send(ipc.Request{Action: "ping"}))
		fmt.Printf("%s  daemon is alive%s\n", colGreen+"[+]"+colReset, colReset)
		_ = resp

	case "help", "--help", "-h":
		usage()

	default:
		fmt.Printf("%s unknown command: %s%s\n", colRed+"[!]"+colReset, os.Args[1], colReset)
		usage()
		os.Exit(1)
	}
}

func printItems(raw interface{}) {
	data, _ := json.Marshal(raw)
	var items []db.Item
	if err := json.Unmarshal(data, &items); err != nil || len(items) == 0 {
		fmt.Printf("%s  no results%s\n", colDim+"[-]", colReset)
		return
	}

	wsNames := map[int]string{
		0: "work", 1: "code", 2: "research", 3: "temp", 4: "sensitive",
	}
	catColor := map[string]string{
		"code": colCyan, "url": colGreen, "json": colYellow,
		"email": colGreen, "sensitive": colRed, "text": colWhite,
	}

	fmt.Printf("\n%s  %d items%s\n\n", colBold+"[*]", len(items), colReset)
	fmt.Printf("  %s%-6s  %-10s  %-10s  %-5s  %s%s\n",
		colDim, "ID", "CATEGORY", "WORKSPACE", "PIN", "CONTENT", colReset)
	fmt.Printf("  %s%s%s\n", colDim, strings.Repeat("-", 80), colReset)

	for _, item := range items {
		ts := time.UnixMilli(item.Timestamp).Format("01-02 15:04")
		pin := " "
		if item.Pinned == 1 {
			pin = colYellow + "*" + colReset
		}
		ws := wsNames[item.WorkspaceID]
		cat := item.Category
		cc := catColor[cat]
		if cc == "" {
			cc = colWhite
		}

		preview := strings.ReplaceAll(item.Content, "\n", " ")
		preview = strings.TrimSpace(preview)
		if len(preview) > 55 {
			preview = preview[:55] + "..."
		}
		if item.Sensitive == 1 {
			preview = colRed + "[sensitive content redacted]" + colReset
		}

		fmt.Printf("  #%-5d  %s%-10s%s  %-10s  %s  %s%s%s  %s\n",
			item.ID,
			cc, cat, colReset,
			ws,
			pin,
			colDim, ts, colReset,
			preview,
		)
	}
	fmt.Println()
}

func usage() {
	fmt.Printf(`
%selite-clipboard :: ecb%s

  %sCOMMANDS%s
    search  [query]      search clipboard history
    ws      <0-4>        list items by workspace
    pin     <id>         pin an item
    unpin   <id>         unpin an item
    del     <id>         soft-delete an item
    clear   [ws]         clear history (optionally by workspace)
    pinned               list all pinned items
    ping                 check daemon status

  %sWORKSPACES%s
    0  work        1  code        2  research
    3  temp        4  sensitive

`, colBold, colReset, colBold, colReset, colBold, colReset)
}

func must(resp ipc.Response, err error) ipc.Response {
	if err != nil {
		fmt.Printf("%s  daemon unreachable: %v%s\n", colRed+"[!]"+colReset, err, colReset)
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Printf("%s  %s%s\n", colRed+"[!]"+colReset, resp.Error, colReset)
		os.Exit(1)
	}
	return resp
}

func ok(resp ipc.Response) {
	fmt.Printf("%s  %v%s\n", colGreen+"[+]"+colReset, resp.Data, colReset)
}

func mustID(argPos int) int64 {
	if len(os.Args) <= argPos {
		die("missing item id")
	}
	n, err := strconv.ParseInt(os.Args[argPos], 10, 64)
	if err != nil {
		die("id must be integer")
	}
	return n
}

func die(msg string) {
	fmt.Printf("%s  %s%s\n", colRed+"[!]"+colReset, msg, colReset)
	os.Exit(1)
}
