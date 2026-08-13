// Package utils holds small cross-cutting helpers: env expansion, dotenv
// loading and secret resolution.
package utils

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fjacquet/pve_exporter/internal/models"
)

var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)

// ExpandEnv replaces every ${VAR} reference in s with the value of the
// environment variable VAR, returning an error if any referenced variable is
// unset. Strings without references are returned unchanged.
//
// A reference may carry a fallback as ${VAR:-default}, borrowing the shell /
// docker-compose syntax and its meaning: unset OR empty falls back, and the reference
// never errors. That lets a shipped config.yaml drive a non-secret setting from the
// environment while still starting on a host that never exported it. Use it only where a
// safe default exists — a bare ${VAR} keeps the fail-loud behaviour that protects secrets.
func ExpandEnv(s string) (string, error) {
	var missing []string
	out := envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		sub := envRefPattern.FindStringSubmatch(match)
		name, fallback := sub[1], sub[2]
		v, ok := os.LookupEnv(name)
		if ok && v != "" {
			return v
		}
		if fallback != "" {
			return fallback[len(":-"):] // group 2 keeps its ":-" prefix, so "" means absent
		}
		if !ok {
			missing = append(missing, name)
		}
		return ""
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unset environment variable(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// ResolveSecrets expands ${ENV_VAR} references in every cluster's connection
// fields and loads tokenSecret from tokenSecretFile when provided.
func ResolveSecrets(cfg *models.Config) error {
	for i := range cfg.Clusters {
		cl := &cfg.Clusters[i]

		host, err := ExpandEnv(cl.Host)
		if err != nil {
			return fmt.Errorf("clusters[%d] (%s) host: %w", i, cl.Name, err)
		}
		cl.Host = host

		tokenID, err := ExpandEnv(cl.TokenID)
		if err != nil {
			return fmt.Errorf("clusters[%d] (%s) tokenID: %w", i, cl.Name, err)
		}
		cl.TokenID = tokenID

		if cl.TokenSecretFile != "" && cl.TokenSecret == "" {
			b, err := os.ReadFile(cl.TokenSecretFile) //nolint:gosec // operator-supplied secret path
			if err != nil {
				return fmt.Errorf("clusters[%d] (%s) tokenSecretFile: %w", i, cl.Name, err)
			}
			cl.TokenSecret = strings.TrimSpace(string(b))
			continue
		}

		secret, err := ExpandEnv(cl.TokenSecret)
		if err != nil {
			return fmt.Errorf("clusters[%d] (%s) tokenSecret: %w", i, cl.Name, err)
		}
		cl.TokenSecret = secret
	}
	return nil
}
