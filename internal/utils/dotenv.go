package utils

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

// LoadDotEnv loads a .env file (if present) into the process environment before
// secret resolution. It looks for ".env" in the working directory first, then
// next to the config file. Variables already set in the environment win, so the
// .env file only fills gaps. Failure is non-fatal.
func LoadDotEnv(cfgPath string) {
	candidates := []string{".env"}
	if cfgPath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(cfgPath), ".env"))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := godotenv.Load(p); err != nil {
			log.WithField("path", p).WithError(err).Warn("failed to load .env file")
			return
		}
		log.WithField("path", p).Debug("loaded .env file")
		return
	}
}
