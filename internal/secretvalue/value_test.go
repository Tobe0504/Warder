package secretvalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const canary = "sk_live_canary_do_not_print_me"

func TestEveryFormatVerbRedacts(t *testing.T) {
	v := NewString(canary)

	verbs := []string{"%s", "%q", "%v", "%+v", "%#v", "%x", "%X", "%d", "%08s"}
	for _, verb := range verbs {
		out := fmt.Sprintf(verb, v)
		if strings.Contains(out, canary) {
			t.Fatalf("verb %s printed the plaintext: %q", verb, out)
		}
	}

	// Also inside a wrapping struct, which is how it usually reaches a log line.
	type wrapper struct {
		Key   string
		Value Value
	}
	out := fmt.Sprintf("%+v", wrapper{Key: "DATABASE_URL", Value: v})
	if strings.Contains(out, canary) {
		t.Fatalf("plaintext printed through a struct: %q", out)
	}
	if !strings.Contains(out, "DATABASE_URL") {
		t.Fatal("the key should still be visible; only the value is secret")
	}
}

func TestStructuredLoggingRedacts(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	v := NewString(canary)
	logger.Info("delivering secret", "key", "DATABASE_URL", "value", v)
	logger.Info("in a group", slog.Group("secret", "key", "STRIPE_SECRET_KEY", "value", v))

	if strings.Contains(buf.String(), canary) {
		t.Fatalf("the logger emitted plaintext: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "DATABASE_URL") {
		t.Fatal("the key should survive redaction; it is what makes the log useful")
	}
}

// An accidental include in a response body must fail the request rather than
// succeed quietly.
func TestJSONSerializationRefuses(t *testing.T) {
	type response struct {
		Key   string `json:"key"`
		Value Value  `json:"value"`
	}

	_, err := json.Marshal(response{Key: "DATABASE_URL", Value: NewString(canary)})
	if err == nil {
		t.Fatal("plaintext was serialized into JSON")
	}
	if !errors.Is(err, ErrNotSerializable) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// json.Encoder buffers a value before writing, so a refused marshal must leave
// nothing at all on the wire rather than a truncated object.
func TestRefusedEncodeWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(struct {
		Value Value `json:"value"`
	}{NewString(canary)})

	if err == nil {
		t.Fatal("encoding succeeded")
	}
	if buf.Len() != 0 {
		t.Fatalf("a refused encode wrote %q", buf.String())
	}
}

func TestExposeReturnsPlaintext(t *testing.T) {
	v := NewString(canary)

	if got := v.ExposeString(); got != canary {
		t.Fatalf("Expose returned %q", got)
	}
	if !bytes.Equal(v.Expose(), []byte(canary)) {
		t.Fatal("Expose returned the wrong bytes")
	}
	if v.Len() != len(canary) {
		t.Fatalf("Len returned %d", v.Len())
	}
}

func TestDestroy(t *testing.T) {
	buf := []byte(canary)
	v := New(buf)
	v.Destroy()

	if bytes.Contains(buf, []byte("sk_live")) {
		t.Fatal("Destroy left the plaintext in the buffer")
	}
}

func TestEmpty(t *testing.T) {
	if !NewString("").Empty() {
		t.Fatal("an empty value should report empty")
	}
	if NewString("x").Empty() {
		t.Fatal("a non-empty value reported empty")
	}
}
