package httpapi

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/identity"
	"github.com/Tobe0504/Warder/internal/ratelimit"
	"github.com/google/uuid"
)

type contextKey string

const (
	principalKey contextKey = "principal"
	requestIDKey contextKey = "request_id"
)

// PrincipalFrom returns the authenticated principal for a request.
func PrincipalFrom(ctx context.Context) (*domain.Principal, bool) {
	p, ok := ctx.Value(principalKey).(*domain.Principal)
	return p, ok
}

// RequestIDFrom returns the correlation id for a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// middleware is the standard wrapper shape.
type middleware func(http.Handler) http.Handler

// chain applies middleware so that the first listed runs outermost.
func chain(h http.Handler, mw ...middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// withRequestID attaches a correlation id used across log lines.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A client-supplied id is not trusted for correlation; it would let a
		// caller collide their entries with someone else's.
		id := uuid.NewString()
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// withRecovery converts a panic into a generic 500.
//
// A panic message can contain anything that was on the stack, so it is logged
// and never rendered.
func withRecovery(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("handler panicked",
						"method", r.Method, "path", r.URL.Path,
						"request_id", RequestIDFrom(r.Context()),
						"panic", recovered)
					writeJSON(w, r, logger, http.StatusInternalServerError,
						map[string]any{"error": ErrInternal})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// withSecurityHeaders sets the response headers appropriate to a JSON API.
//
// This API serves no HTML and is not meant to be reached by a browser at all —
// the dashboard talks to the BFF, which talks to this. The headers are set
// anyway so that a response rendered somewhere unexpected still carries them.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=()")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")

		// Responses from this API describe or carry credentials. None of it may
		// be retained by any cache on the path.
		h.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		h.Set("Pragma", "no-cache")

		next.ServeHTTP(w, r)
	})
}

// withServiceToken authenticates the caller as the trusted BFF.
//
// This is what enforces the rule that the browser never reaches the core API.
// The dashboard's session cookie alone is not sufficient here: a request must
// also carry the service credential, which lives only on the BFF's server side.
// A browser that has somehow learned this API's address and stolen a session
// still cannot use it.
func withServiceToken(expected string, logger *slog.Logger) middleware {
	expectedBytes := []byte(expected)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The health check is exempt. It answers {"status":"ok"} and
			// nothing else — there is no information in it worth a credential —
			// and every container platform probes an unauthenticated path to
			// decide whether an instance is alive. Requiring the service token
			// here means the platform sees 401, marks the service unhealthy,
			// and refuses to route traffic to a process that is working fine.
			if r.Method == http.MethodGet && r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			presented := []byte(r.Header.Get("X-Service-Token"))

			if subtle.ConstantTimeCompare(presented, expectedBytes) != 1 {
				// Logged at warn, because in a working deployment this never
				// happens: either the BFF is misconfigured or something that is
				// not the BFF is talking to this surface. Both are worth seeing.
				logger.Warn("request rejected: missing or invalid service credential",
					"method", r.Method, "path", r.URL.Path,
					"client_ip", ClientIP(r, false),
					"presented", len(presented) > 0)
				writeError(w, r, logger, ErrServiceUnauthorized, nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// withAuthentication resolves the principal from the presented credential.
func withAuthentication(chainProvider *identity.Chain, trustProxy bool, logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := bearerToken(r)
			if presented == "" {
				writeError(w, r, logger, ErrUnauthorized, nil)
				return
			}

			principal, err := chainProvider.Authenticate(r.Context(), identity.Request{
				Credential: presented,
				Headers:    r.Header,
				ClientIP:   ClientIP(r, trustProxy),
				UserAgent:  r.UserAgent(),
			})
			if err != nil {
				// Every authentication failure looks the same from outside.
				writeError(w, r, logger, ErrUnauthorized, nil)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)))
		})
	}
}

// withRateLimit throttles by a caller-derived key.
func withRateLimit(limiter ratelimit.Limiter, keyFor func(*http.Request) string, logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.Allow(keyFor(r))
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
				writeError(w, r, logger, ErrRateLimited, nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// withMaxBody bounds the request body so a large upload cannot exhaust memory.
func withMaxBody(limit int64) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// withRequestLogging records one line per request.
//
// It logs the method, path, status, and duration, and nothing from the body,
// the query string, or the headers. The logger redacts as well, but the surest
// way not to log a credential is not to hand one to the logger.
func withRequestLogging(logger *slog.Logger, trustProxy bool) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, r)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()),
				"client_ip", ClientIP(r, trustProxy),
			}
			if p, ok := PrincipalFrom(r.Context()); ok {
				attrs = append(attrs,
					"actor_type", string(p.ActorType),
					"actor_id", p.ID.String(),
				)
			}
			logger.Info("request", attrs...)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// bearerToken extracts a credential from the Authorization header.
//
// The header is the only accepted location. A credential in a query string
// would be written to access logs, proxy logs, and browser history, so there is
// no query-parameter fallback anywhere in this API.
func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}

// ClientIP returns the observed client address.
//
// Forwarded headers are honoured only when the deployment states that it sits
// behind a proxy that sets them. Trusting them unconditionally would let any
// caller choose their own rate-limit bucket and forge the address recorded in
// the audit trail.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			// The left-most entry is the original client as recorded by the
			// nearest trusted proxy.
			first, _, _ := strings.Cut(forwarded, ",")
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-Ip")); realIP != "" {
			return realIP
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
