package logging

import (
	"os"

	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func init() {
	// Set up the logger
	log.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(logrus.InfoLevel)
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

// Progress represents a progress bar - used for the in-progress logging in case of long running command
type Progress struct {
	bar *progressbar.ProgressBar
}

// NewProgress creates a new progress bar
func NewProgress(total, width int) *Progress {
	bar := progressbar.NewOptions(total,
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(width),
		progressbar.OptionSetDescription("Processing"),
		progressbar.OptionShowIts(),
	)

	return &Progress{
		bar: bar,
	}
}

// Update updates the progress bar
func (p *Progress) Update(current int) {
	p.bar.Set(current)
}

// Complete completes the progress bar
func (p *Progress) Complete() {
	p.bar.Finish()
}
