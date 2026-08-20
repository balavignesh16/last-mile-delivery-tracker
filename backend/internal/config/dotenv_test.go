package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFile_MissingFileIsNotAnError(t *testing.T) {
	if err := LoadDotEnvFile(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Fatalf("expected no error for a missing .env file, got: %v", err)
	}
}

func TestLoadDotEnvFile_SetsMissingVarsOnly(t *testing.T) {
	// Deliberately using names that don't collide with any real variable
	// config.Load() reads, so this test can never leak state into
	// config_test.go's assertions regardless of test execution order.
	const (
		unsetVar  = "LMT_TEST_DOTENV_UNSET_VAR"
		quotedVar = "LMT_TEST_DOTENV_QUOTED_VAR"
		presetVar = "LMT_TEST_DOTENV_PRESET_VAR"
	)
	t.Cleanup(func() {
		os.Unsetenv(unsetVar)
		os.Unsetenv(quotedVar)
	})

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "# comment\n" + unsetVar + "=localhost\n\n" + quotedVar + "=\"lastmile\"\n" + presetVar + "=fromfile\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	t.Setenv(presetVar, "fromenv")

	if err := LoadDotEnvFile(envPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := os.Getenv(unsetVar); got != "localhost" {
		t.Errorf("%s = %q, want localhost", unsetVar, got)
	}
	if got := os.Getenv(quotedVar); got != "lastmile" {
		t.Errorf("%s = %q, want lastmile (quotes should be stripped)", quotedVar, got)
	}
	if got := os.Getenv(presetVar); got != "fromenv" {
		t.Errorf("%s = %q, want fromenv (real env must win over .env file)", presetVar, got)
	}
}

// TestLoadFirstDotEnv_FindsRepoRootFromBackendDir reproduces the exact
// real-world layout: `.env` created at the repository root per the README,
// backend started from cwd=backend/ per the same README's documented
// command. The first candidate (".env") does not exist in backend/; the
// second ("../.env") must be found instead.
func TestLoadFirstDotEnv_FindsRepoRootFromBackendDir(t *testing.T) {
	const marker = "LMT_TEST_FIRSTDOTENV_ROOT_MARKER"
	t.Cleanup(func() { os.Unsetenv(marker) })

	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	if err := os.Mkdir(backendDir, 0o755); err != nil {
		t.Fatalf("failed to create backend dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(marker+"=found-at-root\n"), 0o600); err != nil {
		t.Fatalf("failed to write root .env: %v", err)
	}

	t.Chdir(backendDir)

	loadedPath, err := LoadFirstDotEnv(".env", "../.env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loadedPath != "../.env" {
		t.Errorf("loadedPath = %q, want ../.env", loadedPath)
	}
	if got := os.Getenv(marker); got != "found-at-root" {
		t.Errorf("%s = %q, want found-at-root", marker, got)
	}
}

func TestLoadFirstDotEnv_FirstExistingCandidateWins(t *testing.T) {
	const marker = "LMT_TEST_FIRSTDOTENV_PRIORITY_MARKER"
	t.Cleanup(func() { os.Unsetenv(marker) })

	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	if err := os.Mkdir(backendDir, 0o755); err != nil {
		t.Fatalf("failed to create backend dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(marker+"=from-root\n"), 0o600); err != nil {
		t.Fatalf("failed to write root .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendDir, ".env"), []byte(marker+"=from-backend-dir\n"), 0o600); err != nil {
		t.Fatalf("failed to write backend .env: %v", err)
	}

	t.Chdir(backendDir)

	loadedPath, err := LoadFirstDotEnv(".env", "../.env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loadedPath != ".env" {
		t.Errorf("loadedPath = %q, want .env (nearer candidate should win)", loadedPath)
	}
	if got := os.Getenv(marker); got != "from-backend-dir" {
		t.Errorf("%s = %q, want from-backend-dir", marker, got)
	}
}

func TestLoadFirstDotEnv_NoCandidatesExist(t *testing.T) {
	t.Chdir(t.TempDir())

	loadedPath, err := LoadFirstDotEnv(".env", "../.env")
	if err != nil {
		t.Fatalf("expected no error when no candidate exists, got: %v", err)
	}
	if loadedPath != "" {
		t.Errorf("loadedPath = %q, want empty string", loadedPath)
	}
}
