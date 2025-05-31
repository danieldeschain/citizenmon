package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"game-monitor/pkg/processor"
	"game-monitor/pkg/stats"
	"game-monitor/pkg/watcher"
)

// Add global variable to control raw log display
var ShowRawLogLines = false

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
	// Remove truncation to prevent text from being cut off
	// outputRich.Truncation = fyne.TextTruncateClip
	// Enable text wrapping for outputRich to prevent long lines from breaking
	outputRich.Wrapping = fyne.TextWrapWord

	// Separate RichText for history
	historyRich := widget.NewRichText()
	historyRich.Wrapping = fyne.TextWrapWord
	// Placeholders for all-time stats lists
	allTimeKills := []struct {
		Name  string
		Count int
	}{}
	allTimeDeaths := []struct {
		Name  string
		Count int
	}{}
	
	// Placeholders for current session stats lists
	sessionKills := []struct {
		Name  string
		Count int
	}{}
	sessionDeaths := []struct {
		Name  string
		Count int
	}{}	// All-time stats lists with enhanced styling
	allTimeKillList := widget.NewList(
		func() int { return len(allTimeKills) },
		func() fyne.CanvasObject {
			return widget.NewHyperlink("", nil)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(allTimeKills) {
				e := allTimeKills[i]
				
				// Add medal emoji for top ranks
				medal := ""
				switch i {
				case 0: medal = "🥇 "
				case 1: medal = "🥈 "
				case 2: medal = "🥉 "
				default: medal = "🎯 "
				}
				
				url := fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", e.Name)
				o.(*widget.Hyperlink).SetText(fmt.Sprintf("%s#%d • %s (%d kills)", medal, i+1, e.Name, e.Count))
				o.(*widget.Hyperlink).SetURLFromString(url)
			}		},
	)
	allTimeDeathList := widget.NewList(
		func() int { return len(allTimeDeaths) },
		func() fyne.CanvasObject {
			return widget.NewHyperlink("", nil)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(allTimeDeaths) {
				e := allTimeDeaths[i]
				
				// Add skull emoji for top killers
				skull := ""
				switch i {
				case 0: skull = "💀 "
				case 1: skull = "☠️ "
				case 2: skull = "⚰️ "
				default: skull = "🔴 "
				}
				
				if e.Name == "Suicide" {
					o.(*widget.Hyperlink).SetText(fmt.Sprintf("%s#%d • %s (%d deaths)", skull, i+1, e.Name, e.Count))
					o.(*widget.Hyperlink).SetURL(nil)
				} else {
					url := fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", e.Name)
					o.(*widget.Hyperlink).SetText(fmt.Sprintf("%s#%d • %s (%d deaths)", skull, i+1, e.Name, e.Count))
					o.(*widget.Hyperlink).SetURLFromString(url)
				}
			}
		},
	)
	// Session stats lists with enhanced styling
	sessionKillList := widget.NewList(
		func() int { return len(sessionKills) },
		func() fyne.CanvasObject {
			return widget.NewHyperlink("", nil)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(sessionKills) {
				e := sessionKills[i]
				
				// Add lightning emoji for session stats
				lightning := ""
				switch i {
				case 0: lightning = "⚡ "
				case 1: lightning = "🔥 "
				case 2: lightning = "💥 "
				default: lightning = "🎯 "
				}
				
				url := fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", e.Name)
				o.(*widget.Hyperlink).SetText(fmt.Sprintf("%s#%d • %s (%d kills)", lightning, i+1, e.Name, e.Count))
				o.(*widget.Hyperlink).SetURLFromString(url)
			}		},
	)
	sessionDeathList := widget.NewList(
		func() int { return len(sessionDeaths) },
		func() fyne.CanvasObject {
			return widget.NewHyperlink("", nil)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(sessionDeaths) {
				e := sessionDeaths[i]
				
				// Add warning emoji for session deaths
				warning := ""
				switch i {
				case 0: warning = "⚠️ "
				case 1: warning = "🚨 "
				case 2: warning = "💀 "
				default: warning = "🔴 "
				}
				
				if e.Name == "Suicide" {
					o.(*widget.Hyperlink).SetText(fmt.Sprintf("%s#%d • %s (%d deaths)", warning, i+1, e.Name, e.Count))
					o.(*widget.Hyperlink).SetURL(nil)
				} else {
					url := fmt.Sprintf("https://robertsspaceindustries.com/en/citizens/%s", e.Name)
					o.(*widget.Hyperlink).SetText(fmt.Sprintf("%s#%d • %s (%d deaths)", warning, i+1, e.Name, e.Count))
					o.(*widget.Hyperlink).SetURLFromString(url)
				}
			}
		},
	)
	updateStats := func(playerName string) {
		fyne.Do(func() {
			// Load all-time stats
			allTimeStatsData := stats.Load(playerName)
			allTimeKills = allTimeKills[:0]
			for n, c := range allTimeStatsData.Kills {
				allTimeKills = append(allTimeKills, struct {
					Name  string
					Count int
				}{n, c})
			}
			sort.Slice(allTimeKills, func(i, j int) bool { return allTimeKills[i].Count > allTimeKills[j].Count })
			if len(allTimeKills) > 10 {
				allTimeKills = allTimeKills[:10]
			}
			allTimeKillList.Refresh()
			
			allTimeDeaths = allTimeDeaths[:0]
			for n, c := range allTimeStatsData.Deaths {
				allTimeDeaths = append(allTimeDeaths, struct {
					Name  string
					Count int
				}{n, c})
			}
			sort.Slice(allTimeDeaths, func(i, j int) bool { return allTimeDeaths[i].Count > allTimeDeaths[j].Count })
			if len(allTimeDeaths) > 10 {
				allTimeDeaths = allTimeDeaths[:10]
			}
			allTimeDeathList.Refresh()
			
			// Load current session stats
			sessionStatsData := stats.GetCurrentSession(playerName)
			sessionKills = sessionKills[:0]
			for n, c := range sessionStatsData.Kills {
				sessionKills = append(sessionKills, struct {
					Name  string
					Count int
				}{n, c})
			}
			sort.Slice(sessionKills, func(i, j int) bool { return sessionKills[i].Count > sessionKills[j].Count })
			if len(sessionKills) > 10 {
				sessionKills = sessionKills[:10]
			}
			sessionKillList.Refresh()
			
			sessionDeaths = sessionDeaths[:0]
			for n, c := range sessionStatsData.Deaths {
				sessionDeaths = append(sessionDeaths, struct {
					Name  string
					Count int
				}{n, c})
			}
			sort.Slice(sessionDeaths, func(i, j int) bool { return sessionDeaths[i].Count > sessionDeaths[j].Count })
			if len(sessionDeaths) > 10 {
				sessionDeaths = sessionDeaths[:10]
			}
			sessionDeathList.Refresh()
		})
	}// core and adapter
	core := processor.New(nil, playerLabel)
	h := &logHandlerAdapter{proc: core, outputRich: outputRich, window: window, allSegments: make([]struct {
		segments   []widget.RichTextSegment
		rawLogLine string
	}, 0)}
	h.onStatsUpdate = updateStats
	core.AppendOutput = func(line string, logTime ...time.Time) {
		// Update player label when player name is detected
		if core.PlayerName != "" && playerLabel != nil {
			fyne.Do(func() {
				playerLabel.SetText(core.PlayerName)
			})
		}

		// Prepend the local timestamp to the log line (convert UTC to local)
		if len(logTime) > 0 {
			localTime := logTime[0].Local()
			line = localTime.Format("2006-01-02 15:04:05") + " " + line
		}
		h.AppendOutputWithRaw(line, core.LastRawLogLine)
	}

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
		core.AppendOutput("Monitoring: " + path)
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
		clearLogsBtn)) // Feed tab
	// Single toggle button for raw logs
	var rawToggleBtn *widget.Button
	updateRawToggleBtn := func() {
		if ShowRawLogLines {
			rawToggleBtn.SetText("Disable Raw Logs")
		} else {
			rawToggleBtn.SetText("Enable Raw Logs")
		}
	}

	rawToggleBtn = widget.NewButton("Enable Raw Logs", func() {
		ShowRawLogLines = !ShowRawLogLines
		updateRawToggleBtn()
		h.refreshFeedDisplay()
	})
	scroll := container.NewScroll(outputRich)
	scroll.SetMinSize(fyne.NewSize(0, 400)) // Ensure scroll area is visible
	feedTab := container.NewTabItem("Feed", container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Current Player:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			playerLabel,
			widget.NewLabel("Feed:"),
			rawToggleBtn,
		), nil, nil, nil, scroll))
	// Statistics tab with All-time and Current sections
	allTimeKillScroll := container.NewScroll(allTimeKillList)
	allTimeDeathScroll := container.NewScroll(allTimeDeathList)
	allTimeKillScroll.SetMinSize(fyne.NewSize(0, 350))
	allTimeDeathScroll.SetMinSize(fyne.NewSize(0, 350))

	sessionKillScroll := container.NewScroll(sessionKillList)
	sessionDeathScroll := container.NewScroll(sessionDeathList)
	sessionKillScroll.SetMinSize(fyne.NewSize(0, 350))
	sessionDeathScroll.SetMinSize(fyne.NewSize(0, 350))	// Reset button for all-time stats
	resetButton := widget.NewButtonWithIcon("Reset All-time Stats", nil, func() {
		if playerLabel.Text == "<none>" {
			dialog.ShowInformation("No Player", "Please select a player first.", window)
			return
		}
		
		// Create custom confirmation dialog
		confirmLabel := widget.NewRichTextFromMarkdown("## Reset All-time Statistics\n\nAre you sure you want to reset all-time statistics for **" + playerLabel.Text + "**?\n\n*This action cannot be undone.*")
		
		yesBtn := widget.NewButtonWithIcon("Yes, Reset", nil, func() {})
		noBtn := widget.NewButtonWithIcon("No, Cancel", nil, func() {})
		
		yesBtn.Importance = widget.DangerImportance
		noBtn.Importance = widget.MediumImportance
		
		content := container.NewVBox(
			confirmLabel,
			container.NewBorder(nil, nil, nil, nil, 
				container.NewHBox(yesBtn, noBtn),
			),
		)
		
		confirmDialog := dialog.NewCustom("Confirm Reset", "Close", content, window)
		
		yesBtn.OnTapped = func() {
			confirmDialog.Hide()
			stats.ResetAllTime(playerLabel.Text)
			updateStats(playerLabel.Text)
			dialog.ShowInformation("Reset Complete", "All-time statistics have been reset.", window)
		}
		
		noBtn.OnTapped = func() {
			confirmDialog.Hide()
		}
		
		confirmDialog.Show()
	})
	resetButton.Importance = widget.HighImportance
	// All-time stats tab with enhanced styling
	allTimeKillCard := container.NewBorder(
		container.NewVBox(
			widget.NewCard("", "", container.NewVBox(
				widget.NewLabelWithStyle("🎯 Top 10 Victims (You Killed)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
			)),
		), nil, nil, nil, allTimeKillScroll)
	
	allTimeDeathCard := container.NewBorder(
		container.NewVBox(
			widget.NewCard("", "", container.NewVBox(
				widget.NewLabelWithStyle("💀 Top 10 Killers (Killed You)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
			)),
		), nil, nil, nil, allTimeDeathScroll)

	allTimeTab := container.NewTabItem("📊 All-time", container.NewVBox(
		widget.NewCard("All-Time Statistics", "Persistent stats saved across sessions", 
			container.NewGridWithColumns(2, allTimeKillCard, allTimeDeathCard)),
		container.NewBorder(nil, nil, nil, nil,
			container.NewHBox(
				widget.NewSeparator(),
				resetButton,
				widget.NewSeparator(),
			)),
	))
	// Current session stats tab with enhanced styling
	sessionKillCard := container.NewBorder(
		container.NewVBox(
			widget.NewCard("", "", container.NewVBox(
				widget.NewLabelWithStyle("🎯 Session Victims (You Killed)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
			)),
		), nil, nil, nil, sessionKillScroll)
	
	sessionDeathCard := container.NewBorder(
		container.NewVBox(
			widget.NewCard("", "", container.NewVBox(
				widget.NewLabelWithStyle("💀 Session Killers (Killed You)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
				widget.NewSeparator(),
			)),
		), nil, nil, nil, sessionDeathScroll)

	currentTab := container.NewTabItem("⚡ Current Session", 
		widget.NewCard("Current Session Statistics", "Stats reset when the app restarts",
			container.NewGridWithColumns(2, sessionKillCard, sessionDeathCard)))

	// Create nested tabs for statistics
	statsTabs := container.NewAppTabs(allTimeTab, currentTab)
	statsTab := container.NewTabItem("Statistics", statsTabs)

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
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") && !strings.HasSuffix(f.Name(), "_stats.json") {
				feedFiles = append(feedFiles, f.Name())
			}
		}
		// Sort newest first
		if len(feedFiles) > 1 {
			// Sort by file mod time descending
			sort.Slice(feedFiles, func(i, j int) bool {
				fi, _ := os.Stat(filepath.Join(getFeedDir(), feedFiles[i]))
				fj, _ := os.Stat(filepath.Join(getFeedDir(), feedFiles[j]))
				return fi.ModTime().After(fj.ModTime())
			})
		}
		return feedFiles
	}

	var feedFiles []string
	var selectedFeedPath string
	feedSelectEntry := widget.NewSelectEntry(nil)
	feedSelectEntry.SetPlaceHolder("Search or select log...")

	refreshFeedSelectEntry := func() {
		feedFiles = getFeedFiles()
		feedSelectEntry.SetOptions(feedFiles)
		if len(feedFiles) > 0 {
			feedSelectEntry.SetText(feedFiles[0])
		}
	}

	feedSelectEntry.OnChanged = func(selected string) {
		// Autocomplete: filter options as user types
		q := strings.ToLower(feedSelectEntry.Text)
		var filtered []string
		for _, f := range feedFiles {
			if strings.Contains(strings.ToLower(f), q) {
				filtered = append(filtered, f)
			}
		}
		feedSelectEntry.SetOptions(filtered)
		// Fyne workaround: no .Open(), so show a List below if filtering (simulate dropdown)
		// (Implementation: see below for a custom popup if needed)

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
	}

	refreshFeedSelectEntry()

	historyTab := container.NewTabItem("History", container.NewBorder(
		container.NewVBox(
			widget.NewButton("Open Log", func() {
				showLogBrowser(getFeedFiles, func(filename string) {
					selectedFeedPath = filepath.Join(getFeedDir(), filename)
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
			}),
			widget.NewButton("Convert Log", func() { convertLogToHistory(window) }),
		),
		widget.NewButton("Export as HTML", func() {
			if selectedFeedPath == "" {
				dialog.ShowInformation("No Feed Selected", "Please select a feed to export.", window)
				return
			}
			exportFeedToHTML(selectedFeedPath, window)
		}),
		nil, nil,
		container.NewVScroll(historyRich),
	))

	// assemble tabs
	tabs := container.NewAppTabs(
		feedTab,
		statsTab,
		configTab,
		historyTab,
	)
	// auto-start or config
	if saved != "" {
		// Ensure feed initializes with the game log and displays monitoring message
		core.AppendOutput("Monitoring: " + saved)
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

// HTML escape function for feed export
func htmlEscape(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(text, "&", "&amp;"), "<", "&lt;"), ">", "&gt;")
}

// Export feed to HTML file
func exportFeedToHTML(feedPath string, parent fyne.Window) {
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

// --- Convert Log to History ---
func convertLogToHistory(parent fyne.Window) {
	dialog.ShowFileOpen(func(uc fyne.URIReadCloser, err error) {
		if uc == nil || err != nil {
			return
		}
		defer uc.Close()
		logPath := uc.URI().Path()
		data, err := os.ReadFile(logPath)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to read log: %w", err), parent)
			return
		}
		lines := strings.Split(string(data), "\n")

		// Extract player name and date from log or filename
		playerName := "Unknown"
		logDate := time.Now().Format("2006-01-02")
		base := filepath.Base(logPath)
		// Try to extract date from filename (YYYY-MM-DD)
		for _, part := range strings.FieldsFunc(base, func(r rune) bool { return r == ' ' || r == '_' || r == '-' || r == '(' || r == ')' }) {
			if len(part) == 10 && part[4] == '-' && part[7] == '-' {
				logDate = part
				break
			}
		} // Try to find player name in log lines using the same detection logic as processor
		for _, line := range lines {
			// Look for nickname="PlayerName" pattern first
			if strings.Contains(line, "nickname=") {
				nicknameRegex := regexp.MustCompile(`nickname="([^"]+)"`)
				if matches := nicknameRegex.FindStringSubmatch(line); len(matches) > 1 {
					playerName = matches[1]
					break
				}
			}
			// Fallback: Look for Player[PlayerName] pattern
			if strings.Contains(line, "Player[") {
				playerRegex := regexp.MustCompile(`Player\[([^\]]+)\]`)
				if matches := playerRegex.FindStringSubmatch(line); len(matches) > 1 {
					playerName = matches[1]
					break
				}
			}
			// Legacy fallback
			if strings.Contains(line, "Player name:") {
				playerName = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				break
			}
		}
		// Only use filename extraction as a last resort if no player name found in log content
		if playerName == "Unknown" {
			// Try to get from filename (before first space or underscore)
			if idx := strings.IndexAny(base, " _"); idx > 0 {
				possibleName := base[:idx]
				// Only use filename if it doesn't look like a generic word
				if possibleName != "Game" && possibleName != "Log" && possibleName != "StarCitizen" {
					playerName = possibleName
				}
			}
		}
		playerName = strings.ReplaceAll(playerName, " ", "_")
		if playerName == "" {
			playerName = "Unknown"
		}
		// Remove debug dialog - directly proceed with conversion
		// Scan all lines from top to bottom for kill messages (not just via processor)
		var feed [][]FeedSegment
		// Temporary processor to parse the log
		proc := processor.New(nil, nil)
		// Set the processor's player name first
		proc.PlayerName = playerName
		// Updated to match the required signature with logTime parameter
		proc.AppendOutput = func(line string, logTime ...time.Time) {
			if line == "" || line == "PlayerName is empty, skipping stats update for line" {
				return
			}
			// Remove 'Player appeared' lines for the player character (robust, trims and matches underscores)
			if strings.HasPrefix(line, "Player appeared:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) > 1 {
					appearedName := strings.TrimSpace(parts[1])
					if strings.EqualFold(strings.ReplaceAll(appearedName, " ", "_"), strings.ReplaceAll(playerName, " ", "_")) {
						return
					}
				}
			}
			// Extract timestamp - use current time as fallback
			ts := time.Now().Format("2006-01-02 15:04:05")
			if len(logTime) > 0 && !logTime[0].IsZero() {
				ts = logTime[0].Local().Format("2006-01-02 15:04:05")
			}
			// Enhanced hyperlinking for kill/death/incap/corpse lines
			segments := CreateEnhancedSegments(line, ts, playerName)
			feed = append(feed, segments)
		}

		// Process all lines for kills/deaths/incaps/corpse
		for _, line := range lines {
			// Temporarily disable stats update in processor
			oldStats := proc.Stats
			proc.Stats = stats.New() // blank stats so no file is written
			proc.ProcessLogLine(line)
			proc.Stats = oldStats
		} // Save processed events without showing debug dialogs
		if len(feed) == 0 {
			feed = append(feed, []FeedSegment{
				{Type: "text", Text: fmt.Sprintf("%s No kill/death messages found in this log for player %s.\n", time.Now().Format("2006-01-02 15:04:05"), playerName)},
			})
		}

		// Save as .json in feeds dir, with Player_YYYY-MM-DD.json naming
		feedsDir := filepath.Join(os.Getenv("APPDATA"), "citizenmon", "feeds")
		os.MkdirAll(feedsDir, 0755)
		jsonName := playerName + "_" + logDate + ".json"
		jsonPath := filepath.Join(feedsDir, jsonName)
		idx := 1
		for {
			if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
				break
			}
			jsonPath = filepath.Join(feedsDir, fmt.Sprintf("%s_%d.json", playerName+"_"+logDate, idx))
			idx++
		}
		f, err := os.Create(jsonPath)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to save history: %w", err), parent)
			return
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		_ = enc.Encode(feed)
		dialog.ShowInformation("Converted", "Log converted to history: "+jsonPath, parent)
		if fyne.CurrentApp() != nil {
			for _, w := range fyne.CurrentApp().Driver().AllWindows() {
				if w.Title() == "Citizen Killstalker" {
					w.Content().Refresh()
				}
			}
		}
	}, parent)
}

// CreateEnhancedSegments creates segments with enhanced hyperlinking for log conversion
func CreateEnhancedSegments(line, timestamp, playerName string) []FeedSegment {
	var segments []FeedSegment
	segments = append(segments, FeedSegment{Type: "text", Text: timestamp + " "})

	// First, check if this is an already-processed message from the event aggregation system
	// These should not be re-processed through the enhanced hyperlinking system
	if strings.HasPrefix(line, "You were killed by: ") ||
		strings.HasPrefix(line, "You died by ") ||
		strings.HasPrefix(line, "You turned to a corpse") ||
		strings.HasPrefix(line, "Mission Event: ") ||
		strings.HasPrefix(line, "Vehicle ") && strings.Contains(line, " was destroyed by ") {
		// Handle as plain text without further processing
		segments = append(segments, FeedSegment{Type: "text", Text: line})
		segments = append(segments, FeedSegment{Type: "text", Text: "\n"})
		return segments
	}

	// Handle different types of processor output lines with specific patterns

	// 1. Corpse messages: "PlayerName has turned to a corpse"
	if strings.Contains(line, "has turned to a corpse") {
		parts := strings.SplitN(line, " has turned to a corpse", 2)
		if len(parts) > 0 {
			name := strings.TrimSpace(parts[0])
			if isNPCName(name) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(name)})
			} else if isPetName(name) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(name)})
			} else if shouldHyperlinkName(name) {
				segments = append(segments, FeedSegment{Type: "hyperlink", Text: name, URL: "https://robertsspaceindustries.com/en/citizens/" + name})
			} else {
				segments = append(segments, FeedSegment{Type: "text", Text: name})
			}
			segments = append(segments, FeedSegment{Type: "text", Text: " has turned to a corpse"})
			segments = append(segments, FeedSegment{Type: "text", Text: "\n"})
			return segments
		}
	}

	// 2. Kill messages: "You killed: PlayerName using weapon" or "You were killed by: PlayerName using weapon"
	if strings.Contains(line, "You killed:") || strings.Contains(line, "You were killed by:") || strings.Contains(line, "You incapacitated:") {
		return createKillMessageSegments(line, segments, playerName)
	}

	// 3. Vehicle destruction: "Vehicle Name was destroyed by PlayerName using weapon"
	if strings.Contains(line, "Vehicle") && (strings.Contains(line, "destroyed") || strings.Contains(line, "disabled")) {
		return createVehicleMessageSegments(line, segments)
	}

	// 4. Generic fallback for other lines - apply basic hyperlinking
	words := strings.Fields(line)
	byIdx := -1
	for i, w := range words {
		if strings.ToLower(w) == "by" && i < len(words)-1 {
			byIdx = i + 1
		}
	}

	for i, w := range words {
		clean := strings.Trim(w, ",.?!;:'\"[]()")
		shouldHyperlink := false

		// Hyperlink player names in specific contexts
		if len(clean) >= 3 {
			if i == byIdx || // After "by"
				strings.EqualFold(strings.ReplaceAll(clean, " ", "_"), strings.ReplaceAll(playerName, " ", "_")) { // Player's own name
				shouldHyperlink = shouldHyperlinkName(clean)
			}
		}

		if shouldHyperlink {
			segments = append(segments, FeedSegment{Type: "hyperlink", Text: w, URL: "https://robertsspaceindustries.com/en/citizens/" + clean})
		} else {
			// Apply NPC/pet formatting even for non-hyperlinked names
			displayText := w
			if isNPCName(clean) {
				displayText = strings.Replace(w, clean, formatNPCName(clean), 1)
			} else if isPetName(clean) {
				displayText = strings.Replace(w, clean, formatPetName(clean), 1)
			}
			segments = append(segments, FeedSegment{Type: "text", Text: displayText})
		}

		if i < len(words)-1 {
			segments = append(segments, FeedSegment{Type: "text", Text: " "})
		}
	}

	segments = append(segments, FeedSegment{Type: "text", Text: "\n"})
	return segments
}

// createKillMessageSegments handles kill/death/incap messages
func createKillMessageSegments(line string, baseSegments []FeedSegment, playerName string) []FeedSegment {
	segments := baseSegments

	// Parse different kill message patterns
	if strings.HasPrefix(line, "You killed:") {
		// "You killed: PlayerName using weapon"
		parts := strings.SplitN(line, "You killed:", 2)
		if len(parts) > 1 {
			remaining := strings.TrimSpace(parts[1])
			usingIdx := strings.Index(remaining, " using ")

			segments = append(segments, FeedSegment{Type: "text", Text: "You killed: "})

			if usingIdx > 0 {
				// Has weapon info
				victim := strings.TrimSpace(remaining[:usingIdx])
				weapon := strings.TrimSpace(remaining[usingIdx+7:])

				// Apply enhanced formatting for NPCs and pets
				if isNPCName(victim) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(victim)})
				} else if isPetName(victim) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(victim)})
				} else if shouldHyperlinkName(victim) {
					segments = append(segments, FeedSegment{Type: "hyperlink", Text: victim, URL: "https://robertsspaceindustries.com/en/citizens/" + victim})
				} else {
					segments = append(segments, FeedSegment{Type: "text", Text: victim})
				}
				segments = append(segments, FeedSegment{Type: "text", Text: " using " + weapon})
			} else {
				// No weapon info
				victim := strings.TrimSpace(remaining)
				if isNPCName(victim) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(victim)})
				} else if isPetName(victim) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(victim)})
				} else if shouldHyperlinkName(victim) {
					segments = append(segments, FeedSegment{Type: "hyperlink", Text: victim, URL: "https://robertsspaceindustries.com/en/citizens/" + victim})
				} else {
					segments = append(segments, FeedSegment{Type: "text", Text: victim})
				}
			}
		}
	} else if strings.HasPrefix(line, "You were killed by:") {
		// "You were killed by: PlayerName using weapon"
		parts := strings.SplitN(line, "You were killed by:", 2)
		if len(parts) > 1 {
			remaining := strings.TrimSpace(parts[1])
			usingIdx := strings.Index(remaining, " using ")

			segments = append(segments, FeedSegment{Type: "text", Text: "You were killed by: "})

			if usingIdx > 0 {
				// Has weapon info
				killer := strings.TrimSpace(remaining[:usingIdx])
				weapon := strings.TrimSpace(remaining[usingIdx+7:])

				// Apply enhanced formatting for NPCs, pets, and suicide
				if strings.ToLower(killer) == "suicide" {
					segments = append(segments, FeedSegment{Type: "text", Text: killer})
				} else if isNPCName(killer) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(killer)})
				} else if isPetName(killer) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(killer)})
				} else if shouldHyperlinkName(killer) {
					segments = append(segments, FeedSegment{Type: "hyperlink", Text: killer, URL: "https://robertsspaceindustries.com/en/citizens/" + killer})
				} else {
					segments = append(segments, FeedSegment{Type: "text", Text: killer})
				}
				segments = append(segments, FeedSegment{Type: "text", Text: " using " + weapon})
			} else {
				// No weapon info
				killer := strings.TrimSpace(remaining)
				if strings.ToLower(killer) == "suicide" {
					segments = append(segments, FeedSegment{Type: "text", Text: killer})
				} else if isNPCName(killer) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(killer)})
				} else if isPetName(killer) {
					segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(killer)})
				} else if shouldHyperlinkName(killer) {
					segments = append(segments, FeedSegment{Type: "hyperlink", Text: killer, URL: "https://robertsspaceindustries.com/en/citizens/" + killer})
				} else {
					segments = append(segments, FeedSegment{Type: "text", Text: killer})
				}
			}
		}
	} else if strings.HasPrefix(line, "You incapacitated:") {
		// "You incapacitated: PlayerName"
		parts := strings.SplitN(line, "You incapacitated:", 2)
		if len(parts) > 1 {
			victim := strings.TrimSpace(parts[1])
			segments = append(segments, FeedSegment{Type: "text", Text: "You incapacitated: "})

			if isNPCName(victim) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(victim)})
			} else if isPetName(victim) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(victim)})
			} else if shouldHyperlinkName(victim) {
				segments = append(segments, FeedSegment{Type: "hyperlink", Text: victim, URL: "https://robertsspaceindustries.com/en/citizens/" + victim})
			} else {
				segments = append(segments, FeedSegment{Type: "text", Text: victim})
			}
		}
	}

	segments = append(segments, FeedSegment{Type: "text", Text: "\n"})
	return segments
}

// createVehicleMessageSegments handles vehicle destruction messages
func createVehicleMessageSegments(line string, baseSegments []FeedSegment) []FeedSegment {
	segments := baseSegments

	// Parse vehicle destruction: "Vehicle Name was destroyed by PlayerName using weapon"
	byIdx := strings.Index(line, " by ")
	usingIdx := strings.Index(line, " using ")

	if byIdx > 0 {
		beforeBy := line[:byIdx]
		afterBy := line[byIdx+4:]

		segments = append(segments, FeedSegment{Type: "text", Text: beforeBy + " by "})

		if usingIdx > byIdx {
			// Has weapon info
			killer := strings.TrimSpace(afterBy[:usingIdx-byIdx-4])
			weapon := strings.TrimSpace(afterBy[usingIdx-byIdx-4+7:])

			// Apply enhanced formatting for NPCs, pets, and suicide
			if strings.ToLower(killer) == "suicide" {
				segments = append(segments, FeedSegment{Type: "text", Text: killer})
			} else if isNPCName(killer) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(killer)})
			} else if isPetName(killer) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(killer)})
			} else if shouldHyperlinkName(killer) {
				segments = append(segments, FeedSegment{Type: "hyperlink", Text: killer, URL: "https://robertsspaceindustries.com/en/citizens/" + killer})
			} else {
				segments = append(segments, FeedSegment{Type: "text", Text: killer})
			}
			segments = append(segments, FeedSegment{Type: "text", Text: " using " + weapon})
		} else {
			// No weapon info or collision
			killer := strings.TrimSpace(afterBy)
			if strings.ToLower(killer) == "suicide" {
				segments = append(segments, FeedSegment{Type: "text", Text: killer})
			} else if isNPCName(killer) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatNPCName(killer)})
			} else if isPetName(killer) {
				segments = append(segments, FeedSegment{Type: "text", Text: formatPetName(killer)})
			} else if shouldHyperlinkName(killer) {
				segments = append(segments, FeedSegment{Type: "hyperlink", Text: killer, URL: "https://robertsspaceindustries.com/en/citizens/" + killer})
			} else {
				segments = append(segments, FeedSegment{Type: "text", Text: killer})
			}
		}
	} else {
		// Fallback: just add as text
		segments = append(segments, FeedSegment{Type: "text", Text: line})
	}

	segments = append(segments, FeedSegment{Type: "text", Text: "\n"})
	return segments
}

// --- LOG BROWSER WINDOW ---
func showLogBrowser(getFeedFiles func() []string, onSelect func(filename string)) {
	logs := getFeedFiles()
	filtered := make([]string, len(logs))
	copy(filtered, logs)

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search logs...")

	list := widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i int, o fyne.CanvasObject) {
			if i < len(filtered) {
				o.(*widget.Label).SetText(filtered[i])
			}
		},
	)

	var browserWin fyne.Window
	list.OnSelected = func(id int) {
		if id >= 0 && id < len(filtered) {
			onSelect(filtered[id])
			browserWin.Close()
		}
	}

	searchEntry.OnChanged = func(s string) {
		q := strings.ToLower(s)
		filtered = filtered[:0]
		for _, f := range logs {
			if strings.Contains(strings.ToLower(f), q) {
				filtered = append(filtered, f)
			}
		}
		list.Refresh()
	}

	browserWin = fyne.CurrentApp().NewWindow("Open Log")
	browserWin.SetContent(container.NewBorder(
		searchEntry, nil, nil, nil,
		container.NewVScroll(list),
	))
	browserWin.Resize(fyne.NewSize(400, 500))
	browserWin.Show()
}

// Added missing methods to logHandlerAdapter to implement watcher.LogHandler
func (a *logHandlerAdapter) AppendOutput(line string) {
	a.AppendOutputWithRaw(line, "")
}

func (a *logHandlerAdapter) AppendOutputWithRaw(line string, rawLogLine string) {
	fyne.Do(func() {
		fmt.Printf("AppendOutputWithRaw called with: '%s' (raw: '%s')\n", line, rawLogLine)

		// Create segments for this line with improved hyperlink logic
		var segments []widget.RichTextSegment
		// Enhanced player name detection for hyperlinks
		words := strings.Fields(line)
		
		// Find "by" index for context-aware hyperlinking
		byIdx := -1
		for i, w := range words {
			if strings.ToLower(w) == "by" && i < len(words)-1 {
				byIdx = i + 1
			}
		}
				for i, word := range words {
			clean := strings.Trim(word, ",.?!;:'\"[]()")
			shouldCreateHyperlink := false
			displayText := word

			// Enhanced hyperlinking logic - handle timestamped messages properly
			if len(clean) >= 3 {
				// Check various contexts where player names appear
				// For kill messages, look for position after "killed:" word
				killedIdx := -1
				incapIdx := -1
				for j, w := range words {
					if strings.Contains(w, "killed:") {
						killedIdx = j + 1
					}
					if strings.Contains(w, "incapacitated:") {
						incapIdx = j + 1
					}
				}

				if i == byIdx || // After "by"
					i == killedIdx || // After "killed:"
					i == incapIdx || // After "incapacitated:"
					(strings.Contains(line, "corpse") && !strings.HasPrefix(line, "You")) || // In corpse messages (but not "You" messages)
					(strings.Contains(line, "died") && i > 0 && strings.ToLower(words[i-1]) == "by") { // Deaths by player
					shouldCreateHyperlink = shouldHyperlinkName(clean)
				}
			}

			// Apply NPC/pet formatting even for non-hyperlinked names
			if isNPCName(clean) {
				displayText = strings.Replace(word, clean, formatNPCName(clean), 1)
			} else if isPetName(clean) {
				displayText = strings.Replace(word, clean, formatPetName(clean), 1)
			}

			if shouldCreateHyperlink {
				segments = append(segments, &widget.HyperlinkSegment{
					Text: displayText,
					URL:  parseURL("https://robertsspaceindustries.com/en/citizens/" + clean),
				})
			} else {
				segments = append(segments, &widget.TextSegment{
					Text:  displayText,
					Style: widget.RichTextStyle{Inline: true},
				})
			}

			if i < len(words)-1 {
				segments = append(segments, &widget.TextSegment{
					Text:  " ",
					Style: widget.RichTextStyle{Inline: true},
				})
			}
		}

		// Add newline
		segments = append(segments, &widget.TextSegment{
			Text:  "\n",
			Style: widget.RichTextStyle{Inline: true},
		}) // Store in allSegments with raw log line
		a.allSegments = append(a.allSegments, struct {
			segments   []widget.RichTextSegment
			rawLogLine string
		}{segments, rawLogLine})

		fmt.Printf("Stored message in allSegments. Total count now: %d\n", len(a.allSegments))

		// Directly append to RichText widget instead of calling refreshFeedDisplay
		// This avoids performance issues and UI conflicts
		a.outputRich.Segments = append(a.outputRich.Segments, segments...)

		// If raw logs are enabled, add the raw log line
		if ShowRawLogLines && rawLogLine != "" {
			rawSegment := &widget.TextSegment{
				Text:  "↳ Raw: " + rawLogLine + "\n",
				Style: widget.RichTextStyle{Inline: true},
			}
			a.outputRich.Segments = append(a.outputRich.Segments, rawSegment)
		}

		// Refresh the widget to show new content
		a.outputRich.Refresh()
		fmt.Printf("Directly appended segments to outputRich. Total segments now: %d\n", len(a.outputRich.Segments))

		// Trigger stats update if we have a player name
		if a.proc.PlayerName != "" && a.onStatsUpdate != nil {
			a.onStatsUpdate(a.proc.PlayerName)
		}
	})
}

// DetectPlayerName method for logHandlerAdapter
func (a *logHandlerAdapter) DetectPlayerName(line string) {
	a.proc.DetectPlayerName(line)
}

// ProcessLogLine method for logHandlerAdapter
func (a *logHandlerAdapter) ProcessLogLine(line string) {
	a.proc.ProcessLogLine(line)
}

// Helper function to parse URL safely
func parseURL(urlStr string) *url.URL {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	return u
}

// Helper function to check if a string looks like a valid player name
func isValidPlayerName(name string) bool {
	// Player names are typically alphanumeric with underscores, 3+ characters
	if len(name) < 3 || len(name) > 30 {
		return false
	}

	// Check for valid player name characters (letters, numbers, underscores)
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}

	// Avoid common non-player words and common English words
	lowerName := strings.ToLower(name)
	commonWords := []string{
		"system", "server", "admin", "you", "killed", "using", "with", "the", "and",
		"or", "by", "from", "to", "at", "in", "on", "for", "was", "were", "has",
		"have", "had", "been", "being", "are", "is", "am", "will", "would", "could",
		"should", "may", "might", "can", "cannot", "turned", "corpse", "incapacitated",
	}

	for _, word := range commonWords {
		if lowerName == word {
			return false
		}
	}
	// Avoid common non-player words with contains check
	if strings.Contains(lowerName, "system") ||
		strings.Contains(lowerName, "server") ||
		strings.Contains(lowerName, "admin") {
		return false
	}

	// Don't consider NPCs as valid player names
	if isNPCName(name) {
		return false
	}

	// Don't consider pets as valid player names
	if isPetName(name) {
		return false
	}

	// Don't consider system names as valid player names
	if isSystemName(name) {
		return false
	}

	return true
}

// IsValidPlayerName - exported version for testing
func IsValidPlayerName(name string) bool {
	return isValidPlayerName(name)
}

// Helper function to detect and format NPC names
func isNPCName(name string) bool {
	return strings.Contains(name, "PU_Human_Enemy_GroundCombat_NPC") ||
		strings.Contains(name, "_NPC_") ||
		strings.Contains(name, "NPC_")
}

// IsNPCName - exported version for testing
func IsNPCName(name string) bool {
	return isNPCName(name)
}

// Helper function to detect and format pet names
func isPetName(name string) bool {
	return strings.Contains(strings.ToLower(name), "_pet_") ||
		strings.HasPrefix(name, "Pet_")
}

// IsPetName - exported version for testing
func IsPetName(name string) bool {
	return isPetName(name)
}

// Helper function to format NPC names (shorten to "NPC")
func formatNPCName(name string) string {
	if isNPCName(name) {
		return "NPC"
	}
	return name
}

// FormatNPCName - exported version for testing
func FormatNPCName(name string) string {
	return formatNPCName(name)
}

// Helper function to format pet names (extract first part before underscore)
func formatPetName(name string) string {
	if isPetName(name) {
		// Handle Pet_ prefix format
		if strings.HasPrefix(name, "Pet_") {
			parts := strings.Split(name, "_")
			if len(parts) >= 2 {
				return "NPC " + parts[1] // Get the part after Pet_
			}
		}
		// Handle _pet_ format (e.g., Kopion_pet_123)
		if strings.Contains(strings.ToLower(name), "_pet_") {
			parts := strings.Split(name, "_")
			if len(parts) > 0 {
				return "NPC " + parts[0] // Get the first part
			}
		}
	}
	return name
}

// FormatPetName - exported version for testing
func FormatPetName(name string) string {
	return formatPetName(name)
}

// Helper function to check if a name should be hyperlinked
func shouldHyperlinkName(name string) bool {
	// Don't hyperlink suicide
	if strings.ToLower(name) == "suicide" {
		return false
	}

	// Don't hyperlink "unknown"
	if strings.ToLower(name) == "unknown" {
		return false
	}

	// Don't hyperlink if it's "SELF"
	if strings.ToUpper(name) == "SELF" {
		return false // SELF should not be hyperlinked for suicide cases
	}

	// Don't hyperlink NPC names
	if isNPCName(name) {
		return false
	}

	// Don't hyperlink pet names
	if isPetName(name) {
		return false // Pets should not be hyperlinked
	}

	// Only hyperlink if it's a valid player name
	return isValidPlayerName(name)
}

// ShouldHyperlinkName - exported version for testing
func ShouldHyperlinkName(name string) bool {
	return shouldHyperlinkName(name)
}

// Helper function to check if a name is a system/weapon/vehicle name
func isSystemName(name string) bool {
	systemNames := []string{
		"collision", "fall", "suicide", "system", "server", "admin",
		"ballistic", "energy", "missile", "torpedo", "cannon", "rifle",
		"pistol", "shotgun", "sniper", "launcher", "turret", "shield",
		"armor", "helmet", "suit", "vehicle", "ship", "quantum", "jump",
		"unknown", // Add unknown as a system name too
	}

	lowerName := strings.ToLower(name)
	for _, sys := range systemNames {
		if strings.Contains(lowerName, sys) {
			return true
		}
	}

	// Check for NPC names
	if isNPCName(name) {
		return true
	}

	// Check for pet names
	if isPetName(name) {
		return true
	}

	return false
}
