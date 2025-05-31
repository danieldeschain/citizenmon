package ui

import (
	"fmt"
	"game-monitor/pkg/processor"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// logHandlerAdapter routes processed log events into the UI and uses native Fyne toasts.
type logHandlerAdapter struct {
	proc          *processor.Processor
	outputRich    *widget.RichText
	window        fyne.Window
	onStatsUpdate func(playerName string) // callback to update stats
	allSegments   []struct {
		segments   []widget.RichTextSegment
		rawLogLine string
	} // stores all lines with raw log line
}

// Helper to refresh outputRich based on ShowRawLogLines
func (a *logHandlerAdapter) refreshFeedDisplay() {
	// Debug: Print info about refresh
	fmt.Printf("RefreshFeedDisplay called: allSegments count: %d, ShowRawLogLines: %v\n", len(a.allSegments), ShowRawLogLines)

	// Create a completely new segments array
	displaySegments := make([]widget.RichTextSegment, 0)

	// Limit the number of displayed lines to prevent performance issues
	const maxDisplayLines = 1000
	startIdx := 0
	if len(a.allSegments) > maxDisplayLines {
		startIdx = len(a.allSegments) - maxDisplayLines
		fmt.Printf("Limiting display: showing last %d lines (from %d to %d)\n", maxDisplayLines, startIdx, len(a.allSegments))
	}

	for i := startIdx; i < len(a.allSegments); i++ {
		entry := a.allSegments[i]

		// Add the main message segments - make sure to copy each segment properly
		for _, seg := range entry.segments {
			displaySegments = append(displaySegments, seg)
		}
		// Add raw log line if enabled and available
		if ShowRawLogLines && entry.rawLogLine != "" {
			// Add a subtle separator before the raw log line
			displaySegments = append(displaySegments, &widget.TextSegment{
				Text:  "    ↳ Raw: ",
				Style: widget.RichTextStyle{Inline: true},
			})
			displaySegments = append(displaySegments, &widget.TextSegment{
				Text:  entry.rawLogLine,
				Style: widget.RichTextStyle{Inline: true},
			})
			displaySegments = append(displaySegments, &widget.TextSegment{
				Text:  "\n",
				Style: widget.RichTextStyle{Inline: true},
			})
		}
	}
	// Replace the segments completely and force a refresh
	a.outputRich.Segments = displaySegments
	a.outputRich.Refresh()

	fmt.Printf("RefreshFeedDisplay completed: Set %d segments in outputRich\n", len(displaySegments))
}
