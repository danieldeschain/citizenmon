package watcher

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/fsnotify/fsnotify"
)

// LogHandler defines the interface the watcher uses to feed log lines.
type LogHandler interface {
	DetectPlayerName(line string)
	ProcessLogLine(line string)
	AppendOutput(line string)
}

// WatchLogFile tails the game log at the given path using fsnotify.
func WatchLogFile(path string, proc LogHandler) {
	// Normalize and clean the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		proc.AppendOutput("failed to get absolute path: " + err.Error())
		return
	}
	absPath = filepath.Clean(absPath)

	// Create watcher
	w, err := fsnotify.NewWatcher()
	if err != nil {
		proc.AppendOutput("failed to create watcher: " + err.Error())
		return
	}
	defer w.Close()

	// Watch the directory
	dir := filepath.Dir(absPath)
	if err := w.Add(dir); err != nil {
		proc.AppendOutput("failed to watch directory: " + err.Error())
		return
	}

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
	}

	// Seek to end for new data
	offset, _ := file.Seek(0, io.SeekCurrent)

	// Polling goroutine to check for new data every 1 second
	go func() {
		for {
			time.Sleep(1 * time.Second)
			info, err := file.Stat()
			if err == nil && info.Size() > offset {
				file.Seek(offset, io.SeekStart)
				scanner2 := bufio.NewScanner(file)
				scanner2.Buffer(buf, 10*1024*1024)
				for scanner2.Scan() {
					line := scanner2.Text()
					fyne.Do(func() { proc.DetectPlayerName(line); proc.ProcessLogLine(line) })
				}
				offset, _ = file.Seek(0, io.SeekCurrent)
			}
		}
	}()

	for {
		select {
		case ev := <-w.Events:
			// Only proceed if the event is for our file
			evClean := filepath.Clean(ev.Name)
			if !strings.EqualFold(evClean, absPath) {
				continue
			}

			// Handle rotation or removal
			if ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				file.Close()
				time.Sleep(100 * time.Millisecond)

				file, err = os.Open(absPath)
				if err != nil {
					continue
				}
				offset = 0
			}

			// Handle writes or creations
			if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Wait briefly for writes to flush
				time.Sleep(50 * time.Millisecond)

				// Check for truncation or file stat error
				info, statErr := file.Stat()
				if statErr != nil {
					continue
				}
				if info.Size() < offset {
					offset = 0
				}

				// Read new lines with large buffer
				file.Seek(offset, io.SeekStart)
				scanner2 := bufio.NewScanner(file)
				scanner2.Buffer(buf, 10*1024*1024)
				for scanner2.Scan() {
					line := scanner2.Text()
					fyne.Do(func() { proc.DetectPlayerName(line); proc.ProcessLogLine(line) })
				}
				offset, _ = file.Seek(0, io.SeekCurrent)
			}

		case <-w.Errors:
			// Optionally handle watcher errors
			// (no-op)
		}
	}
}
