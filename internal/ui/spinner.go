// Package ui provides terminal UI components.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Spinner displays an animated spinner to indicate activity.
type Spinner struct {
	frames   []string
	interval time.Duration
	message  string
	writer   io.Writer
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	running  bool
}

// SpinnerOption configures a Spinner.
type SpinnerOption func(*Spinner)

// WithMessage sets the message displayed next to the spinner.
func WithMessage(msg string) SpinnerOption {
	return func(s *Spinner) {
		s.message = msg
	}
}

// WithWriter sets the output writer (default: os.Stderr).
func WithWriter(w io.Writer) SpinnerOption {
	return func(s *Spinner) {
		s.writer = w
	}
}

// WithInterval sets the animation interval (default: 100ms).
func WithInterval(d time.Duration) SpinnerOption {
	return func(s *Spinner) {
		s.interval = d
	}
}

// NewSpinner creates a new spinner.
func NewSpinner(opts ...SpinnerOption) *Spinner {
	s := &Spinner{
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		interval: 100 * time.Millisecond,
		writer:   os.Stderr,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.run()
}

// Stop halts the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stop)
	<-s.done

	// Clear the spinner line
	s.clearLine()
}

// UpdateMessage changes the spinner message while running.
func (s *Spinner) UpdateMessage(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}

func (s *Spinner) run() {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := s.message
			s.mu.Unlock()

			s.clearLine()
			_, _ = fmt.Fprintf(s.writer, "\r%s %s", s.frames[frame], msg)

			frame = (frame + 1) % len(s.frames)
		}
	}
}

func (s *Spinner) clearLine() {
	// Move to start of line and clear it
	_, _ = fmt.Fprintf(s.writer, "\r\033[K")
}

// IsTerminal returns true if the given writer is a terminal.
func IsTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
	return false
}
