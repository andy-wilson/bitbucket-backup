// Package ui provides terminal UI components.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Spinner frames for activity indicator
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ProgressBar displays an animated progress bar with ETA.
type ProgressBar struct {
	writer        io.Writer
	total         int
	completed     int
	failed        int
	current       string
	startTime     time.Time
	width         int
	interval      time.Duration
	stop          chan struct{}
	done          chan struct{}
	mu            sync.Mutex
	running       bool
	avgDuration   time.Duration
	completedList []time.Duration // Track individual completion times for ETA
	spinnerIdx    int             // Current spinner frame
	twoLineMode   bool            // Show current repo on separate line above progress bar
}

// ProgressBarOption configures a ProgressBar.
type ProgressBarOption func(*ProgressBar)

// WithBarWriter sets the output writer (default: os.Stderr).
func WithBarWriter(w io.Writer) ProgressBarOption {
	return func(p *ProgressBar) {
		p.writer = w
	}
}

// WithBarWidth sets the progress bar width in characters (default: 40).
func WithBarWidth(width int) ProgressBarOption {
	return func(p *ProgressBar) {
		p.width = width
	}
}

// WithUpdateInterval sets the refresh interval (default: 200ms).
func WithUpdateInterval(d time.Duration) ProgressBarOption {
	return func(p *ProgressBar) {
		p.interval = d
	}
}

// WithTwoLineMode enables two-line display with current repo above the progress bar.
func WithTwoLineMode() ProgressBarOption {
	return func(p *ProgressBar) {
		p.twoLineMode = true
	}
}

// NewProgressBar creates a new progress bar.
func NewProgressBar(total int, opts ...ProgressBarOption) *ProgressBar {
	p := &ProgressBar{
		writer:        os.Stderr,
		total:         total,
		width:         40,
		interval:      200 * time.Millisecond,
		startTime:     time.Now(),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
		completedList: make([]time.Duration, 0, total),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Start begins the progress bar animation.
func (p *ProgressBar) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.startTime = time.Now()
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	p.mu.Unlock()

	// In two-line mode, print initial empty line for the status line
	if p.twoLineMode {
		fmt.Fprintln(p.writer, "")
	}

	go p.run()
}

// Stop halts the progress bar and moves to next line.
func (p *ProgressBar) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	close(p.stop)
	<-p.done

	// Print final state and move to next line
	p.render()
	fmt.Fprintln(p.writer)
}

// SetCurrent sets the current item being processed.
func (p *ProgressBar) SetCurrent(name string) {
	p.mu.Lock()
	p.current = name
	p.mu.Unlock()
}

// Complete marks an item as completed.
// Note: Does not clear current - caller should manage via SetCurrent.
func (p *ProgressBar) Complete(name string) {
	p.mu.Lock()
	elapsed := time.Since(p.startTime)
	if p.completed > 0 {
		// Track time since last completion for ETA calculation
		avgPerItem := elapsed / time.Duration(p.completed+1)
		p.avgDuration = avgPerItem
	}
	p.completed++
	p.completedList = append(p.completedList, elapsed)
	p.mu.Unlock()
}

// Fail marks an item as failed.
// Note: Does not clear current - caller should manage via SetCurrent.
func (p *ProgressBar) Fail(name string) {
	p.mu.Lock()
	p.failed++
	p.mu.Unlock()
}

// GetStats returns current statistics.
func (p *ProgressBar) GetStats() (completed, failed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.completed, p.failed
}

func (p *ProgressBar) run() {
	defer close(p.done)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.render()
		}
	}
}

func (p *ProgressBar) render() {
	p.mu.Lock()
	completed := p.completed
	failed := p.failed
	total := p.total
	current := p.current
	startTime := p.startTime
	twoLineMode := p.twoLineMode
	spinnerIdx := p.spinnerIdx
	p.spinnerIdx = (p.spinnerIdx + 1) % len(spinnerFrames)
	p.mu.Unlock()

	elapsed := time.Since(startTime)
	processed := completed + failed
	percent := float64(0)
	if total > 0 {
		percent = float64(processed) / float64(total) * 100
	}

	// Calculate ETA
	var eta time.Duration
	var etaTime time.Time
	if processed > 0 && processed < total {
		avgPerItem := elapsed / time.Duration(processed)
		remaining := total - processed
		eta = avgPerItem * time.Duration(remaining)
		etaTime = time.Now().Add(eta)
	}

	// Build progress bar
	bar := p.buildBar(percent)

	if twoLineMode {
		// Two-line mode: status line above, progress bar below
		// Move cursor up, clear line, write status, move down, clear line, write progress

		// Build status line with spinner and current repo
		statusLine := ""
		if current != "" {
			spinner := spinnerFrames[spinnerIdx]
			statusLine = fmt.Sprintf("%s %s", spinner, current)
		} else if processed >= total && total > 0 {
			statusLine = "✓ Complete"
		} else {
			statusLine = "Waiting..."
		}

		// Build progress line
		progressLine := fmt.Sprintf("%s %.0f%% (%d/%d", bar, percent, processed, total)
		if failed > 0 {
			progressLine += fmt.Sprintf(", %d failed", failed)
		}
		progressLine += ")"
		progressLine += fmt.Sprintf(" ⏱ %s", formatDuration(elapsed))
		if eta > 0 {
			progressLine += fmt.Sprintf(" ETA: %s (%s)", formatDuration(eta), etaTime.Format("15:04:05"))
		}

		// Move up, clear, write status, move down, clear, write progress
		fmt.Fprintf(p.writer, "\033[F\033[K%s\n\033[K%s", statusLine, progressLine)
	} else {
		// Single-line mode (original behavior)
		statusLine := fmt.Sprintf("\r%s %.0f%% (%d/%d", bar, percent, processed, total)
		if failed > 0 {
			statusLine += fmt.Sprintf(", %d failed", failed)
		}
		statusLine += ")"

		// Runtime
		statusLine += fmt.Sprintf(" ⏱ %s", formatDuration(elapsed))

		// ETA
		if eta > 0 {
			statusLine += fmt.Sprintf(" ETA: %s (%s)", formatDuration(eta), etaTime.Format("15:04:05"))
		} else if processed >= total && total > 0 {
			statusLine += " ✓ Complete"
		}

		// Current item (truncated to fit)
		if current != "" {
			maxLen := 30
			display := current
			if len(display) > maxLen {
				display = "..." + display[len(display)-maxLen+3:]
			}
			statusLine += fmt.Sprintf(" │ %s", display)
		}

		// Clear line and write
		p.clearLine()
		fmt.Fprint(p.writer, statusLine)
	}
}

func (p *ProgressBar) buildBar(percent float64) string {
	filled := int(float64(p.width) * percent / 100)
	if filled > p.width {
		filled = p.width
	}
	empty := p.width - filled

	// Use block characters for nice visual
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return "[" + bar + "]"
}

func (p *ProgressBar) clearLine() {
	fmt.Fprintf(p.writer, "\r\033[K")
}

// formatDuration formats a duration in a human-friendly way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%02ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%02dm%02ds", hours, mins, secs)
}
