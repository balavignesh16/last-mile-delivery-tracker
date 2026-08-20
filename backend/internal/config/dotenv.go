package config

import (
	"os"
	"strings"
)

// LoadDotEnvFile applies KEY=VALUE pairs from a .env-style file into the
// process environment, without overwriting variables already set. It is a
// convenience for running the backend directly on the host (go run) against
// a dockerized database; Docker Compose supplies environment variables on
// its own and does not depend on this function.
//
// A missing file is not an error — the common case is running under Compose
// or a CI environment where variables are already set some other way.
//
// This exists so the project does not need a third-party .env-parsing
// dependency for what is a ~20 line problem.
func LoadDotEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)

		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}

	return nil
}

// LoadFirstDotEnv tries each candidate path in order and loads the first
// one that actually exists on disk, returning the path it loaded ("" if
// none of the candidates exist — not an error, same convention as
// LoadDotEnvFile).
//
// This exists because the process's working directory depends on how the
// backend is started, and the project's own documented commands don't
// agree on one: `cd backend && go run ./cmd/server` runs with cwd=backend/,
// while `.env` is created at the repository root one level up. Rather than
// guess a single relative path, the caller lists every directory a
// documented command might actually be run from.
func LoadFirstDotEnv(candidates ...string) (loadedPath string, err error) {
	for _, path := range candidates {
		if _, statErr := os.Stat(path); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return "", statErr
		}
		if err := LoadDotEnvFile(path); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", nil
}
