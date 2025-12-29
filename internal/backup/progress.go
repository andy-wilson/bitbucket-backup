package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andy-wilson/bb-backup/internal/ui"
)

// Progress tracks and reports backup progress.
type Progress struct {
	mu           sync.Mutex   // Only for current string and non-atomic operations
	startTime    time.Time
	total        int64
	completed    atomic.Int64 // Lock-free counter
	failed       atomic.Int64 // Lock-free counter
	interrupted  atomic.Int64 // Lock-free counter
	active       atomic.Int64 // Number of repos currently being processed
	current      string       // Most recently started repo (for display)
	jsonOutput   bool
	quiet        bool
	interactive  bool
	lastUpdate   time.Time
	updatePeriod time.Duration
	progressBar  *ui.ProgressBar
}

// ProgressEvent represents a progress update in JSON format.
type ProgressEvent struct {
	Type       string  `json:"type"`
	Timestamp  string  `json:"timestamp"`
	Total      int     `json:"total"`
	Completed  int     `json:"completed"`
	Failed     int     `json:"failed"`
	Percent    float64 `json:"percent"`
	Current    string  `json:"current,omitempty"`
	Message    string  `json:"message,omitempty"`
	ElapsedSec float64 `json:"elapsed_seconds"`
}

// NewProgress creates a new progress tracker.
func NewProgress(total int, jsonOutput, quiet, interactive bool) *Progress {
	p := &Progress{
		startTime:    time.Now(),
		total:        int64(total),
		jsonOutput:   jsonOutput,
		quiet:        quiet,
		interactive:  interactive,
		updatePeriod: 500 * time.Millisecond,
	}

	// Create progress bar for interactive mode
	if interactive && !jsonOutput && !quiet {
		p.progressBar = ui.NewProgressBar(total, ui.WithTwoLineMode())
		p.progressBar.Start()
	}

	return p
}

// Start marks the start of a new item.
func (p *Progress) Start(name string) {
	p.StartWithType(name, "")
}

// StartWithType marks the start of a new item with a type indicator (e.g., "updating", "cloning").
func (p *Progress) StartWithType(name, itemType string) {
	activeCount := p.active.Add(1) // Increment active counter

	p.mu.Lock()
	defer p.mu.Unlock()

	if itemType != "" {
		p.current = itemType + ": " + name
	} else {
		p.current = name
	}

	if p.progressBar != nil {
		// Show active count when multiple workers are running
		if activeCount > 1 {
			p.progressBar.SetCurrent(fmt.Sprintf("%d repos in progress", activeCount))
		} else {
			p.progressBar.SetCurrent(p.current)
		}
	} else {
		if itemType != "" {
			p.emit("start", fmt.Sprintf("%s: %s", itemType, name))
		} else {
			p.emit("start", fmt.Sprintf("Starting: %s", name))
		}
	}
}

// Complete marks an item as completed.
func (p *Progress) Complete(name string) {
	p.completed.Add(1)       // Atomic increment
	activeCount := p.active.Add(-1) // Decrement active counter

	p.mu.Lock()
	p.current = ""
	p.mu.Unlock()

	if p.progressBar != nil {
		p.progressBar.Complete(name)
		// Update status to reflect remaining active count
		if activeCount > 1 {
			p.progressBar.SetCurrent(fmt.Sprintf("%d repos in progress", activeCount))
		} else if activeCount == 1 {
			p.progressBar.SetCurrent("1 repo in progress")
		} else {
			// activeCount == 0: nothing in progress
			p.progressBar.SetCurrent("")
		}
	} else {
		p.mu.Lock()
		p.emitProgress("complete", fmt.Sprintf("Completed: %s", name))
		p.mu.Unlock()
	}
}

// Fail marks an item as failed.
func (p *Progress) Fail(name string, err error) {
	p.failed.Add(1)          // Atomic increment
	activeCount := p.active.Add(-1) // Decrement active counter

	p.mu.Lock()
	p.current = ""
	p.mu.Unlock()

	if p.progressBar != nil {
		p.progressBar.Fail(name)
		// Update status to reflect remaining active count
		if activeCount > 1 {
			p.progressBar.SetCurrent(fmt.Sprintf("%d repos in progress", activeCount))
		} else if activeCount == 1 {
			p.progressBar.SetCurrent("1 repo in progress")
		} else {
			// activeCount == 0: nothing in progress
			p.progressBar.SetCurrent("")
		}
	} else {
		p.mu.Lock()
		p.emitProgress("fail", fmt.Sprintf("Failed: %s - %v", name, err))
		p.mu.Unlock()
	}
}

// Update emits a progress update if enough time has passed.
func (p *Progress) Update() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if time.Since(p.lastUpdate) < p.updatePeriod {
		return
	}

	p.emitProgress("progress", "")
}

// Interrupt marks an item as interrupted (e.g., by CTRL-C).
func (p *Progress) Interrupt(name string) {
	p.interrupted.Add(1) // Atomic increment
	p.active.Add(-1)     // Decrement active counter

	p.mu.Lock()
	p.current = ""
	p.mu.Unlock()
	// Don't update progress bar - just track the count
}

// Summary prints the final summary.
func (p *Progress) Summary() {
	// Stop progress bar if running
	if p.progressBar != nil {
		p.progressBar.Stop()
	}

	completed := p.completed.Load()
	failed := p.failed.Load()
	interrupted := p.interrupted.Load()

	elapsed := time.Since(p.startTime)
	var msg string
	if interrupted > 0 {
		msg = fmt.Sprintf("Backup complete: %d/%d succeeded, %d failed, %d interrupted in %s",
			completed, p.total, failed, interrupted, elapsed.Round(time.Second))
	} else {
		msg = fmt.Sprintf("Backup complete: %d/%d succeeded, %d failed in %s",
			completed, p.total, failed, elapsed.Round(time.Second))
	}

	// For interactive mode, print the summary after progress bar stops
	if p.interactive && !p.jsonOutput && !p.quiet {
		fmt.Printf("\n%s\n", msg)
		return
	}

	p.mu.Lock()
	p.emit("summary", msg)
	p.mu.Unlock()
}

// emitProgress emits a progress event with rate limiting for text output.
func (p *Progress) emitProgress(eventType, message string) {
	if p.quiet && !p.jsonOutput {
		return
	}

	now := time.Now()
	if !p.jsonOutput && time.Since(p.lastUpdate) < p.updatePeriod && eventType == "progress" {
		return
	}
	p.lastUpdate = now

	p.emitLocked(eventType, message)
}

// emit emits a progress event unconditionally.
func (p *Progress) emit(eventType, message string) {
	if p.quiet && !p.jsonOutput {
		return
	}
	p.emitLocked(eventType, message)
}

// emitLocked emits the event (caller must hold lock for current string).
func (p *Progress) emitLocked(eventType, message string) {
	completed := p.completed.Load()
	failed := p.failed.Load()

	if p.jsonOutput {
		event := ProgressEvent{
			Type:       eventType,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Total:      int(p.total),
			Completed:  int(completed),
			Failed:     int(failed),
			Percent:    p.percent(),
			Current:    p.current,
			Message:    message,
			ElapsedSec: time.Since(p.startTime).Seconds(),
		}
		data, _ := json.Marshal(event)
		_, _ = fmt.Fprintln(os.Stdout, string(data))
	} else if message != "" {
		fmt.Printf("[%d/%d] %s\n", completed+failed, p.total, message)
	}
}

// percent calculates completion percentage.
func (p *Progress) percent() float64 {
	if p.total == 0 {
		return 0
	}
	return float64(p.completed.Load()+p.failed.Load()) / float64(p.total) * 100
}

// GetStats returns the current stats.
func (p *Progress) GetStats() (completed, failed int) {
	return int(p.completed.Load()), int(p.failed.Load())
}

// UpdateStatus updates the current status text without changing progress counts.
// Used to show metadata fetch progress (e.g., "fetching PRs: repo-name (5/10)").
func (p *Progress) UpdateStatus(status string) {
	p.mu.Lock()
	p.current = status
	p.mu.Unlock()

	if p.progressBar != nil {
		p.progressBar.SetCurrent(status)
	}
}
