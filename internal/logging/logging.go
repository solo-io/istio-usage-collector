package logging

import (
	"sync"
	"time"

	"github.com/pterm/pterm"
)

// These variables control global logging behavior
var (
	// Enable or disable progress output
	progressEnabled = true
)

func EnableDebugMessages() {
	pterm.EnableDebugMessages()
}

// Info logs an informational message
func Info(format string, args ...interface{}) {
	pterm.Info.Printfln(format, args...)
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	pterm.Warning.Printfln(format, args...)
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	pterm.Error.Printfln(format, args...)
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	pterm.Debug.Printfln(format, args...)
}

// Success logs a success message
func Success(format string, args ...interface{}) {
	pterm.Success.Printfln(format, args...)
}

// DisableProgress disables progress bar output
func DisableProgress() {
	progressEnabled = false
}

// EnableProgress enables progress bar output
func EnableProgress() {
	progressEnabled = true
}

// Progress represents a progress bar
type Progress struct {
	bar *pterm.ProgressbarPrinter
	// Keep track of current value for incremental updates
	current int
	// Keep track of total for calculation
	total int
	// Mutex for thread safety
	mu sync.Mutex
}

// NewProgress creates a new progress bar
func NewProgress(title string, total int) *Progress {
	if !progressEnabled {
		// Return a dummy progress object that doesn't show anything
		return &Progress{
			total: total,
		}
	}

	// Configure the progress bar with an enhanced style
	bar, _ := pterm.DefaultProgressbar.
		WithTitle(title).
		WithTotal(total).
		WithShowCount(true).
		WithShowPercentage(true).
		WithShowElapsedTime(true).
		WithShowTitle(true).
		WithRemoveWhenDone(false).
		Start()

	return &Progress{
		bar:     bar,
		total:   total,
		current: 0,
	}
}

// Increment increments the progress bar by 1
func (p *Progress) Increment() {
	if p.bar == nil || !progressEnabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.bar.Add(1)
	p.current++
}

// Complete completes the progress bar
func (p *Progress) Complete() {
	if p.bar == nil || !progressEnabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Update to 100% if not already there
	if p.current < p.total {
		p.bar.Add(p.total - p.current)
	}

	// Wait a moment to ensure it's visible
	time.Sleep(100 * time.Millisecond)

	// Stop the progress bar with success
	p.bar.Stop()

	// Show a success message
	pterm.Success.Printfln("Completed %s", p.bar.Title)
}
