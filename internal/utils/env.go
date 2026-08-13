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
// safe default exists.
//
// A bare ${VAR} fails when the variable is UNSET; an exported-but-empty one expands to
// the empty string, as it always has. Credential fields get the stricter treatment —
// see ExpandEnvSecret.
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

// ExpandEnvSecret expands like ExpandEnv, but additionally rejects a credential that was
// written as an env reference yet resolves to nothing. A stray `PVE1_PASSWORD=` line in
// a .env file is a plausible typo, and without this the exporter would authenticate with an
// empty credential and report a failure that names the wrong cause.
//
// It fires only when the field actually contains a ${...} reference: a literal value is
// passed through untouched and an omitted optional credential stays omitted, so it cannot
// break a config that never referenced the environment in the first place.
func ExpandEnvSecret(field, s string) (string, error) {
	out, err := ExpandEnv(s)
	if err != nil {
		return "", err
	}
	if out == "" && envRefPattern.MatchString(s) {
		return "", fmt.Errorf("%s references %s, which resolved to an empty value", field, s)
	}
	return out, nil
}

// ResolveSecrets expands ${ENV_VAR} references in every cluster's connection
// fields and loads tokenSecret from tokenSecretFile when provided.
func ResolveSecrets(cfg *models.Config) error {
	for i := range cfg.Clusters {
		cl := &cfg.Clusters[i]

		host, err := ExpandEnvSecret("host", cl.Host)
		if err != nil {
			return fmt.Errorf("clusters[%d] (%s) host: %w", i, cl.Name, err)
		}
		cl.Host = host

		tokenID, err := ExpandEnvSecret("tokenID", cl.TokenID)
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

		secret, err := ExpandEnvSecret("tokenSecret", cl.TokenSecret)
		if err != nil {
			return fmt.Errorf("clusters[%d] (%s) tokenSecret: %w", i, cl.Name, err)
		}
		cl.TokenSecret = secret
	}
	return nil
}
