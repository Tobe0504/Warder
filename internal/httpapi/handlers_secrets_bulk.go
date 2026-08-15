package httpapi

import (
	"fmt"
	"net/http"

	"github.com/Tobe0504/Warder/internal/secrets"
	"github.com/Tobe0504/Warder/internal/secretvalue"
)

// maxSecretsPerBatch bounds one import.
//
// A .env file with more than this in it is not a configuration, it is a
// mistake — most likely a whole file pasted into the wrong box. The limit also
// bounds how much plaintext one request can hold in memory at once.
const maxSecretsPerBatch = 100

type bulkSecretEntry struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

type createSecretsRequest struct {
	Secrets []bulkSecretEntry `json:"secrets"`
	// ExpiresAt applies to every secret in the batch. Per-key expiry is a
	// deliberate, one-at-a-time decision; a bulk import is not the place to
	// make twenty of them.
	ExpiresAt string `json:"expiresAt"`
}

// handleCreateSecrets stores several secrets in one transaction.
//
// This exists because the alternative — the browser looping over the
// single-secret endpoint — fails badly in exactly the case it is needed. Twenty
// requests means twenty chances to be interrupted, a rate limit that trips
// partway through, and an environment left holding half a configuration with no
// record of which half.
func (s *Server) handleCreateSecrets(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	environmentID, valid := pathUUID(r, "environmentID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}

	environment, err := s.projects.GetEnvironment(r.Context(), principal.OrganizationID, environmentID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var req createSecretsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	if len(req.Secrets) == 0 {
		v.add("secrets", "Add at least one secret.")
	}
	if len(req.Secrets) > maxSecretsPerBatch {
		v.add("secrets", fmt.Sprintf("Import at most %d secrets at a time.", maxSecretsPerBatch))
	}
	expiresAt := v.futureTime("expiresAt", req.ExpiresAt, s.now())
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	// Which keys the environment already has, so an overlapping paste can name
	// them rather than failing on a database constraint the interface would
	// have to guess at. Adding a value to an existing key is a rotation, and
	// rotation is a separate, deliberate act.
	existing := make(map[string]bool)
	if current, err := s.secretRepo.ListSecrets(r.Context(), principal.OrganizationID, environmentID); err == nil {
		for _, summary := range current {
			existing[summary.Key] = true
		}
	}

	// Everything is validated before anything is created, so a bad key at the
	// end of a paste does not leave the first nineteen stored.
	seen := make(map[string]bool, len(req.Secrets))
	requests := make([]secrets.CreateRequest, 0, len(req.Secrets))
	values := make([]secretvalue.Value, 0, len(req.Secrets))

	// Plaintext is wrapped the moment it is read and destroyed on every exit
	// path, including the validation failures below.
	defer func() {
		for _, value := range values {
			value.Destroy()
		}
	}()

	for i, entry := range req.Secrets {
		field := fmt.Sprintf("secrets.%d.key", i)

		key := v.requireSecretKey(field, entry.Key)
		description := v.optionalText(fmt.Sprintf("secrets.%d.description", i), entry.Description, maxDescriptionLength)

		if entry.Value == "" {
			v.add(fmt.Sprintf("secrets.%d.value", i), "A value is required.")
		}
		if key != "" && seen[key] {
			// Two rows naming the same key is ambiguous, and silently letting
			// the last one win is the kind of quiet behaviour that loses a
			// value somebody meant to set.
			v.add(field, "This key appears twice in the batch.")
		}
		if key != "" && existing[key] {
			v.add(field, "This environment already has that key. Rotate it instead of adding it again.")
		}
		seen[key] = true

		value := secretvalue.NewString(entry.Value)
		values = append(values, value)

		requests = append(requests, secrets.CreateRequest{
			OrganizationID: principal.OrganizationID,
			ProjectID:      environment.ProjectID,
			EnvironmentID:  environmentID,
			Key:            key,
			Description:    description,
			Value:          value,
			ExpiresAt:      expiresAt,
		})
	}

	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	created, err := s.secrets.CreateMany(r.Context(), s.requestContext(r, principal), requests)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]map[string]any, 0, len(created))
	for _, item := range created {
		out = append(out, map[string]any{
			"id":      item.Secret.ID.String(),
			"key":     item.Secret.Key,
			"version": item.Version.Version,
			"masked":  maskedValue,
		})
	}

	writeJSON(w, r, s.logger, http.StatusCreated, map[string]any{
		"created": out,
		"count":   len(out),
	})
}
