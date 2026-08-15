package apitest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tobe0504/Warder/internal/apitest"
)

// buildCLI compiles the real ward binary once per test binary run.
func buildCLI(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "ward")
	cmd := exec.Command("go", "build", "-o", binary, "github.com/Tobe0504/Warder/cmd/ward")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI: %v\n%s", err, output)
	}
	return binary
}

// TestCLIRunInjectsSecretsIntoAChildProcess exercises the actual binary against
// the actual API: a machine token in the environment, `ward run`, and a child
// process that reports whether it received the credential.
//
// This is the claim the product is sold on, checked end to end rather than
// asserted about.
func TestCLIRunInjectsSecretsIntoAChildProcess(t *testing.T) {
	h := apitest.New(t)
	binary := buildCLI(t)

	const value = "postgres://app:cli-e2e-canary@db.internal:5432/app"

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", value)
	h.NewSecret(org, project.DevelopmentID, "REDIS_URL", "redis://localhost:6379")

	identityID := h.NewIdentity(org, apitest.Unique("payments-api"), "WORKLOAD")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	// The child compares rather than prints, so the value never reaches the
	// test output — the same discipline the CLI itself follows.
	script := `
		if [ "$DATABASE_URL" = "` + value + `" ]; then echo DATABASE_URL_OK; else echo DATABASE_URL_MISSING; fi
		if [ -n "$REDIS_URL" ]; then echo REDIS_URL_OK; else echo REDIS_URL_MISSING; fi
		if [ -z "$WARDER_TOKEN" ]; then echo TOKEN_WITHHELD; else echo TOKEN_LEAKED; fi
	`

	cmd := exec.Command(binary, "run", "--project", project.Slug, "--env", "development", "--", "sh", "-c", script)
	cmd.Env = append(os.Environ(),
		"WARDER_RUNTIME_URL="+h.Runtime.URL,
		"WARDER_TOKEN="+token.Secret,
		"WARDER_CONFIG_DIR="+t.TempDir(),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ward run failed: %v\n%s", err, output)
	}
	combined := string(output)

	if !strings.Contains(combined, "DATABASE_URL_OK") {
		t.Fatalf("the child process did not receive DATABASE_URL:\n%s", combined)
	}
	if !strings.Contains(combined, "REDIS_URL_OK") {
		t.Fatalf("the child process did not receive REDIS_URL:\n%s", combined)
	}

	// The runtime credential must not be passed down. The child needs the
	// secrets, not the ability to ask the broker for more of them.
	if !strings.Contains(combined, "TOKEN_WITHHELD") {
		t.Fatalf("the child process inherited WARDER_TOKEN:\n%s", combined)
	}

	// Nothing the CLI printed contains a value.
	if strings.Contains(combined, value) || strings.Contains(combined, "cli-e2e-canary") {
		t.Fatalf("the CLI printed a secret value:\n%s", combined)
	}

	// It does report what it injected, by name, which is what makes it usable.
	if !strings.Contains(combined, "DATABASE_URL") || !strings.Contains(combined, "2 secret(s) injected") {
		t.Fatalf("the CLI should summarise what it injected:\n%s", combined)
	}
}

// The CLI must never write secrets to disk, and specifically never to a .env
// file, which is the habit the product exists to replace.
func TestCLIWritesNothingToDisk(t *testing.T) {
	h := apitest.New(t)
	binary := buildCLI(t)

	const value = "sk_live_disk_canary_value"

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "STRIPE_SECRET_KEY", value)

	identityID := h.NewIdentity(org, apitest.Unique("worker"), "SERVICE")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID, []string{"USE_SECRET"}, nil)

	workDir := t.TempDir()
	configDir := t.TempDir()

	cmd := exec.Command(binary, "run", "--project", project.Slug, "--env", "development", "--", "sh", "-c", "true")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"WARDER_RUNTIME_URL="+h.Runtime.URL,
		"WARDER_TOKEN="+token.Secret,
		"WARDER_CONFIG_DIR="+configDir,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ward run failed: %v\n%s", err, output)
	}

	// Nothing was created in the working directory at all — no .env, no cache,
	// no temporary file.
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the CLI created files in the working directory: %v", names)
	}

	// And nothing under the config directory contains the value.
	assertNoValueOnDisk(t, configDir, value)
	assertNoValueOnDisk(t, workDir, value)
}

func assertNoValueOnDisk(t *testing.T, root, value string) {
	t.Helper()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(contents), value) {
			t.Fatalf("a secret value was written to %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// A developer logging in through the CLI gets a credential file only they can
// read, and the file holds a session rather than any secret value.
func TestCLILoginStoresACredentialSafely(t *testing.T) {
	h := apitest.New(t)
	binary := buildCLI(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "DATABASE_URL", "login-canary-value-not-real")

	configDir := t.TempDir()

	login := exec.Command(binary, "login", "--url", h.Runtime.URL, "--email", org.Email)
	login.Env = append(os.Environ(), "WARDER_CONFIG_DIR="+configDir)
	login.Stdin = strings.NewReader("correct-horse-battery-staple-test\n")

	if output, err := login.CombinedOutput(); err != nil {
		t.Fatalf("ward login failed: %v\n%s", err, output)
	}

	credentialsFile := filepath.Join(configDir, "credentials.json")
	info, err := os.Stat(credentialsFile)
	if err != nil {
		t.Fatalf("the credential file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the credential file is mode %04o, want 0600", perm)
	}

	stored, err := os.ReadFile(credentialsFile)
	if err != nil {
		t.Fatalf("reading the credential file: %v", err)
	}
	if strings.Contains(string(stored), "login-canary-value-not-real") {
		t.Fatal("the credential file contains a secret value")
	}
	if strings.Contains(string(stored), "correct-horse-battery-staple-test") {
		t.Fatal("the credential file contains the password")
	}
	if !strings.Contains(string(stored), "vsn_") {
		t.Fatal("the credential file should hold a session token")
	}

	// The stored login then works for running something.
	run := exec.Command(binary, "run", "--project", project.Slug, "--env", "development", "--",
		"sh", "-c", `if [ -n "$DATABASE_URL" ]; then echo GOT_IT; fi`)
	run.Env = append(os.Environ(), "WARDER_CONFIG_DIR="+configDir)

	output, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("ward run with a stored login failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "GOT_IT") {
		t.Fatalf("the developer's login did not deliver the secret:\n%s", output)
	}
	if strings.Contains(string(output), "login-canary-value-not-real") {
		t.Fatalf("the CLI printed the value:\n%s", output)
	}
}

// Asking for a secret the caller is not authorized for must stop the command
// rather than starting a process that will fail confusingly later.
func TestCLIRefusesToStartWhenARequestedSecretIsDenied(t *testing.T) {
	h := apitest.New(t)
	binary := buildCLI(t)

	org := h.NewOrganization()
	project := h.NewProject(org)
	h.NewSecret(org, project.DevelopmentID, "ALLOWED", "allowed-value")

	identityID := h.NewIdentity(org, apitest.Unique("narrow"), "SERVICE")
	h.Grant(org, project.ID, project.DevelopmentID, "MACHINE", identityID, []string{"USE_SECRET"}, nil)
	token := h.NewToken(org, project.ID, project.DevelopmentID, identityID,
		[]string{"USE_SECRET"}, []string{"ALLOWED"})

	cmd := exec.Command(binary, "run",
		"--project", project.Slug, "--env", "development",
		"--key", "ALLOWED", "--key", "NOT_GRANTED",
		"--", "sh", "-c", "echo SHOULD_NOT_RUN")
	cmd.Env = append(os.Environ(),
		"WARDER_RUNTIME_URL="+h.Runtime.URL,
		"WARDER_TOKEN="+token.Secret,
		"WARDER_CONFIG_DIR="+t.TempDir(),
	)

	output, _ := cmd.CombinedOutput()
	combined := string(output)

	if strings.Contains(combined, "SHOULD_NOT_RUN") {
		t.Fatalf("the command started despite a denied secret:\n%s", combined)
	}
	if !strings.Contains(combined, "not authorized") {
		t.Fatalf("the CLI should say which keys were refused:\n%s", combined)
	}
}
