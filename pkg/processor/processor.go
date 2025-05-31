package processor

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"game-monitor/pkg/stats"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

var (
	corpseRegex  = regexp.MustCompile(`\bCorpse\b`)
	vehicleRegex = regexp.MustCompile(
		`CVehicle::OnAdvanceDestroyLevel: Vehicle '([^']+)' .*advanced from destroy level ([0-9]+) to ([0-9]+) caused by '([^']+)' .*with '([^']+)'`,
	)
)

// cleanName removes numeric suffixes and replaces underscores with spaces.
func cleanName(name string) string {
	reNum := regexp.MustCompile(`_[0-9]+$`)
	clean := reNum.ReplaceAllString(name, "")
	return strings.ReplaceAll(clean, "_", " ")
}

// Processor holds state needed to parse and display log info.
type Processor struct {
	PlayerName      string
	Stats           stats.Stats // All-time stats (persisted to file)
	SessionStats    stats.Stats // Current session stats (reset on app restart)
	OutputBox       *widget.Entry
	PlayerLabel     *widget.Label
	AppendOutput    func(line string, logTime ...time.Time) // logTime is optional, for UI to use
	LastRawLogLine  string                                  // NEW: holds the last raw log line processed
	EventAggregator *EventAggregator                        // NEW: aggregates related events into mission summaries
}

// New creates a Processor bound to the given output entry and label.
func New(output *widget.Entry, label *widget.Label) *Processor {
	p := &Processor{
		Stats:           stats.New(),
		SessionStats:    stats.New(), // Initialize current session stats
		OutputBox:       output,
		PlayerLabel:     label,
		EventAggregator: NewEventAggregator(),
	} // default AppendOutput updates the UI entry on main thread
	p.AppendOutput = func(line string, logTime ...time.Time) {
		ts := ""
		if len(logTime) > 0 {
			// Convert UTC timestamp to local timezone
			localTime := logTime[0].Local()
			ts = localTime.Format("2006-01-02 15:04:05") + " "
		} else {
			ts = time.Now().Format("2006-01-02 15:04:05") + " "
		}
		fyne.Do(func() {
			if p.PlayerLabel != nil && p.PlayerName != "" {
				p.PlayerLabel.SetText(p.PlayerName)
			}
			if p.OutputBox != nil {
				p.OutputBox.SetText(p.OutputBox.Text + ts + line + "\n")
			}
		})
	}
	return p
}

// DetectPlayerName scans a line to set p.PlayerName once.
func (p *Processor) DetectPlayerName(line string) {
	if p.PlayerName != "" {
		return
	}

	// Extract timestamp from the current line for consistent timestamping
	logTime, hasTime := ExtractLogTimestamp(line)

	// Look for nickname="PlayerName" pattern in network messages
	if strings.Contains(line, "nickname=") {
		// Extract nickname using regex for better accuracy
		nicknameRegex := regexp.MustCompile(`nickname="([^"]+)"`)
		if matches := nicknameRegex.FindStringSubmatch(line); len(matches) > 1 {
			p.PlayerName = matches[1]
			if hasTime {
				p.AppendOutput("Detected player name: "+p.PlayerName, logTime)
			} else {
				p.AppendOutput("Detected player name: " + p.PlayerName)
			}
			p.Stats = stats.Load(p.PlayerName)
			return
		}
	}

	// Fallback: Look for Player[PlayerName] pattern in inventory/other messages
	if strings.Contains(line, "Player[") {
		playerRegex := regexp.MustCompile(`Player\[([^\]]+)\]`)
		if matches := playerRegex.FindStringSubmatch(line); len(matches) > 1 {
			p.PlayerName = matches[1]
			if hasTime {
				p.AppendOutput("Detected player name: "+p.PlayerName, logTime)
			} else {
				p.AppendOutput("Detected player name: " + p.PlayerName)
			}
			p.Stats = stats.Load(p.PlayerName)
			return
		}
	}

	// Legacy fallback for older log formats
	if strings.Contains(line, "Character:") && strings.Contains(line, "name") {
		parts := strings.Fields(line)
		for i, tok := range parts {
			if tok == "name" && i+1 < len(parts) {
				p.PlayerName = strings.Trim(parts[i+1], "-:[]{}\\\",'")
				if hasTime {
					p.AppendOutput("Detected player name: "+p.PlayerName, logTime)
				} else {
					p.AppendOutput("Detected player name: " + p.PlayerName)
				}
				p.Stats = stats.Load(p.PlayerName)
				return
			}
		}
	}
}

// Helper to extract UTC timestamp from a log line and convert to local time
func ExtractLogTimestamp(line string) (time.Time, bool) {
	// Look for timestamp pattern <YYYY-MM-DDTHH:MM:SS.sssZ>
	if idx1 := strings.Index(line, "<"); idx1 != -1 {
		if idx2 := strings.Index(line[idx1:], ">"); idx2 != -1 {
			timestamp := line[idx1+1 : idx1+idx2]
			if t, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
				return t, true
			}
		}
	}

	// Fallback: look in individual fields
	fields := strings.Fields(line)
	for _, f := range fields {
		if len(f) >= 20 && (strings.HasSuffix(f, "Z") || strings.HasSuffix(f, "+00:00")) {
			if t, err := time.Parse(time.RFC3339Nano, f); err == nil {
				return t, true
			}
		}
		if !strings.Contains(f, "-") && !strings.Contains(f, ":") {
			break
		}
	}
	return time.Time{}, false
}

// ProcessLogLine updates stats based on a single log line.
func (p *Processor) ProcessLogLine(line string) {
	p.LastRawLogLine = line // NEW: always set the last raw log line
	logTime, hasTime := ExtractLogTimestamp(line)

	if !hasTime {
		logTime = time.Now()
	}

	// If player name not detected yet, just return without processing events
	if p.PlayerName == "" {
		return
	}

	// First, flush old events that are beyond the aggregation window
	oldMessages := p.EventAggregator.FlushOldEvents(logTime, p)
	for _, msg := range oldMessages {
		p.AppendOutput(msg, logTime)
	}

	var eventDetected bool

	// Vehicle destruction
	if strings.Contains(line, "CVehicle::OnAdvanceDestroyLevel") {
		if m := vehicleRegex.FindStringSubmatch(line); len(m) == 6 {
			fullID := m[1]
			toLevel := m[3]
			causeRaw := m[4]
			weaponRaw := m[5]

			// Add to event aggregator
			event := PendingEvent{
				Type:        EventVehicleDestruction,
				Timestamp:   logTime,
				PlayerName:  p.PlayerName,
				VehicleName: fullID,
				Cause:       causeRaw,
				Weapon:      weaponRaw,
				RawLine:     line,
				Details:     map[string]string{"destroyLevel": toLevel},
			}
			p.EventAggregator.AddEvent(event)
			eventDetected = true
		}
	}
	// Player deaths and kills
	if strings.Contains(line, "CActor::Kill:") {		// suicide
		suicidePattern := fmt.Sprintf(`CActor::Kill: '%s'.*killed by '%s'`, regexp.QuoteMeta(p.PlayerName), regexp.QuoteMeta(p.PlayerName))
		suicideRe := regexp.MustCompile(suicidePattern)
		if suicideRe.MatchString(line) {
			p.Stats.Deaths["Suicide"]++
			p.SessionStats.Deaths["Suicide"]++
			stats.Save(p.PlayerName, p.Stats)
			stats.UpdateCurrentSession(p.PlayerName, p.SessionStats)

			// Add to event aggregator
			event := PendingEvent{
				Type:       EventPlayerDeath,
				Timestamp:  logTime,
				PlayerName: p.PlayerName,
				Cause:      "suicide",
				Weapon:     "suicide",
				RawLine:    line,
			}
			p.EventAggregator.AddEvent(event)
			eventDetected = true
		} else {			// Check if this player died
			rDeath := regexp.MustCompile(`CActor::Kill: '` + regexp.QuoteMeta(p.PlayerName) + `'.*killed by '([^']+)'(?:.*using '([^']+)')?(?:.*with damage type '([^']+)')?`)
			if m := rDeath.FindStringSubmatch(line); len(m) > 1 {
				killer := m[1]
				weapon := ""
				damageType := ""
				if len(m) >= 3 && m[2] != "" {
					weapon = m[2]
				}
				if len(m) >= 4 && m[3] != "" {
					damageType = m[3]
				}

				p.Stats.Deaths[killer]++
				p.SessionStats.Deaths[killer]++
				stats.Save(p.PlayerName, p.Stats)
				stats.UpdateCurrentSession(p.PlayerName, p.SessionStats)

				// Add to event aggregator
				event := PendingEvent{
					Type:       EventPlayerDeath,
					Timestamp:  logTime,
					PlayerName: p.PlayerName,
					Cause:      killer,
					Weapon:     weapon,
					RawLine:    line,
					Details:    map[string]string{"damageType": damageType},
				}
				p.EventAggregator.AddEvent(event)
				eventDetected = true
			} else {				// kill by player with method
				rMethod := regexp.MustCompile(`CActor::Kill: '([A-Za-z0-9_]+)'.*killed by '` + regexp.QuoteMeta(p.PlayerName) + `'.*using '([^']+)'`)
				if m := rMethod.FindStringSubmatch(line); len(m) == 3 {
					victim := m[1]
					method := cleanName(m[2])
					p.Stats.Kills[victim]++
					p.SessionStats.Kills[victim]++
					stats.Save(p.PlayerName, p.Stats)
					stats.UpdateCurrentSession(p.PlayerName, p.SessionStats)
					p.AppendOutput(fmt.Sprintf("You killed: %s using %s", victim, method), logTime)
					return
				}
				// fallback kill by player
				rKill := regexp.MustCompile(`CActor::Kill: '([A-Za-z0-9_]+)'.*killed by '` + regexp.QuoteMeta(p.PlayerName) + `'`)
				if m := rKill.FindStringSubmatch(line); len(m) > 1 {
					victim := m[1]
					p.Stats.Kills[victim]++
					p.SessionStats.Kills[victim]++
					stats.Save(p.PlayerName, p.Stats)
					stats.UpdateCurrentSession(p.PlayerName, p.SessionStats)
					p.AppendOutput("You killed: "+victim, logTime)
					return
				}
			}
		}
	}
	// Actor state changes (corpse)
	if corpseRegex.MatchString(line) || strings.Contains(line, "Entering control state") {
		// Try to extract the player name from the line
		if idx := strings.Index(line, "Player '"); idx != -1 {
			endIdx := strings.Index(line[idx+8:], "'")
			if endIdx != -1 {
				extracted := line[idx+8 : idx+8+endIdx]
				if extracted != "" && extracted == p.PlayerName {
					// Add to event aggregator for player state changes
					event := PendingEvent{
						Type:       EventActorState,
						Timestamp:  logTime,
						PlayerName: p.PlayerName,
						Cause:      "corpse",
						RawLine:    line,
					}
					p.EventAggregator.AddEvent(event)
					eventDetected = true
				}
			}
		}
	}
	// Incapacitations (not aggregated, output immediately)
	if strings.Contains(line, "Logged an incap") {
		r := regexp.MustCompile(`nickname: ([A-Za-z0-9_]+)`)
		if m := r.FindStringSubmatch(line); len(m) > 1 && m[1] != p.PlayerName {
			target := m[1]
			p.Stats.Incaps[target]++
			p.SessionStats.Incaps[target]++
			stats.Save(p.PlayerName, p.Stats)
			stats.UpdateCurrentSession(p.PlayerName, p.SessionStats)
			p.AppendOutput("You incapacitated: "+target, logTime)
			return
		}
	}
	// If we detected an event that should be aggregated, don't try to create a summary yet
	// Let events accumulate in the aggregator
	if eventDetected {
		// Only try to create summaries when we flush old events or when forced
		// This allows multiple related events to accumulate before processing
	}
}

// EventType represents different types of events that can be aggregated
type EventType int

const (
	EventVehicleDestruction EventType = iota
	EventPlayerDeath
	EventVehicleSpawn
	EventActorState
)

// PendingEvent holds information about an event waiting to be aggregated
type PendingEvent struct {
	Type        EventType
	Timestamp   time.Time
	PlayerName  string
	VehicleName string
	Cause       string
	Weapon      string
	RawLine     string
	Details     map[string]string
}

// EventAggregator manages combining related events into mission summaries
type EventAggregator struct {
	PendingEvents []PendingEvent
	TimeWindow    time.Duration // Events within this window are considered related
}

// NewEventAggregator creates a new event aggregator with a 5-second time window
func NewEventAggregator() *EventAggregator {
	return &EventAggregator{
		PendingEvents: make([]PendingEvent, 0),
		TimeWindow:    5 * time.Second, // Events within 5 seconds are considered related
	}
}

// AddEvent adds an event to the pending list
func (ea *EventAggregator) AddEvent(event PendingEvent) {
	ea.PendingEvents = append(ea.PendingEvents, event)
}

// FlushOldEvents processes and flushes events older than the time window
func (ea *EventAggregator) FlushOldEvents(currentTime time.Time, processor *Processor) []string {
	var messages []string
	var remainingEvents []PendingEvent
	var oldEvents []PendingEvent

	for _, event := range ea.PendingEvents {
		if currentTime.Sub(event.Timestamp) > ea.TimeWindow {
			oldEvents = append(oldEvents, event)
		} else {
			remainingEvents = append(remainingEvents, event)
		}
	}
	// Group old events by player and try to create mission summaries
	playerEvents := make(map[string][]PendingEvent)
	for _, event := range oldEvents {
		playerEvents[event.PlayerName] = append(playerEvents[event.PlayerName], event)
	}

	// Create mission summaries for each player
	for _, events := range playerEvents {
		if summary := ea.createMissionSummary(events); summary != "" {
			messages = append(messages, summary)
		} else {
			// If no summary could be created, output individual events
			for _, event := range events {
				messages = append(messages, ea.CreateIndividualEventMessage(event))
			}
		}
	}

	ea.PendingEvents = remainingEvents
	return messages
}

// ProcessEventsForPlayer looks for related events for a specific player and creates summaries
func (ea *EventAggregator) ProcessEventsForPlayer(playerName string, currentTime time.Time) string {
	var relatedEvents []PendingEvent
	var remainingEvents []PendingEvent

	for _, event := range ea.PendingEvents {
		if event.PlayerName == playerName && currentTime.Sub(event.Timestamp) <= ea.TimeWindow {
			relatedEvents = append(relatedEvents, event)
		} else {
			remainingEvents = append(remainingEvents, event)
		}
	}

	ea.PendingEvents = remainingEvents

	if len(relatedEvents) > 0 {
		return ea.createMissionSummary(relatedEvents)
	}

	return ""
}

// createMissionSummary analyzes related events and creates a coherent mission summary
func (ea *EventAggregator) createMissionSummary(events []PendingEvent) string {
	if len(events) == 0 {
		return ""
	}

	// Sort events by timestamp
	for i := 0; i < len(events)-1; i++ {
		for j := i + 1; j < len(events); j++ {
			if events[i].Timestamp.After(events[j].Timestamp) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}

	// Analyze the sequence of events
	var vehicleDestroyed bool
	var playerDied bool
	var crashCause bool
	var playerName string
	var vehicleName string

	for _, event := range events {
		switch event.Type {
		case EventVehicleDestruction:
			vehicleDestroyed = true
			vehicleName = event.VehicleName
			if strings.ToLower(event.Cause) == "collision" || strings.ToLower(event.Weapon) == "collision" {
				crashCause = true
			}
		case EventPlayerDeath:
			playerDied = true
			playerName = event.PlayerName
			if strings.ToLower(event.Cause) == "crash" || strings.ToLower(event.Weapon) == "crash" {
				crashCause = true
			}
		}
	}

	// Create mission summary based on detected patterns
	if vehicleDestroyed && playerDied && crashCause && playerName != "" {
		if vehicleName != "" {
			return fmt.Sprintf("Mission Event: %s crashed their %s and died", playerName, cleanName(vehicleName))
		} else {
			return fmt.Sprintf("Mission Event: %s died in a crash", playerName)
		}
	}

	// If we can't create a meaningful summary, return empty string to use individual events
	return ""
}

// CreateIndividualEventMessage creates a message for a single event that couldn't be aggregated
func (ea *EventAggregator) CreateIndividualEventMessage(event PendingEvent) string {
	switch event.Type {
	case EventVehicleDestruction:
		if event.VehicleName != "" {
			return fmt.Sprintf("Vehicle %s was destroyed by %s", cleanName(event.VehicleName), event.Cause)
		}
		return fmt.Sprintf("Vehicle was destroyed by %s", event.Cause)
	case EventPlayerDeath:
		if event.Weapon != "" && event.Weapon != "unknown" {
			return fmt.Sprintf("You were killed by: %s using %s", event.Cause, event.Weapon)
		}
		return fmt.Sprintf("You died by %s", event.Cause)
	case EventActorState:
		if event.Cause == "corpse" {
			return "You turned to a corpse"
		}
		return fmt.Sprintf("You %s", event.Cause)
	default:
		return event.RawLine
	}
}
