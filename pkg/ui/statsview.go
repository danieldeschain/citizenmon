package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// StatsView represents the UI component for displaying player statistics.
type StatsView struct {
	container *fyne.Container
}

// NewStatsView creates a new StatsView instance.
func NewStatsView() *StatsView {
	return &StatsView{
		container: container.NewVBox(widget.NewLabel("Stats")),
	}
}

// Update updates the stats view with new data.
func (s *StatsView) Update(data string) {
	// Implementation for updating stats view
}
