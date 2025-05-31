package watcher

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
)

// LogHandler defines the interface the watcher uses to feed log lines.
type LogHandler interface {
	DetectPlayerName(line string)
	ProcessLogLine(line string)
	AppendOutput(line string)
}

// WatchLogFile tails the game log at the given path using polling.
func WatchLogFile(path string, proc LogHandler) {
	// Normalize and clean the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		proc.AppendOutput("failed to get absolute path: " + err.Error())
		return
	}
	absPath = filepath.Clean(absPath)

	// Open the log file
	file, err := os.Open(absPath)
	if err != nil {
		proc.AppendOutput("failed to open log file: " + err.Error())
		return
	}
	defer file.Close()

	// Initial scan: detect player name only, with large buffer for long lines
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		proc.DetectPlayerName(scanner.Text())
	}	// Seek to end for new data
	offset, _ := file.Seek(0, io.SeekCurrent)

	// Poll for changes every 500ms (half second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// Check file stat
		info, err := os.Stat(absPath)
		if err != nil {
			// File might have been moved/deleted, try to reopen
			file.Close()
			time.Sleep(100 * time.Millisecond)
			
			file, err = os.Open(absPath)
			if err != nil {
				continue
			}
			offset = 0
			continue
		}

		// Check for truncation
		if info.Size() < offset {
			offset = 0
		}

		// Check if file has new content
		if info.Size() > offset {
			// Read new lines with large buffer
			file.Seek(offset, io.SeekStart)
			scanner2 := bufio.NewScanner(file)
			scanner2.Buffer(buf, 10*1024*1024)
			for scanner2.Scan() {
				line := scanner2.Text()
				fyne.Do(func() { 
					proc.DetectPlayerName(line)
					proc.ProcessLogLine(line) 
				})
			}
			offset, _ = file.Seek(0, io.SeekCurrent)
		}
	}
}
