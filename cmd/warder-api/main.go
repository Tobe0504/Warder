// Command warder-api runs the Warder core API.
//
// It serves two independent listeners:
//
//	admin    the human-facing API, reachable only by the Next.js BFF
//	runtime  the machine-to-machine API that delivers secrets
//
// They are separate servers on separate addresses so that a deployment can put
// them on different networks, and so that no routing mistake in one can expose
// the other.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/authz"
	"github.com/Tobe0504/Warder/internal/config"
	"github.com/Tobe0504/Warder/internal/credential"
	"github.com/Tobe0504/Warder/internal/crypto"
	"github.com/Tobe0504/Warder/internal/httpapi"
	"github.com/Tobe0504/Warder/internal/logging"
	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/store"
)

func main() {
	// No default command. Someone who has just installed this and typed its
	// name should be told what it can do, not shown three errors about
	// environment variables for a `serve` they did not ask for.
	if len(os.Args) < 2 {
		usage()
		return
	}
	command := os.Args[1]

	var err error
	switch command {
	case "serve":
		err = serve()
	case "migrate":
		err = migrate()
	case "keygen":
		err = keygen()
	case "init":
		err = initConfig()
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		err = fmt.Errorf("unknown command %q", command)
	}

	if err != nil {
		// Startup errors go to stderr plainly. They are read by a person at a
		// terminal, and config.Load has already ensured they name what is
		// missing without quoting any value.
		fmt.Fprintf(os.Stderr, "warder-api: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `warder-api: Warder core API

Usage:
  warder-api init      Generate a complete starting configuration
  warder-api serve     Run the admin and runtime API servers
  warder-api migrate   Apply database migrations and exit
  warder-api keygen    Print a fresh key encryption key and exit

Configuration is read from the environment. See .env.example.
`)
}

// keygen prints a new key encryption key for an operator to place in their
// secret manager.
//
// It writes to stdout and nowhere else: not to a file, not to a log. Whoever
// runs it is responsible for where it goes next, and the instructions say so.
func keygen() error {
	key, err := crypto.GenerateKEK()
	if err != nil {
		return err
	}

	fmt.Printf("v1:%s\n", base64.StdEncoding.EncodeToString(key))
	fmt.Fprint(os.Stderr, `
Store this in your secret manager and supply it as WARDER_KEYRING.

  - Do not commit it. Do not put it in the database.
  - Losing it means every secret encrypted under it is unrecoverable.
  - To rotate, add a new version (v2:...) alongside the old one and set
    WARDER_ACTIVE_KEY_VERSION=v2. Existing ciphertext keeps decrypting under
    the version it was written with.
`)
	return nil
}

// initConfig generates a complete, consistent starting configuration.
//
// Doing this by hand means generating a key, generating a service token, and
// then assembling a connection URI that has to agree with both, three chances
// to make a mistake that only shows up as a confusing failure later. This emits
// all of it at once, already consistent.
//
// It writes to stdout so the output can be redirected, with the explanation on
// stderr so that redirecting produces a usable file rather than prose.
func initConfig() error {
	key, err := crypto.GenerateKEK()
	if err != nil {
		return err
	}

	serviceToken, err := credential.Mint(credential.KindSession)
	if err != nil {
		return err
	}
	// Only the random half is used. The kind prefix and public handle are for
	// credentials that get looked up in a database; this one is compared
	// directly, so it is simply a long random string.
	token := serviceToken.Secret

	adminAddr := getenv("WARDER_ADMIN_ADDR", "127.0.0.1:8080")

	fmt.Printf(`# ---------------------------------------------------------------------------
# Core API: save as .env in the repository root
# ---------------------------------------------------------------------------
WARDER_ENV=development
WARDER_DATABASE_URL=postgres://warder:warder-local-dev-only@127.0.0.1:5432/warder?sslmode=disable
WARDER_ADMIN_ADDR=%s
WARDER_RUNTIME_ADDR=127.0.0.1:8081
WARDER_KEYRING=v1:%s
WARDER_ACTIVE_KEY_VERSION=v1
WARDER_SERVICE_TOKEN=%s

# ---------------------------------------------------------------------------
# Dashboard: save as web/.env.local
# ---------------------------------------------------------------------------
# One variable. It carries the service token, the core API address, and the
# deployment posture together, so they cannot drift apart.
WARDER_URL=warder+insecure://%s@%s/development
`, adminAddr, base64.StdEncoding.EncodeToString(key), token, token, adminAddr)

	fmt.Fprint(os.Stderr, `
Two files, generated together so they agree.

  1. Save the first block as .env in the repository root.
  2. Save the second block as web/.env.local.
  3. Both are git-ignored. Neither should ever be committed.

WARNING: this generates a NEW encryption key, named v1.

  Run it only on a deployment with no secrets stored yet. If secrets already
  exist, they were sealed with a different key that also called itself v1, and
  replacing it makes every one of them permanently unreadable. Because the
  version name collides, the failure reports as "authentication failed" rather
  than as a missing key, which reads like tampering rather than like the
  configuration mistake it is.

  To add a key to a deployment that already has secrets, do not use this
  command. Generate one with 'keygen', append it under a new version name, and
  point WARDER_ACTIVE_KEY_VERSION at it:

      WARDER_KEYRING=v1:<existing>,v2:<new>
      WARDER_ACTIVE_KEY_VERSION=v2

  Existing ciphertext keeps decrypting under v1. Never reuse a version name for
  a different key.

About the keyring: losing it means every secret encrypted under it is
unrecoverable. For anything beyond local development, put it in a KMS:
see docs/security/key-management.md.

For production, the dashboard's URL changes shape:

  WARDER_URL=warder://<token>@warder-api.internal:8443/production?origin=https://vault.example.com

The warder:// scheme requires TLS. Using warder+insecure:// with /production
is refused at startup rather than silently sending the token in the clear.
`)
	return nil
}

func migrate() error {
	// Only the database URL. A migration does not need the keyring, and asking
	// for it would put that value in one more shell than necessary.
	dsn, err := config.MigrationDSN()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	applied, err := store.Migrate(ctx, dsn)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		fmt.Println("Database is up to date.")
		return nil
	}
	for _, name := range applied {
		fmt.Printf("applied %s\n", name)
	}
	return nil
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(logging.Options{
		Level: logging.ParseLevel(os.Getenv("WARDER_LOG_LEVEL")),
		JSON:  cfg.Production(),
	})

	// Key material is loaded before anything else. A deployment that cannot
	// build its keyring must not start and begin accepting writes it will be
	// unable to decrypt later.
	keys, err := crypto.ParseKeyring(cfg.Keyring)
	if err != nil {
		return err
	}
	keyProvider, err := crypto.NewLocalKeyringProvider(keys, cfg.ActiveKeyVersion)
	if err != nil {
		return err
	}
	encryption := crypto.NewEnvelopeEncryptionService(keyProvider)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	accounts := store.NewAccountRepo(db)
	projects := store.NewProjectRepo(db)
	secretRepo := store.NewSecretRepo(db)
	machines := store.NewMachineRepo(db)
	grants := store.NewGrantRepo(db)
	auditRepo := store.NewAuditRepo(db)

	recorder := audit.NewDBRecorder(db.Pool, logger)
	policy := authz.NewEngine(grants, time.Now)

	secretService := secrets.NewService(secrets.Config{
		DB:      db,
		Secrets: secretRepo,
		Crypto:  encryption,
		Policy:  policy,
		Audit:   recorder,
		Logger:  logger,
	})

	server := httpapi.New(httpapi.Deps{
		Config:     cfg,
		Logger:     logger,
		DB:         db,
		Accounts:   accounts,
		Projects:   projects,
		Machines:   machines,
		Grants:     grants,
		Audit:      auditRepo,
		SecretRepo: secretRepo,
		Secrets:    secretService,
		Policy:     policy,
		Recorder:   recorder,
		Crypto:     encryption,
	})

	logger.Info("starting warder-api",
		"environment", string(cfg.Env),
		"admin_addr", cfg.AdminAddr,
		"runtime_addr", cfg.RuntimeAddr,
		// The provider description names key versions, never key material.
		"key_provider", keyProvider.Describe(),
	)

	adminServer := newHTTPServer(cfg.AdminAddr, server.AdminHandler())
	runtimeServer := newHTTPServer(cfg.RuntimeAddr, server.RuntimeHandler())

	errs := make(chan error, 2)
	go func() { errs <- listen(logger, "admin", adminServer) }()
	go func() { errs <- listen(logger, "runtime", runtimeServer) }()

	// Expired runtime sessions are already refused on presentation. Purging
	// them keeps the table from growing without bound.
	go purgeExpiredSessions(ctx, machines, logger)

	select {
	case err := <-errs:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_ = adminServer.Shutdown(shutdownCtx)
	_ = runtimeServer.Shutdown(shutdownCtx)
	logger.Info("stopped")
	return nil
}

// newHTTPServer applies timeouts to every listener.
//
// A server without them will hold a connection open indefinitely for a client
// that stops sending, which is a denial-of-service vector that costs nothing to
// exploit.
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
}

func listen(logger *slog.Logger, name string, server *http.Server) error {
	logger.Info("listening", "surface", name, "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s listener: %w", name, err)
	}
	return nil
}

func purgeExpiredSessions(ctx context.Context, machines *store.MachineRepo, logger *slog.Logger) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			removed, err := machines.PurgeExpiredRuntimeSessions(ctx, time.Hour)
			if err != nil {
				logger.Warn("could not purge expired runtime sessions", "error", err)
				continue
			}
			if removed > 0 {
				logger.Info("purged expired runtime sessions", "count", removed)
			}
		case <-ctx.Done():
			return
		}
	}
}

// getenv reads an environment variable with a fallback, for the init command
// which runs before configuration is loaded.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
