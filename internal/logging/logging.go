// Package logging configures the global logrus logger.
package logging

import (
	"io"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

// PrepareLogs configures JSON logging. When logName is empty, logs go to stdout
// only; otherwise they are tee'd to both stdout and the named file. File errors
// degrade gracefully to stdout rather than crashing startup.
func PrepareLogs(logName string) error {
	log.SetFormatter(&log.JSONFormatter{})

	if logName == "" {
		log.SetOutput(os.Stdout)
		return nil
	}

	if dir := filepath.Dir(logName); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.WithError(err).Warn("could not create log directory, falling back to stdout")
			log.SetOutput(os.Stdout)
			return nil
		}
	}

	f, err := os.OpenFile(logName, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644) //nolint:gosec // operator-supplied log path
	if err != nil {
		log.WithError(err).Warn("could not open log file, falling back to stdout")
		log.SetOutput(os.Stdout)
		return nil
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	return nil
}
