package logging

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()
var currentProgressBar *progressbar.ProgressBar

// progressMutex is used to ensure that the progress bar is updated in a thread-safe manner
var progressMutex sync.Mutex

// Custom writer that coordinates with the progress bar
// This is used to ensure that the progress bar stays below the log messages, avoiding issues where new log messages cause the progress bar to be redrawn each log
type progressAwareWriter struct {
	out io.Writer
}

func (pw *progressAwareWriter) Write(p []byte) (n int, err error) {
	progressMutex.Lock()
	defer progressMutex.Unlock()

	// If there's an active progress bar, clear it before writing logs
	if currentProgressBar != nil {
		currentProgressBar.Clear()
	}

	// Write the log message
	n, err = pw.out.Write(p)

	// If there's an active progress bar, redraw it after the log
	if currentProgressBar != nil {
		currentProgressBar.RenderBlank()
	}

	return n, err
}

func init() {
	// Set up the logger
	log.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})

	// Use our custom writer that's aware of the progress bar
	log.SetOutput(&progressAwareWriter{out: os.Stdout})
}

// SetLevel sets the log level
func SetLevel(level string) {
	switch level {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	case "fatal":
		log.SetLevel(logrus.FatalLevel)
	default:
		// Default to info level
		log.SetLevel(logrus.InfoLevel)
		log.Warnf("Invalid log level %s, defaulting to info", level)
	}
}

// Info logs an informational message
func Info(format string, args ...interface{}) {
	log.Infof(format, args...)
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	log.Warnf(format, args...)
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	log.Debugf(format, args...)
}

// Progress represents a progress bar - used for the in-progress logging in case of long running command
type Progress struct {
	bar *progressbar.ProgressBar
}

// NewProgress creates a new progress bar
func NewProgress(title string, total int) *Progress {
	progressMutex.Lock()
	defer progressMutex.Unlock()

	bar := progressbar.NewOptions(total,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetDescription(fmt.Sprintf("[cyan]%s[reset]", title)),
		progressbar.OptionShowIts(),
		progressbar.OptionFullWidth(),
	)

	currentProgressBar = bar

	return &Progress{
		bar: bar,
	}
}

// Update updates the progress bar
func (p *Progress) Update(current int) {
	progressMutex.Lock()
	defer progressMutex.Unlock()

	p.bar.Set(current)
}

// Complete completes the progress bar
func (p *Progress) Complete() {
	progressMutex.Lock()
	defer progressMutex.Unlock()

	p.bar.Finish()

	// Clear the reference to the current progress bar so future logs aren't affected by it
	currentProgressBar = nil

	// Add new line so subsequent logs aren't on the same line
	fmt.Println()
}
