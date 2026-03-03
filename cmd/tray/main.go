package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/cloakedbyte/elite-clipboard/internal/clipboard"
	"github.com/cloakedbyte/elite-clipboard/internal/db"
	"github.com/cloakedbyte/elite-clipboard/internal/ipc"
)

var wsNames = []string{"Work", "Code", "Research", "Temp", "Sensitive"}

type TrayApp struct {
	fyneApp     fyne.App
	win         fyne.Window
	items       []db.Item
	filtered    []db.Item
	listData    binding.StringList
	searchVal   string
	activeWS    int
	selectedID  int64
	statusLabel *widget.Label
	list        *widget.List
}

func main() {
	a := &TrayApp{
		listData:   binding.NewStringList(),
		activeWS:   -1,
		selectedID: -1,
	}

	a.fyneApp = app.NewWithID("io.elite.clipboard")
	a.fyneApp.Settings().SetTheme(theme.DarkTheme())

	a.win = a.fyneApp.NewWindow("elite-clipboard")
	a.win.Resize(fyne.NewSize(620, 580))
	a.win.CenterOnScreen()
	a.win.SetCloseIntercept(func() { a.win.Hide() })

	a.statusLabel = widget.NewLabelWithStyle(
		"connecting...",
		fyne.TextAlignCenter,
		fyne.TextStyle{Italic: true},
	)

	a.win.SetContent(a.buildUI())

	if desk, ok := a.fyneApp.(desktop.App); ok {
		desk.SetSystemTrayIcon(theme.ContentCopyIcon())
		desk.SetSystemTrayMenu(fyne.NewMenu("elite-clipboard",
			fyne.NewMenuItem("Open", func() {
				a.win.Show()
				a.win.RequestFocus()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() { a.fyneApp.Quit() }),
		))
	}

	go func() {
		for {
			resp, err := ipc.Send(ipc.Request{Action: "search", Limit: 200})
			if err != nil || !resp.OK {
				fyne.Do(func() {
					a.statusLabel.SetText("daemon unreachable -- retrying...")
				})
				time.Sleep(3 * time.Second)
				continue
			}
			raw, _ := json.Marshal(resp.Data)
			var items []db.Item
			if json.Unmarshal(raw, &items) != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			fyne.Do(func() {
				a.items = items
				a.applyFilter()
				a.statusLabel.SetText(fmt.Sprintf(
					"%d items  ::  synced %s",
					len(a.items),
					time.Now().Format("15:04:05"),
				))
			})
			time.Sleep(2 * time.Second)
		}
	}()

	a.win.ShowAndRun()
}

func (a *TrayApp) buildUI() fyne.CanvasObject {
	// -- search --
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("  search clipboard history...")
	searchEntry.OnChanged = func(q string) {
		a.searchVal = q
		a.applyFilter()
	}

	// -- list --
	a.list = widget.NewListWithData(
		a.listData,
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			lbl.Truncation = fyne.TextTruncateEllipsis
			return lbl
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			val, _ := i.(binding.String).Get()
			o.(*widget.Label).SetText(val)
		},
	)

	a.list.OnSelected = func(id widget.ListItemID) {
		if id >= len(a.filtered) {
			return
		}
		a.selectedID = a.filtered[id].ID
	}

	// double tap to copy
	// Fyne doesn't expose double-click on list natively, so we use a Copy button
	// and also single-tap copies directly
	a.list.OnSelected = func(id widget.ListItemID) {
		if id >= len(a.filtered) {
			return
		}
		item := a.filtered[id]
		a.selectedID = item.ID

		// copy to clipboard in goroutine so xclip finishes before anything else
		go func(content string) {
			if err := clipboard.Write(content); err != nil {
				fyne.Do(func() {
					a.statusLabel.SetText("copy failed: " + err.Error())
				})
				return
			}
			fyne.Do(func() {
				a.statusLabel.SetText(fmt.Sprintf(
					"copied #%d  ::  %s",
					item.ID,
					time.Now().Format("15:04:05"),
				))
			})
		}(item.Content)

		a.list.UnselectAll()
	}

	// -- action toolbar --
	copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		if a.selectedID < 0 {
			a.statusLabel.SetText("select an item first")
			return
		}
		for _, item := range a.filtered {
			if item.ID == a.selectedID {
				go func(content string, id int64) {
					clipboard.Write(content)
					fyne.Do(func() {
						a.statusLabel.SetText(fmt.Sprintf("copied #%d", id))
					})
				}(item.Content, item.ID)
				return
			}
		}
	})

	pinBtn := widget.NewButtonWithIcon("Pin", theme.RadioButtonCheckedIcon(), func() {
		if a.selectedID < 0 {
			a.statusLabel.SetText("select an item first")
			return
		}
		id := a.selectedID
		go func() {
			ipc.Send(ipc.Request{Action: "pin", ID: id})
			fyne.Do(func() {
				a.statusLabel.SetText(fmt.Sprintf("pinned #%d", id))
				a.selectedID = -1
			})
		}()
	})

	deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		if a.selectedID < 0 {
			a.statusLabel.SetText("select an item first")
			return
		}
		id := a.selectedID
		dialog.ShowConfirm("Delete", fmt.Sprintf("Delete item #%d?", id), func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ipc.Send(ipc.Request{Action: "delete", ID: id})
				fyne.Do(func() {
					a.statusLabel.SetText(fmt.Sprintf("deleted #%d", id))
					a.selectedID = -1
				})
			}()
		}, a.win)
	})
	deleteBtn.Importance = widget.DangerImportance

	clearBtn := widget.NewButtonWithIcon("Clear All", theme.ContentClearIcon(), func() {
		dialog.ShowConfirm(
			"Clear History",
			"Delete all unpinned items?",
			func(ok bool) {
				if !ok {
					return
				}
				var wsID *int
				if a.activeWS >= 0 {
					wsID = &a.activeWS
				}
				go func() {
					ipc.Send(ipc.Request{Action: "clear", WorkspaceID: wsID})
					fyne.Do(func() {
						a.statusLabel.SetText("history cleared")
						a.selectedID = -1
					})
				}()
			}, a.win)
	})
	clearBtn.Importance = widget.DangerImportance

	toolbar := container.NewGridWithColumns(4, copyBtn, pinBtn, deleteBtn, clearBtn)

	// -- workspace filter --
	wsBar := container.NewHBox()
	wsBar.Add(widget.NewButton("All", func() {
		a.activeWS = -1
		a.applyFilter()
	}))
	for i, name := range wsNames {
		wsID := i
		wsBar.Add(widget.NewButton(name, func() {
			a.activeWS = wsID
			a.applyFilter()
		}))
	}

	return container.NewBorder(
		container.NewVBox(searchEntry, widget.NewSeparator()),
		container.NewVBox(
			widget.NewSeparator(),
			toolbar,
			widget.NewSeparator(),
			container.NewHScroll(wsBar),
			a.statusLabel,
		),
		nil, nil,
		a.list,
	)
}

func (a *TrayApp) applyFilter() {
	var out []db.Item
	for _, item := range a.items {
		if a.activeWS >= 0 && item.WorkspaceID != a.activeWS {
			continue
		}
		if a.searchVal != "" &&
			!strings.Contains(strings.ToLower(item.Content), strings.ToLower(a.searchVal)) {
			continue
		}
		out = append(out, item)
	}
	a.filtered = out

	lines := make([]string, len(out))
	for i, item := range out {
		pin := " "
		if item.Pinned == 1 {
			pin = "*"
		}
		preview := strings.ReplaceAll(item.Content, "\n", " ")
		if len(preview) > 68 {
			preview = preview[:68] + "..."
		}
		if item.Sensitive == 1 {
			preview = "[sensitive content redacted]"
		}
		lines[i] = fmt.Sprintf("%s #%-4d  %-10s  %s", pin, item.ID, item.Category, preview)
	}
	a.listData.Set(lines)
}
