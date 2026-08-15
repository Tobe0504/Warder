// Package secretvalue carries plaintext secret material through the
// application in a wrapper that refuses to render itself.
//
// Redaction that depends on developers remembering to redact fails eventually.
// This type inverts the default: a plaintext value formats as "[redacted]"
// through fmt, through slog, and through encoding/json, and the only way to get
// the bytes out is to call Expose. That makes every point where plaintext
// leaves the system a single greppable token:
//
//	git grep -n '\.Expose()'
//
// The result should be a short list: the runtime delivery handler, the reveal
// handler, and the tests, and every entry on it should be reviewable.
package secretvalue

import (
	"errors"
	"log/slog"

	"github.com/Tobe0504/Warder/internal/crypto"
)

// Redacted is what a secret value renders as everywhere except Expose.
const Redacted = "[redacted]"

// ErrNotSerializable is returned when something tries to marshal a secret value
// into JSON. Failing loudly is the point: a response that was about to carry
// plaintext becomes a 500 and an alert, rather than a successful response
// nobody inspects.
var ErrNotSerializable = errors.New("secretvalue: refusing to serialize plaintext; call Expose explicitly")

// Value holds plaintext secret material.
//
// It is a struct rather than a named []byte so that it cannot be silently
// converted back to a printable type by an ordinary conversion.
type Value struct {
	b []byte
}

// New wraps plaintext.
func New(b []byte) Value { return Value{b: b} }

// NewString wraps plaintext held in a string.
func NewString(s string) Value { return Value{b: []byte(s)} }

// Expose returns the underlying plaintext.
//
// Every call is a deliberate decision to move plaintext across a boundary, and
// should be accompanied by an audit event.
func (v Value) Expose() []byte { return v.b }

// ExposeString returns the underlying plaintext as a string.
func (v Value) ExposeString() string { return string(v.b) }

// Len reports the length of the value, which is safe to know and is used to
// reject oversized input before encryption.
func (v Value) Len() int { return len(v.b) }

// Empty reports whether the value holds nothing.
func (v Value) Empty() bool { return len(v.b) == 0 }

// Destroy best-effort wipes the underlying buffer. See crypto.Zeroize for the
// limits of this guarantee.
func (v Value) Destroy() { crypto.Zeroize(v.b) }

// String implements fmt.Stringer.
func (v Value) String() string { return Redacted }

// GoString implements fmt.GoStringer, covering the %#v verb.
func (v Value) GoString() string { return Redacted }

// Format covers every remaining fmt verb, including %s, %q, %v, and %x, so
// there is no formatting directive that prints the underlying bytes.
func (v Value) Format(f interface{ Write([]byte) (int, error) }, _ rune) {
	_, _ = f.Write([]byte(Redacted))
}

// LogValue implements slog.LogValuer, so a value passed to a structured logger
// is redacted before it reaches any handler.
func (v Value) LogValue() slog.Value { return slog.StringValue(Redacted) }

// MarshalJSON refuses. Serializing plaintext must be explicit.
func (v Value) MarshalJSON() ([]byte, error) { return nil, ErrNotSerializable }

// MarshalText refuses, covering encoders that prefer TextMarshaler.
func (v Value) MarshalText() ([]byte, error) { return nil, ErrNotSerializable }
