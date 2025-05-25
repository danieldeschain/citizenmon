package ui

import (
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"game-monitor/pkg/processor"
	"game-monitor/pkg/watcher"
)

// notification represents an overlay pop-up with its own timer.
type notification struct {
	pop   *widget.PopUp
	timer *time.Timer
}

// logHandlerAdapter routes processed log events into the UI and manages overlays.
type logHandlerAdapter struct {
	proc          *processor.Processor
	outputRich    *widget.RichText
	window        fyne.Window
	notifications []*notification
}

// DetectPlayerName delegates to the core processor.
func (a *logHandlerAdapter) DetectPlayerName(line string) {
	a.proc.DetectPlayerName(line)
}

// ProcessLogLine delegates to the core processor.
func (a *logHandlerAdapter) ProcessLogLine(line string) {
	a.proc.ProcessLogLine(line)
}

// showPopup displays a top-right notification that stacks up to three entries
// and auto-hides each independently after 3 seconds.
func (a *logHandlerAdapter) showPopup(message string) {
	canv := a.window.Canvas()
	lbl := canvas.NewText(message, color.White)
	lbl.TextStyle = fyne.TextStyle{Bold: true}
	pop := widget.NewPopUp(container.NewVBox(lbl), canv)

	// Prepend the new notification
	n := &notification{pop: pop}
	a.notifications = append([]*notification{n}, a.notifications...)

	// If more than three, hide the oldest
	if len(a.notifications) > 3 {
		old := a.notifications[3]
		old.pop.Hide()
		if old.timer != nil {
			old.timer.Stop()
		}
		a.notifications = a.notifications[:3]
	}

	// Reposition all notifications
	for idx, notif := range a.notifications {
		offset := float32(10)
		for i := 0; i < idx; i++ {
			offset += a.notifications[i].pop.Content.MinSize().Height + 10
		}
		notif.pop.Move(fyne.NewPos(canv.Size().Width-notif.pop.Size().Width-10, offset))
		notif.pop.Show()
	}

	// Schedule independent hide for this notification
	n.timer = time.AfterFunc(3*time.Second, func() {
		fyne.Do(func() {
			n.pop.Hide()
			// Remove this notification
			for i, x := range a.notifications {
				if x == n {
					a.notifications = append(a.notifications[:i], a.notifications[i+1:]...)
					break
				}
			}
			// Reposition remaining
			for idx, remaining := range a.notifications {
				offset := float32(10)
				for i := 0; i < idx; i++ {
					offset += a.notifications[i].pop.Content.MinSize().Height + 10
				}
				remaining.pop.Move(fyne.NewPos(canv.Size().Width-remaining.pop.Size().Width-10, offset))
			}
		})
	})
}

// AppendOutput writes a processed line to the terminal and triggers overlay popups.
func (a *logHandlerAdapter) AppendOutput(line string) {
	fyne.Do(func() {
		// Update player label
		if a.proc.PlayerLabel != nil && a.proc.PlayerName != "" {
			a.proc.PlayerLabel.SetText(a.proc.PlayerName)
		}

		// Define text styles
		defaultStyle := widget.RichTextStyle{Inline: true, ColorName: theme.ColorNamePlaceHolder, Alignment: fyne.TextAlignLeading}
		highlightStyle := widget.RichTextStyle{Inline: true, ColorName: theme.ColorNamePrimary, Alignment: fyne.TextAlignLeading}

		// Build rich-text segments with timestamp prefix
		fields := strings.Fields(line)
		// prepend timestamp
		ts := time.Now().Format("15:04:05")
		segments := []widget.RichTextSegment{&widget.TextSegment{Text: ts + " ", Style: defaultStyle}}
		// append message words
		for i, w := range fields {
			clean := strings.Trim(w, ",.?!;:'\"[]()")
			style := defaultStyle
			if clean == a.proc.PlayerName || (i > 0 && unicode.IsUpper([]rune(clean)[0])) {
				style = highlightStyle
			}
			segments = append(segments, &widget.TextSegment{Text: w, Style: style})
			if i < len(fields)-1 {
				segments = append(segments, &widget.TextSegment{Text: " ", Style: defaultStyle})
			}
		}
		segments = append(segments, &widget.TextSegment{Text: "\n", Style: defaultStyle})
		a.outputRich.Segments = append(a.outputRich.Segments, segments...)
		a.outputRich.Refresh()

		// Overlay on kill events
		if strings.HasPrefix(line, "You were killed by:") {
			killer := strings.TrimSpace(strings.TrimPrefix(line, "You were killed by:"))
			a.showPopup(fmt.Sprintf("Killed by %s", killer))
		} else if strings.HasPrefix(line, "You killed:") {
			victim := strings.TrimSpace(strings.TrimPrefix(line, "You killed:"))
			a.showPopup(fmt.Sprintf("%s was killed", victim))
		}
	})
}

// Run sets up and runs the Game Monitor UI.
func Run() {
	app := app.NewWithID("io.yourname.gamemonitor")
	window := app.NewWindow("Game Monitor UI")

	// Load preferences
	prefs := app.Preferences()
	saved := prefs.String("logPath")

	// UI components
	playerLabel := widget.NewLabel("<none>")
	outputRich := widget.NewRichText()
	outputRich.Wrapping = fyne.TextWrapWord

	// Processor and handler
	core := processor.New(nil, playerLabel)
	h := &logHandlerAdapter{proc: core, outputRich: outputRich, window: window}
	core.AppendOutput = h.AppendOutput

	// Config tab
	logEntry := widget.NewEntry()
	logEntry.SetPlaceHolder(`Path to your \\Roberts Space Industries\\StarCitizen\\LIVE\\game.log file`)
	if saved != "" {
		logEntry.SetText(saved)
	}
	browse := widget.NewButton("Browse…", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if uri != nil && err == nil {
				u, _ := url.Parse(uri.String())
				logEntry.SetText(filepath.Join(u.Path, "game.log"))
			}
		}, window)
	})
	start := widget.NewButton("Start Monitor", func() {
		path := logEntry.Text
		if _, err := os.Stat(path); err != nil {
			dialog.ShowError(fmt.Errorf("log file not found: %s", path), window)
			return
		}
		prefs.SetString("logPath", path)
		h.AppendOutput("Monitoring: " + path)
		go watcher.WatchLogFile(path, h)
	})
	configTab := container.NewVBox(
		widget.NewLabel("Log File Path:"),
		container.NewBorder(nil, nil, nil, browse, logEntry),
		start,
	)

	// Terminal tab
	scroll := container.NewScroll(outputRich)
	head := container.NewVBox(
		widget.NewLabelWithStyle("Current Player:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		playerLabel,
		widget.NewLabel("Log Output:"),
	)
	termTab := container.NewBorder(head, nil, nil, nil, scroll)

	tabs := container.NewAppTabs(
		container.NewTabItem("Config", configTab),
		container.NewTabItem("Terminal", termTab),
	)

	// Show window
	window.SetContent(tabs)
	window.Resize(fyne.NewSize(800, 500))
	window.ShowAndRun()
}
