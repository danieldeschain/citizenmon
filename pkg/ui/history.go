package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// HistoryView represents the UI component for displaying historical log data.
type HistoryView struct {
	container *fyne.Container
}

// NewHistoryView creates a new HistoryView instance.
func NewHistoryView() *HistoryView {
	return &HistoryView{
		container: container.NewVBox(widget.NewLabel("History")),
	}
}

// Update updates the history view with new data.
func (h *HistoryView) Update(data string, logTime time.Time) {
	localTime := logTime.Local().Format("02.01.2006, 15:04 (MST)")
	formattedData := fmt.Sprintf("[%s] %s", localTime, data)
	// Append formattedData to the history view
	h.container.Add(widget.NewLabel(formattedData))
}
