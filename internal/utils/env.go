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

var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ExpandEnv replaces every ${VAR} reference in s with the value of the
// environment variable VAR, returning an error if any referenced variable is
// unset. Strings without references are returned unchanged.
func ExpandEnv(s string) (string, error) {
	var missing []string
	out := envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1]
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return ""
		}
		return v
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
