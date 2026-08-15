// Package config loads and validates deployment configuration.
//
// Configuration is read from the environment once at startup and validated
// eagerly. A deployment that is missing key material, or that is running in
// production with development defaults, fails to start rather than running in a
// weakened state that nobody notices until it matters.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment distinguishes local development from a real deployment. It gates
// the security posture, not just log verbosity.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
)

// Config is the validated configuration of the core API.
type Config struct {
	Env Environment

	// DatabaseURL is the PostgreSQL connection string used by the running
	// application. It should name a least-privileged role that cannot alter the
	// schema; see deploy/sql/roles.sql.
	DatabaseURL string

	// MigrationDatabaseURL, when set, is used only by the migrate command.
	// Keeping schema change privileges out of the runtime role means a
	// compromised application process cannot drop the audit trigger.
	MigrationDatabaseURL string

	// AdminAddr serves the human-facing API. Only the BFF should be able to
	// reach it.
	AdminAddr string

	// RuntimeAddr serves the machine-to-machine API on a separate listener, so
	// that the two surfaces can be placed on different networks and the BFF can
	// be denied access to secret delivery at the network layer as well as in
	// code.
	RuntimeAddr string

	// AllowPublicRuntimeBind acknowledges that something outside this process
	// decides what can reach the runtime port. Required in production when the
	// listener binds every interface, which is the only option in a container.
	AllowPublicRuntimeBind bool

	// ServiceToken authenticates the BFF to the core API. Requests carrying a
	// user session but no valid service token are refused, which is what stops
	// a browser that has learned the core API's address from calling it
	// directly.
	ServiceToken string

	// Keyring holds the key encryption keys, supplied out of band.
	Keyring          string
	ActiveKeyVersion string

	// SessionTTL bounds a browser session's life.
	SessionTTL time.Duration
	// CLISessionTTL bounds a developer's CLI login.
	CLISessionTTL time.Duration
	// RuntimeSessionTTL bounds the short-lived credential used for the actual
	// secret retrieval call. It is deliberately short: it exists for one
	// process start.
	RuntimeSessionTTL time.Duration

	// TrustProxyHeaders enables reading the client address from X-Forwarded-For.
	// Off by default: a spoofable client address would corrupt both the audit
	// trail and per-address rate limiting.
	TrustProxyHeaders bool
}

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	cfg := &Config{
		Env:                    Environment(getenv("WARDER_ENV", string(EnvDevelopment))),
		DatabaseURL:            os.Getenv("WARDER_DATABASE_URL"),
		MigrationDatabaseURL:   os.Getenv("WARDER_MIGRATION_DATABASE_URL"),
		AdminAddr:              getenv("WARDER_ADMIN_ADDR", "127.0.0.1:8080"),
		RuntimeAddr:            getenv("WARDER_RUNTIME_ADDR", "127.0.0.1:8081"),
		ServiceToken:           os.Getenv("WARDER_SERVICE_TOKEN"),
		Keyring:                os.Getenv("WARDER_KEYRING"),
		ActiveKeyVersion:       getenv("WARDER_ACTIVE_KEY_VERSION", "v1"),
		AllowPublicRuntimeBind: getenv("WARDER_ALLOW_PUBLIC_RUNTIME_BIND", "") == "true",
	}

	var err error
	if cfg.SessionTTL, err = duration("WARDER_SESSION_TTL", 12*time.Hour); err != nil {
		return nil, err
	}
	if cfg.CLISessionTTL, err = duration("WARDER_CLI_SESSION_TTL", 7*24*time.Hour); err != nil {
		return nil, err
	}
	if cfg.RuntimeSessionTTL, err = duration("WARDER_RUNTIME_SESSION_TTL", 5*time.Minute); err != nil {
		return nil, err
	}
	if cfg.TrustProxyHeaders, err = boolean("WARDER_TRUST_PROXY_HEADERS", false); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Production reports whether the deployment is running with production posture.
func (c *Config) Production() bool { return c.Env == EnvProduction }

// MigrationDSN returns the connection string the migrate command should use.
func (c *Config) MigrationDSN() string {
	if c.MigrationDatabaseURL != "" {
		return c.MigrationDatabaseURL
	}
	return c.DatabaseURL
}

func (c *Config) validate() error {
	var problems []string

	switch c.Env {
	case EnvDevelopment, EnvProduction:
	default:
		problems = append(problems, fmt.Sprintf("WARDER_ENV must be %q or %q", EnvDevelopment, EnvProduction))
	}

	if c.DatabaseURL == "" {
		problems = append(problems, "WARDER_DATABASE_URL is required")
	}
	if c.Keyring == "" {
		problems = append(problems, "WARDER_KEYRING is required (run: go run ./cmd/warder-api keygen)")
	}
	if c.ServiceToken == "" {
		problems = append(problems, "WARDER_SERVICE_TOKEN is required")
	}

	// The service token is the only thing standing between a network-adjacent
	// caller and the human-facing API, so a guessable one is refused outright
	// rather than warned about.
	if c.ServiceToken != "" && len(c.ServiceToken) < 32 {
		problems = append(problems, "WARDER_SERVICE_TOKEN must be at least 32 characters")
	}

	if c.RuntimeSessionTTL > time.Hour {
		problems = append(problems, "WARDER_RUNTIME_SESSION_TTL must not exceed 1h; it exists for a single process start")
	}

	if c.Production() {
		// Binding the runtime API to every interface in production is the kind
		// of default that turns a network mistake into secret delivery, so it
		// has to be stated deliberately.
		//
		// "Deliberately" is the operative word, not "never": inside a container
		// there is no specific interface to name, because the address is
		// assigned at start and the platform decides what reaches it. So the
		// requirement is an explicit acknowledgement rather than a prohibition
		// — someone has to have thought about what sits in front of this port.
		bindsEverything := strings.HasPrefix(c.RuntimeAddr, ":") ||
			strings.HasPrefix(c.RuntimeAddr, "0.0.0.0:") ||
			strings.HasPrefix(c.RuntimeAddr, "[::]:")

		if bindsEverything && !c.AllowPublicRuntimeBind {
			problems = append(problems, "WARDER_RUNTIME_ADDR binds every interface. "+
				"Name a specific one, or set WARDER_ALLOW_PUBLIC_RUNTIME_BIND=true if a "+
				"container platform or load balancer controls what reaches this port")
		}
	}

	if len(problems) > 0 {
		return errors.New("configuration is invalid:\n  - " + strings.Join(problems, "\n  - "))
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid duration: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}

func boolean(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s is not a valid boolean", key)
	}
	return v, nil
}
