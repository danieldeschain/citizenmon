package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"game-monitor/pkg/processor"
	"game-monitor/pkg/stats"
	"game-monitor/pkg/watcher"
)

// logHandlerAdapter routes processed log events into the UI and uses native Fyne toasts.
type logHandlerAdapter struct {
	proc          *processor.Processor
	outputRich    *widget.RichText
	window        fyne.Window
	onStatsUpdate func(playerName string) // callback to update stats
}

func (a *logHandlerAdapter) DetectPlayerName(line string) {
	a.proc.DetectPlayerName(line)
	if a.onStatsUpdate != nil {
		a.onStatsUpdate(a.proc.PlayerName)
	}
}

func (a *logHandlerAdapter) ProcessLogLine(line string) {
	a.proc.ProcessLogLine(line)
}

// showPopup sends a native Fyne toast notification above all windows.
func (a *logHandlerAdapter) showPopup(message string) {
	notif := fyne.NewNotification("Game Monitor", message)
	fyne.CurrentApp().SendNotification(notif)
}

// AppendOutput writes a processed log line into the rich terminal and fires toasts on kills.
func (a *logHandlerAdapter) AppendOutput(line string) {
	// Always wrap in fyne.Do to ensure UI thread safety
	fyne.Do(func() {
		if a.proc.PlayerLabel != nil && a.proc.PlayerName != "" {
			a.proc.PlayerLabel.SetText(a.proc.PlayerName)
		}
		defaultStyle := widget.RichTextStyle{Inline: true, ColorName: theme.ColorNamePlaceHolder}
		highlightStyle := widget.RichTextStyle{Inline: true, ColorName: theme.ColorNamePrimary}
		ts := time.Now().Format("15:04:05")
		segments := []widget.RichTextSegment{&widget.TextSegment{Text: ts + " ", Style: defaultStyle}}
		if strings.HasPrefix(line, "[DEBUG]") {
			segments = append(segments, &widget.TextSegment{Text: line, Style: defaultStyle})
			segments = append(segments, &widget.TextSegment{Text: "\n", Style: defaultStyle})
			a.outputRich.Segments = append(a.outputRich.Segments, segments...)
			a.outputRich.Refresh()
			return
		}
		if strings.HasPrefix(line, "Monitoring:") {
			// Special handling: do not hyperlink file path
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				segments = append(segments, &widget.TextSegment{Text: parts[0] + ": ", Style: defaultStyle})
				segments = append(segments, &widget.TextSegment{Text: parts[1], Style: defaultStyle})
			} else {
				segments = append(segments, &widget.TextSegment{Text: line, Style: defaultStyle})
			}
			segments = append(segments, &widget.TextSegment{Text: "\n", Style: defaultStyle})
			a.outputRich.Segments = append(a.outputRich.Segments, segments...)
			a.outputRich.Refresh()
			return
		}
		words := strings.Fields(line)
		for i, w := range words {
			clean := strings.Trim(w, ",.?!;:'\"[]()")
			style := defaultStyle
			if clean == a.proc.PlayerName || (i > 0 && len(clean) > 0 && strings.ToUpper(string(clean[0])) == string(clean[0]) && clean[0] >= 'A' && clean[0] <= 'Z') {
				style = highlightStyle
				if clean != "" {
					if u, err := url.Parse(fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", clean)); err == nil {
						segments = append(segments, &widget.HyperlinkSegment{Text: w, URL: u})
					} else {
						segments = append(segments, &widget.TextSegment{Text: w, Style: style})
					}
				} else {
					segments = append(segments, &widget.TextSegment{Text: w, Style: style})
				}
			} else {
				segments = append(segments, &widget.TextSegment{Text: w, Style: style})
			}
			if i < len(words)-1 {
				segments = append(segments, &widget.TextSegment{Text: " ", Style: defaultStyle})
			}
		}
		segments = append(segments, &widget.TextSegment{Text: "\n", Style: defaultStyle})
		a.outputRich.Segments = append(a.outputRich.Segments, segments...)
		a.outputRich.Refresh()
		if strings.Contains(line, "CActor::Kill:") {
			parts := strings.Split(line, "'")
			if len(parts) >= 4 {
				victim := parts[1]
				killer := parts[3]
				a.showPopup(fmt.Sprintf("%s was killed by %s", victim, killer))
			}
		}
	})
	if a.onStatsUpdate != nil {
		a.onStatsUpdate(a.proc.PlayerName)
	}
}

// Run sets up and runs the UI with Feed, Statistics, and Config tabs.
func Run() {
	a := app.NewWithID("io.yourname.gamemonitor")
	window := a.NewWindow("Citizen Killstalker")
	iconBytes, err := os.ReadFile("icon.png")
	if err == nil {
		window.SetIcon(fyne.NewStaticResource("icon.png", iconBytes))
	}

	prefs := a.Preferences()
	saved := prefs.String("logPath")

	// Helper to get feed save directory
	getFeedDir := func() string {
		dir := filepath.Join(os.Getenv("APPDATA"), "citizenmon", "feeds")
		os.MkdirAll(dir, 0755)
		return dir
	}

	// UI components
	playerLabel := widget.NewLabel("<none>")
	outputRich := widget.NewRichText()
	outputRich.Truncation = fyne.TextTruncateClip

	// Separate RichText for history
	historyRich := widget.NewRichText()
	historyRich.Wrapping = fyne.TextWrapWord

	// Placeholders for stats lists
	kills := []struct {
		Name  string
		Count int
	}{}
	deaths := []struct {
		Name  string
		Count int
	}{}
	killList := widget.NewList(
		func() int { return len(kills) },
		func() fyne.CanvasObject {
			return widget.NewHyperlink("", nil)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(kills) {
				e := kills[i]
				url := fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", e.Name)
				o.(*widget.Hyperlink).SetText(fmt.Sprintf("%d. %s: %d", i+1, e.Name, e.Count))
				o.(*widget.Hyperlink).SetURLFromString(url)
			}
		},
	)
	deathList := widget.NewList(
		func() int { return len(deaths) },
		func() fyne.CanvasObject {
			return widget.NewHyperlink("", nil)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(deaths) {
				e := deaths[i]
				if e.Name == "Suicide" {
					o.(*widget.Hyperlink).SetText(fmt.Sprintf("%d. %s: %d", i+1, e.Name, e.Count))
					o.(*widget.Hyperlink).SetURL(nil)
				} else {
					url := fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", e.Name)
					o.(*widget.Hyperlink).SetText(fmt.Sprintf("%d. %s: %d", i+1, e.Name, e.Count))
					o.(*widget.Hyperlink).SetURLFromString(url)
				}
			}
		},
	)

	updateStats := func(playerName string) {
		fyne.Do(func() {
			statsData := stats.Load(playerName)
			kills = kills[:0]
			for n, c := range statsData.Kills {
				kills = append(kills, struct {
					Name  string
					Count int
				}{n, c})
			}
			sort.Slice(kills, func(i, j int) bool { return kills[i].Count > kills[j].Count })
			if len(kills) > 10 {
				kills = kills[:10]
			}
			killList.Refresh()
			deaths = deaths[:0]
			for n, c := range statsData.Deaths {
				deaths = append(deaths, struct {
					Name  string
					Count int
				}{n, c})
			}
			sort.Slice(deaths, func(i, j int) bool { return deaths[i].Count > deaths[j].Count })
			if len(deaths) > 10 {
				deaths = deaths[:10]
			}
			deathList.Refresh()
		})
	}

	// core and adapter
	core := processor.New(nil, playerLabel)
	h := &logHandlerAdapter{proc: core, outputRich: outputRich, window: window}
	h.onStatsUpdate = updateStats
	core.AppendOutput = h.AppendOutput

	// Config tab
	logEntry := widget.NewEntry()
	logEntry.SetPlaceHolder(`Path to your \\Roberts Space Industries\\StarCitizen\\LIVE\\game.log file`)
	if saved != "" {
		logEntry.SetText(saved)
	}
	browseBtn := widget.NewButton("Browse…", func() {
		dialog.ShowFileOpen(func(uri fyne.URIReadCloser, err error) {
			if uri != nil && err == nil {
				logEntry.SetText(uri.URI().Path())
			}
		}, window)
	})
	startBtn := widget.NewButton("Start Monitor", func() {
		path := logEntry.Text
		if _, err := os.Stat(path); err != nil {
			dialog.ShowError(fmt.Errorf("log file not found: %s", path), window)
			return
		}
		prefs.SetString("logPath", path)
		h.AppendOutput("Monitoring: " + path)
		go watcher.WatchLogFile(path, h)
	})

	clearLogsBtn := widget.NewButton("Clear All Old Logs", func() {
		dialog.ShowConfirm("Delete All Logs?", "Are you sure you want to delete all saved logs and statistics? This cannot be undone.", func(confirm bool) {
			if !confirm {
				return
			}
			feedDir := getFeedDir()
			entries, err := os.ReadDir(feedDir)
			if err == nil {
				for _, entry := range entries {
					if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".txt")) {
						os.Remove(filepath.Join(feedDir, entry.Name()))
					}
				}
			}
			// Also clear all stats files in the same directory
			statEntries, err := os.ReadDir(feedDir)
			if err == nil {
				for _, entry := range statEntries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_stats.json") {
						os.Remove(filepath.Join(feedDir, entry.Name()))
					}
				}
			}
			dialog.ShowInformation("Logs Cleared", "All logs and statistics have been deleted.", window)
		}, window)
	})

	configTab := container.NewTabItem("Config", container.NewVBox(
		widget.NewLabel("Log File Path:"),
		container.NewBorder(nil, nil, nil, browseBtn, logEntry),
		startBtn,
		clearLogsBtn,
	))

	// Feed tab
	scroll := container.NewScroll(outputRich)
	scroll.SetMinSize(fyne.NewSize(0, 400)) // Ensure scroll area is visible

	// Automatically scroll to bottom on new output
	autoScrollToBottom := func() {
		scroll.ScrollToBottom()
	}

	// Patch core.AppendOutput to auto-scroll
	core.AppendOutput = func(line string) {
		h.AppendOutput(line)
		autoScrollToBottom()
	}

	feedTab := container.NewTabItem("Feed", container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Current Player:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			playerLabel,
			widget.NewLabel("Feed:"),
		), nil, nil, nil, scroll))

	// Statistics tab
	killScroll := container.NewScroll(killList)
	deathScroll := container.NewScroll(deathList)
	killScroll.SetMinSize(fyne.NewSize(0, 400)) // Adjust 400 to your preferred min height
	deathScroll.SetMinSize(fyne.NewSize(0, 400))
	// Fix label typo in statistics tab
	statsTab := container.NewTabItem("Statistics", container.NewGridWithColumns(2,
		container.NewVBox(
			widget.NewLabelWithStyle("Top 10 Victims (You Killed):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			killScroll,
		),
		container.NewVBox(
			widget.NewLabelWithStyle("Top 10 Killers (Killed You):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			deathScroll,
		),
	))

	// --- FEED PERSISTENCE ---
	// Helper to get feed save directory
	getFeedDir = func() string {
		dir := filepath.Join(os.Getenv("APPDATA"), "citizenmon", "feeds")
		os.MkdirAll(dir, 0755)
		return dir
	}

	// Helper to generate feed filename
	getFeedFilename := func(playerName string) string {
		if playerName == "" {
			playerName = "Unknown"
		}
		// Sanitize playerName for filename: replace spaces with underscores
		playerName = strings.ReplaceAll(playerName, " ", "_")
		date := time.Now().Format("2006-01-02")
		base := filepath.Join(getFeedDir(), playerName+"_"+date)
		idx := 1
		filename := base + ".txt"
		for {
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				break
			}
			idx++
			filename = fmt.Sprintf("%s_%d.txt", base, idx)
		}
		return filename
	}

	// Save feed to file (JSON, grouped by line)
	saveFeed := func() {
		filename := getFeedFilename(core.PlayerName)
		jsonFile := filename[:len(filename)-4] + ".json"
		// Ensure we do not overwrite an existing file: increment suffix if needed
		base := jsonFile[:len(jsonFile)-5] // remove .json
		idx := 1
		finalFile := jsonFile
		for {
			if _, err := os.Stat(finalFile); os.IsNotExist(err) {
				break
			}
			idx++
			finalFile = fmt.Sprintf("%s_%d.json", base, idx)
		}
		f, err := os.Create(finalFile)
		if err != nil {
			return
		}
		defer f.Close()
		var lines [][]FeedSegment
		var currentLine []FeedSegment
		for _, seg := range outputRich.Segments {
			switch s := seg.(type) {
			case *widget.TextSegment:
				if s.Text == "\n" {
					currentLine = append(currentLine, FeedSegment{Type: "text", Text: "\n"})
					lines = append(lines, currentLine)
					currentLine = nil
				} else if strings.Contains(s.Text, "\n") {
					parts := strings.Split(s.Text, "\n")
					for i, part := range parts {
						if part != "" {
							currentLine = append(currentLine, FeedSegment{Type: "text", Text: part})
						}
						if i < len(parts)-1 {
							currentLine = append(currentLine, FeedSegment{Type: "text", Text: "\n"})
							lines = append(lines, currentLine)
							currentLine = nil
						}
					}
				} else {
					currentLine = append(currentLine, FeedSegment{Type: "text", Text: s.Text})
				}
			case *widget.HyperlinkSegment:
				currentLine = append(currentLine, FeedSegment{Type: "hyperlink", Text: s.Text, URL: s.URL.String()})
			}
		}
		// Do not flush currentLine if not ended with newline (to avoid trailing partial line)
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		_ = enc.Encode(lines)
	}

	// Save on window close
	window.SetCloseIntercept(func() {
		saveFeed()
		window.Close()
	})

	// --- FEED HISTORY TAB (DROPDOWN + EXPANDED VIEW) ---
	getFeedFiles := func() []string {
		dir := getFeedDir()
		files, _ := os.ReadDir(dir)
		var feedFiles []string
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				feedFiles = append(feedFiles, f.Name())
			}
		}
		return feedFiles
	}

	htmlEscape := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&#39;")
		return s
	}

	exportFeedToHTML := func(feedPath string, parent fyne.Window) {
		data, err := os.ReadFile(feedPath)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to read feed: %w", err), parent)
			return
		}
		html := "<html><head><meta charset='utf-8'><title>CitizenMon Feed Export</title></head><body><pre>" +
			htmlEscape(string(data)) + "</pre></body></html>"
		dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
			if uc == nil || err != nil {
				return
			}
			defer uc.Close()
			uc.Write([]byte(html))
		}, parent)
	}

	var selectedFeedPath string
	feedFiles := getFeedFiles()
	feedDropdown := widget.NewSelect(feedFiles, func(selected string) {
		if selected == "" {
			historyRich.Segments = []widget.RichTextSegment{}
			historyRich.Refresh()
			selectedFeedPath = ""
			return
		}
		selectedFeedPath = filepath.Join(getFeedDir(), selected)
		data, _ := os.ReadFile(selectedFeedPath)
		var linesData [][]FeedSegment
		_ = json.Unmarshal(data, &linesData)
		var segments []widget.RichTextSegment
		for _, line := range linesData {
			var lineSegments []widget.RichTextSegment
			var textBuffer strings.Builder
			for _, seg := range line {
				if seg.Type == "text" {
					if seg.Text == "\n" {
						if textBuffer.Len() > 0 {
							lineSegments = append(lineSegments, &widget.TextSegment{Text: textBuffer.String(), Style: widget.RichTextStyle{Inline: true}})
							textBuffer.Reset()
						}
						continue
					} else {
						textBuffer.WriteString(seg.Text)
					}
				} else if seg.Type == "hyperlink" {
					if textBuffer.Len() > 0 {
						lineSegments = append(lineSegments, &widget.TextSegment{Text: textBuffer.String(), Style: widget.RichTextStyle{Inline: true}})
						textBuffer.Reset()
					}
					u, _ := url.Parse(seg.URL)
					lineSegments = append(lineSegments, &widget.HyperlinkSegment{Text: seg.Text, URL: u})
				}
			}
			if textBuffer.Len() > 0 {
				lineSegments = append(lineSegments, &widget.TextSegment{Text: textBuffer.String(), Style: widget.RichTextStyle{Inline: true}})
				textBuffer.Reset()
			}
			lineSegments = append(lineSegments, &widget.TextSegment{Text: "\n", Style: widget.RichTextStyle{Inline: true}})
			segments = append(segments, lineSegments...)
		}
		historyRich.Segments = segments
		historyRich.Refresh()
	})

	if len(feedFiles) > 0 {
		feedDropdown.SetSelected(feedFiles[0])
		selectedFeedPath = filepath.Join(getFeedDir(), feedFiles[0])
		data, _ := os.ReadFile(selectedFeedPath)
		var linesData [][]FeedSegment
		_ = json.Unmarshal(data, &linesData)
		var segments []widget.RichTextSegment
		for _, line := range linesData {
			var lineSegments []widget.RichTextSegment
			var textBuffer strings.Builder
			for _, seg := range line {
				if seg.Type == "text" {
					if seg.Text == "\n" {
						if textBuffer.Len() > 0 {
							lineSegments = append(lineSegments, &widget.TextSegment{Text: textBuffer.String(), Style: widget.RichTextStyle{Inline: true}})
							textBuffer.Reset()
						}
						continue
					} else {
						textBuffer.WriteString(seg.Text)
					}
				} else if seg.Type == "hyperlink" {
					if textBuffer.Len() > 0 {
						lineSegments = append(lineSegments, &widget.TextSegment{Text: textBuffer.String(), Style: widget.RichTextStyle{Inline: true}})
						textBuffer.Reset()
					}
					u, _ := url.Parse(seg.URL)
					lineSegments = append(lineSegments, &widget.HyperlinkSegment{Text: seg.Text, URL: u})
				}
			}
			if textBuffer.Len() > 0 {
				lineSegments = append(lineSegments, &widget.TextSegment{Text: textBuffer.String(), Style: widget.RichTextStyle{Inline: true}})
				textBuffer.Reset()
			}
			lineSegments = append(lineSegments, &widget.TextSegment{Text: "\n", Style: widget.RichTextStyle{Inline: true}})
			segments = append(segments, lineSegments...)
		}
		historyRich.Segments = segments
		historyRich.Refresh()
	}

	historyTab := container.NewTabItem("History", container.NewVSplit(
		container.NewVBox(
			feedDropdown,
		),
		container.NewBorder(nil, nil, nil, nil, container.NewVScroll(historyRich)),
	))
	historyTab.Content = container.NewBorder(
		container.NewVBox(feedDropdown),
		widget.NewButton("Export as HTML", func() {
			if selectedFeedPath == "" {
				dialog.ShowInformation("No Feed Selected", "Please select a feed to export.", window)
				return
			}
			exportFeedToHTML(selectedFeedPath, window)
		}),
		nil, nil,
		container.NewVScroll(historyRich),
	)
	historyRich.ExtendBaseWidget(historyRich)
	historyRich.Resize(fyne.NewSize(0, 400))

	// assemble tabs
	tabs := container.NewAppTabs(
		feedTab,
		statsTab,
		configTab,
		historyTab,
	)

	// auto-start or config
	if saved != "" {
		h.AppendOutput("Monitoring: " + saved)
		go watcher.WatchLogFile(saved, h)
		tabs.Select(feedTab)
	} else {
		tabs.Select(configTab)
	}

	window.SetContent(tabs)
	window.Resize(fyne.NewSize(800, 600))
	window.ShowAndRun()
}

// Serializable struct for a segment (text or hyperlink)
type FeedSegment struct {
	Type string `json:"type"` // "text" or "hyperlink"
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

// Each log line is a slice of segments
// The feed is a slice of lines
