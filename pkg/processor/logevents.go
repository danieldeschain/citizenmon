package processor

import "time"

// KillEvent represents a kill event in the log.
type KillEvent struct {
	Killer    string
	Victim    string
	Weapon    string
	Timestamp time.Time
}

// DeathEvent represents a death event in the log.
type DeathEvent struct {
	Player    string
	Timestamp time.Time
}

// CorpseEvent represents a corpse event in the log.
type CorpseEvent struct {
	Player    string
	Timestamp time.Time
}
