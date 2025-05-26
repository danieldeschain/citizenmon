package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Stats holds tracked player interactions.
type Stats struct {
	Kills       map[string]int `json:"kills"`
	Deaths      map[string]int `json:"deaths"`
	Incaps      map[string]int `json:"incaps"`
	Appearances map[string]int `json:"appearances"`
}

// New initializes an empty Stats.
func New() Stats {
	return Stats{
		Kills:       make(map[string]int),
		Deaths:      make(map[string]int),
		Incaps:      make(map[string]int),
		Appearances: make(map[string]int),
	}
}

// getStatsDir returns the directory for saving stats files (same as feeds)
func getStatsDir() string {
	dir := filepath.Join(os.Getenv("APPDATA"), "citizenmon", "feeds")
	os.MkdirAll(dir, 0755)
	return dir
}

// Load reads stats from <player>_stats.json in the stats dir, or returns empty on error.
func Load(player string) Stats {
	if player == "" {
		return New()
	}
	fname := filepath.Join(getStatsDir(), player+"_stats.json")
	f, err := os.Open(fname)
	if err != nil {
		return New()
	}
	defer f.Close()
	var s Stats
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return New()
	}
	return s
}

// Save writes stats to <player>_stats.json in the stats dir.
func Save(player string, s Stats) error {
	if player == "" {
		return nil
	}
	fname := filepath.Join(getStatsDir(), player+"_stats.json")
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(s)
}
