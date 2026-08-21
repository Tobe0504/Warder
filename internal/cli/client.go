package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotLoggedIn indicates no usable credential is available.
var ErrNotLoggedIn = errors.New("not logged in")

// Client talks to the Warder API.
type Client struct {
	baseURL string
	// source names where baseURL came from, for error messages. Empty means
	// the stored login.
	source string
	http   *http.Client
}

// NewClient constructs a client for a base URL.
func NewClient(baseURL string) *Client {
	return NewClientFrom(baseURL, "")
}

// NewClientFrom constructs a client, recording where the address came from so
// that an unreachable server can say which setting to correct.
func NewClientFrom(baseURL, source string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		source:  source,
		http: &http.Client{
			Timeout: 30 * time.Second,
			// Redirects are not followed. A redirect from the API would send
			// the Authorization header to whatever host the response names,
			// which is a credential-exfiltration primitive handed to anyone who
			// can influence a response.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// apiError is the error shape the server returns.
type apiError struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	} `json:"error"`
}

// post sends an authenticated JSON request.
//
// The credential travels in the Authorization header and nowhere else: not in
// the URL, not in a query parameter, not in the body. URLs are written to
// access logs, proxy logs, and shell history.
func (c *Client) post(ctx context.Context, path, credential string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, credential, body, out)
}

func (c *Client) get(ctx context.Context, path, credential string, out any) error {
	return c.do(ctx, http.MethodGet, path, credential, nil, out)
}

func (c *Client) do(ctx context.Context, method, path, credential string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("could not encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("could not build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "warder-cli/"+Version)

	resp, err := c.http.Do(req)
	if err != nil {
		// The transport error is not wrapped in: it can quote the request URL,
		// which may carry userinfo, and it says nothing a reader can act on
		// that the typed error below does not say better.
		return &UnreachableError{
			BaseURL:  redactURL(c.baseURL),
			Source:   c.source,
			Loopback: isLoopbackURL(c.baseURL),
		}
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("could not read the response")
	}

	if resp.StatusCode >= 400 {
		var apiErr apiError
		if err := json.Unmarshal(payload, &apiErr); err == nil && apiErr.Error.Message != "" {
			message := apiErr.Error.Message
			for field, detail := range apiErr.Error.Details {
				message += fmt.Sprintf("\n  %s: %s", field, detail)
			}
			if resp.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("%s (run `ward login`)", message)
			}
			return errors.New(message)
		}
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("could not read the response")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Typed calls
// ---------------------------------------------------------------------------

type loginResult struct {
	SessionToken string `json:"sessionToken"`
	ExpiresAt    string `json:"expiresAt"`
	User         struct {
		Email        string `json:"email"`
		Name         string `json:"name"`
		Organization string `json:"organization"`
	} `json:"user"`
}

// Login exchanges an email and password for a CLI session.
func (c *Client) Login(ctx context.Context, email, password string) (*loginResult, error) {
	var result loginResult
	err := c.post(ctx, "/cli/login", "", map[string]string{
		"email":    email,
		"password": password,
		"kind":     "cli",
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RuntimeAuthResult is the response from the credential exchange.
type RuntimeAuthResult struct {
	AccessToken  string   `json:"accessToken"`
	ExpiresAt    string   `json:"expiresAt"`
	Project      string   `json:"project"`
	Environment  string   `json:"environment"`
	Identity     string   `json:"identity"`
	ActorType    string   `json:"actorType"`
	Capabilities []string `json:"capabilities"`
}

// RuntimeAuth exchanges a long-lived credential for a short-lived session.
func (c *Client) RuntimeAuth(ctx context.Context, credential, project, environment string) (*RuntimeAuthResult, error) {
	body := map[string]string{}
	if project != "" {
		body["project"] = project
	}
	if environment != "" {
		body["environment"] = environment
	}

	var result RuntimeAuthResult
	if err := c.post(ctx, "/runtime/auth", credential, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SecretsResult is a delivery of secret values.
type SecretsResult struct {
	Environment string            `json:"environment"`
	Secrets     map[string]string `json:"secrets"`
	Denied      []string          `json:"denied"`
	Unavailable []string          `json:"unavailable"`
}

// FetchSecrets retrieves values using a runtime session.
func (c *Client) FetchSecrets(ctx context.Context, accessToken string, keys []string) (*SecretsResult, error) {
	body := map[string]any{}
	if len(keys) > 0 {
		body["keys"] = keys
	}

	var result SecretsResult
	if err := c.post(ctx, "/runtime/secrets", accessToken, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ProjectListResult is the project listing.
type ProjectListResult struct {
	Projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"projects"`
}

// ListProjects returns the projects visible to the logged-in user.
func (c *Client) ListProjects(ctx context.Context, sessionToken string) (*ProjectListResult, error) {
	var result ProjectListResult
	if err := c.get(ctx, "/cli/projects", sessionToken, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EnvironmentListResult is the environment listing for a project.
type EnvironmentListResult struct {
	Environments []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"environments"`
}

// ListEnvironments returns a project's environments.
func (c *Client) ListEnvironments(ctx context.Context, sessionToken, projectID string) (*EnvironmentListResult, error) {
	var result EnvironmentListResult
	if err := c.get(ctx, "/cli/projects/"+projectID+"/environments", sessionToken, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SecretListResult is the secret metadata listing.
//
// Note what this type does not have: a field for a value. Listing secrets from
// the CLI shows names, versions, and expiry. Seeing a value is a different
// operation with a different capability behind it, and it is not something the
// CLI offers at all.
type SecretListResult struct {
	Environment struct {
		Slug string `json:"slug"`
	} `json:"environment"`
	Secrets []struct {
		Key       string `json:"key"`
		Masked    string `json:"masked"`
		Status    string `json:"status"`
		Version   *int   `json:"version"`
		ExpiresAt string `json:"expiresAt"`
		CanUse    bool   `json:"canUse"`
	} `json:"secrets"`
}

// ListSecrets returns secret metadata for an environment.
func (c *Client) ListSecrets(ctx context.Context, sessionToken, environmentID string) (*SecretListResult, error) {
	var result SecretListResult
	if err := c.get(ctx, "/cli/environments/"+environmentID+"/secrets", sessionToken, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
