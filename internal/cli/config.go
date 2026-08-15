// Package cli implements the ward command.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Credentials is the CLI's stored login.
//
// The session token here is a credential, and it is stored the way an SSH
// private key is: in the user's home directory, in a file only they can read.
// It is deliberately not placed in the working tree, where it would be one
// mistaken `git add` away from a repository, and not in an environment variable
// exported from a shell profile, where every process the developer runs, every
// dependency, every AI agent, would inherit it.
type Credentials struct {
	APIURL       string    `json:"apiUrl"`
	RuntimeURL   string    `json:"runtimeUrl"`
	SessionToken string    `json:"sessionToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Email        string    `json:"email"`
	Organization string    `json:"organization"`
}

// Expired reports whether the stored login has lapsed.
func (c *Credentials) Expired(now time.Time) bool {
	return c.ExpiresAt.IsZero() || !c.ExpiresAt.After(now)
}

const (
	credentialsDirName  = ".warder"
	credentialsFileName = "credentials.json"

	// dirPerm and filePerm keep the credential readable only by its owner.
	dirPerm  os.FileMode = 0o700
	filePerm os.FileMode = 0o600
)

// credentialsPath returns the location of the stored login.
func credentialsPath() (string, error) {
	if override := os.Getenv("WARDER_CONFIG_DIR"); override != "" {
		return filepath.Join(override, credentialsFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine your home directory: %w", err)
	}
	return filepath.Join(home, credentialsDirName, credentialsFileName), nil
}

// LoadCredentials reads the stored login.
func LoadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotLoggedIn
		}
		return nil, fmt.Errorf("could not read stored credentials: %w", err)
	}

	// A credential file that has become group- or world-readable is a real
	// problem, and silently continuing would leave the developer believing it
	// is protected. On Windows these bits are not meaningful, so the check is
	// skipped there rather than producing a warning nobody can act on.
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err == nil {
			if perm := info.Mode().Perm(); perm&0o077 != 0 {
				fmt.Fprintf(os.Stderr,
					"warning: %s is readable by other users (mode %04o). Run: chmod 600 %s\n",
					path, perm, path)
			}
		}
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("stored credentials are not readable; run `ward login` again")
	}
	return &creds, nil
}

// SaveCredentials writes the stored login with restrictive permissions.
func SaveCredentials(creds *Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("could not create the configuration directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode credentials: %w", err)
	}

	// Written to a temporary file and renamed, so that an interrupted write
	// cannot leave a truncated credential file behind. The temporary file is
	// created with the final permissions rather than being relaxed and then
	// tightened, so there is no window in which it is world-readable.
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, filePerm); err != nil {
		return fmt.Errorf("could not write credentials: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("could not save credentials: %w", err)
	}
	return nil
}

// ClearCredentials removes the stored login.
func ClearCredentials() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove stored credentials: %w", err)
	}
	return nil
}

// ProjectConfig is the per-directory configuration a developer commits, naming
// which project and environment this working tree runs against.
//
// It contains no credential. That is the point of the file: it can be committed
// so a team shares the same target, while the credential stays out of the
// repository entirely.
type ProjectConfig struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	RuntimeURL  string `json:"runtimeUrl,omitempty"`
}

// LoadProjectConfig reads .warder.json from the working directory, walking up
// toward the root the way version control tools do.
func LoadProjectConfig() (*ProjectConfig, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	for {
		path := filepath.Join(dir, ".warder.json")
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg ProjectConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("%s is not valid JSON", path)
			}
			if err := assertNoSecrets(path, data); err != nil {
				return nil, err
			}
			return &cfg, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// assertNoSecrets refuses a project file that contains a credential.
//
// This file is meant to be committed. Someone will eventually try to put a
// token in it, and the moment that works it becomes the thing the product
// exists to prevent, a credential in the repository. Failing here, with an
// explanation, is better than accepting it.
func assertNoSecrets(path string, data []byte) error {
	lowered := strings.ToLower(string(data))
	for _, marker := range []string{"vlt_", "token", "secret", "password"} {
		if strings.Contains(lowered, marker) {
			return fmt.Errorf(
				"%s appears to contain a credential (%q).\n"+
					"This file is meant to be committed and must hold only the project and environment names.\n"+
					"Run `ward login`, or set WARDER_TOKEN in the runtime's own environment instead",
				path, marker)
		}
	}
	return nil
}
