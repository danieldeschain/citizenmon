package processor

import (
	"fmt"
	"regexp"
	"strings"

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
	PlayerName   string
	Stats        stats.Stats
	OutputBox    *widget.Entry
	PlayerLabel  *widget.Label
	AppendOutput func(line string)
}

// New creates a Processor bound to the given output entry and label.
func New(output *widget.Entry, label *widget.Label) *Processor {
	p := &Processor{
		Stats:       stats.New(),
		OutputBox:   output,
		PlayerLabel: label,
	}
	// default AppendOutput updates the UI entry on main thread
	p.AppendOutput = func(line string) {
		fyne.Do(func() {
			if p.PlayerLabel != nil && p.PlayerName != "" {
				p.PlayerLabel.SetText(p.PlayerName)
			}
			if p.OutputBox != nil {
				p.OutputBox.SetText(p.OutputBox.Text + line + "\n")
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
	if strings.Contains(line, "Character:") && strings.Contains(line, "name") {
		parts := strings.Fields(line)
		for i, tok := range parts {
			if tok == "name" && i+1 < len(parts) {
				p.PlayerName = strings.Trim(parts[i+1], "-:[]{}\\\",'")
				p.AppendOutput("Detected player name: " + p.PlayerName)
				p.Stats = stats.Load(p.PlayerName)
				return
			}
		}
	}
}

// ProcessLogLine updates stats based on a single log line.
func (p *Processor) ProcessLogLine(line string) {
	if p.PlayerName == "" {
		p.AppendOutput("PlayerName is empty, skipping stats update for line: " + line)
		return
	}

	// Vehicle destruction
	if strings.Contains(line, "CVehicle::OnAdvanceDestroyLevel") {
		if m := vehicleRegex.FindStringSubmatch(line); len(m) == 6 {
			fullID := m[1]
			toLevel := m[3]
			causeRaw := m[4]
			weaponRaw := m[5]

			// clean up names
			vehicle := cleanName(fullID)
			cause := causeRaw
			weapon := cleanName(weaponRaw)

			// determine status
			status := map[string]string{"1": "disabled", "2": "destroyed"}[toLevel]

			// construct message
			var msg string
			if strings.EqualFold(weaponRaw, "Collision") {
				msg = fmt.Sprintf("Vehicle %s was %s by %s (collision)", vehicle, status, cause)
			} else {
				msg = fmt.Sprintf("Vehicle %s was %s by %s using %s", vehicle, status, cause, weapon)
			}
			p.AppendOutput(msg)
		}
		return
	}

	// Kills and deaths
	if strings.Contains(line, "CActor::Kill:") {
		// suicide
		suicidePattern := fmt.Sprintf(`CActor::Kill: '%s'.*killed by '%s'`, regexp.QuoteMeta(p.PlayerName), regexp.QuoteMeta(p.PlayerName))
		suicideRe := regexp.MustCompile(suicidePattern)
		if suicideRe.MatchString(line) {
			p.Stats.Deaths["Suicide"]++
			stats.Save(p.PlayerName, p.Stats)
			p.AppendOutput("You died by suicide")
			return
		}

		// kill by player with method
		rMethod := regexp.MustCompile(`CActor::Kill: '([A-Za-z0-9_]+)'.*killed by '` + regexp.QuoteMeta(p.PlayerName) + `'.*using '([^']+)'`)
		if m := rMethod.FindStringSubmatch(line); len(m) == 3 {
			victim := m[1]
			method := cleanName(m[2])
			p.Stats.Kills[victim]++
			stats.Save(p.PlayerName, p.Stats)
			p.AppendOutput(fmt.Sprintf("You killed: %s using %s", victim, method))
			return
		}

		// fallback kill by player
		rKill := regexp.MustCompile(`CActor::Kill: '([A-Za-z0-9_]+)'.*killed by '` + regexp.QuoteMeta(p.PlayerName) + `'`)
		if m := rKill.FindStringSubmatch(line); len(m) > 1 {
			victim := m[1]
			p.Stats.Kills[victim]++
			stats.Save(p.PlayerName, p.Stats)
			p.AppendOutput("You killed: " + victim)
		}

		// killed by another
		rDeath := regexp.MustCompile(`CActor::Kill: '` + regexp.QuoteMeta(p.PlayerName) + `'.*killed by '([A-Za-z0-9_]+)'(?:.*using '([^']+)')?`)
		if m := rDeath.FindStringSubmatch(line); len(m) > 1 {
			killer := m[1]
			weapon := ""
			if len(m) == 3 {
				weapon = cleanName(m[2])
			}
			p.Stats.Deaths[killer]++
			stats.Save(p.PlayerName, p.Stats)
			if weapon != "" {
				p.AppendOutput(fmt.Sprintf("You were killed by: %s using %s", killer, weapon))
			} else {
				p.AppendOutput("You were killed by: " + killer)
			}
		}
	}

	// Incapacitations
	if strings.Contains(line, "Logged an incap") {
		r := regexp.MustCompile(`nickname: ([A-Za-z0-9_]+)`)
		if m := r.FindStringSubmatch(line); len(m) > 1 && m[1] != p.PlayerName {
			target := m[1]
			p.Stats.Incaps[target]++
			stats.Save(p.PlayerName, p.Stats)
			p.AppendOutput("You incapacitated: " + target)
		}
	}

	// Appearances
	if strings.Contains(line, "nickname=") {
		r := regexp.MustCompile(`nickname="?([A-Za-z0-9_]+)"?`)
		if m := r.FindStringSubmatch(line); len(m) > 1 && m[1] != p.PlayerName {
			name := m[1]
			p.Stats.Appearances[name]++
			stats.Save(p.PlayerName, p.Stats)
			p.AppendOutput("Player appeared: " + name)
		}
	}

	// Status changes (corpse, control state)
	if corpseRegex.MatchString(line) || strings.Contains(line, "Entering control state") {
		p.AppendOutput("Player status change: " + line)
	}
}
