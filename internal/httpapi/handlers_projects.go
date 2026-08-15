package httpapi

import (
	"net/http"
	"time"

	"github.com/Tobe0504/Warder/internal/audit"
	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
	"github.com/jackc/pgx/v5"
)

// defaultEnvironments are created with a new project.
//
// They are a convenience, not a security boundary. The policy engine attaches
// no meaning to these names — "production" is not special-cased anywhere — so a
// team that adds "preview" or "qa" gets exactly the same isolation.
var defaultEnvironments = []struct{ Name, Slug string }{
	{"Development", "development"},
	{"Staging", "staging"},
	{"Production", "production"},
}

type projectResponse struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Slug         string                `json:"slug"`
	CreatedAt    string                `json:"createdAt"`
	Environments []environmentResponse `json:"environments,omitempty"`
}

type environmentResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	ProjectID string `json:"projectId"`
	CreatedAt string `json:"createdAt"`
}

func toProjectResponse(p domain.Project) projectResponse {
	return projectResponse{
		ID:        p.ID.String(),
		Name:      p.Name,
		Slug:      p.Slug,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toEnvironmentResponse(e domain.Environment) environmentResponse {
	return environmentResponse{
		ID:        e.ID.String(),
		Name:      e.Name,
		Slug:      e.Slug,
		ProjectID: e.ProjectID.String(),
		CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// handleListProjects returns the projects in the caller's organization.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{}) {
		return
	}

	projects, err := s.projects.ListProjects(r.Context(), principal.OrganizationID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"projects": out})
}

type createProjectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// handleCreateProject creates a project with its default environments.
//
// It also grants the creator USE_SECRET on the development environment. That
// grant is explicit and appears in the access list and the audit trail, rather
// than being an invisible property of having created something. Without it the
// creator could administer a project whose applications they could not run,
// which is a confusing first five minutes; with it, the grant is visible and
// revocable like any other.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !s.allow(w, r, principal, domain.CapManageProject, authzTarget{}) {
		return
	}

	var req createProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	name := v.requireName("name", req.Name)
	slug := v.requireSlug("slug", req.Slug)
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	var project *domain.Project
	var environments []domain.Environment

	err := store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var err error
		project, err = s.projects.CreateProject(r.Context(), tx, principal.OrganizationID, name, slug)
		if err != nil {
			return err
		}

		var development *domain.Environment
		for _, e := range defaultEnvironments {
			env, err := s.projects.CreateEnvironment(r.Context(), tx, project.ID, e.Name, e.Slug)
			if err != nil {
				return err
			}
			if e.Slug == "development" {
				development = env
			}
			environments = append(environments, *env)
		}

		if development != nil {
			if err := s.grants.Create(r.Context(), tx, &domain.AccessGrant{
				OrganizationID: principal.OrganizationID,
				SubjectType:    domain.SubjectUser,
				SubjectID:      principal.ID,
				ProjectID:      &project.ID,
				EnvironmentID:  &development.ID,
				Capabilities:   []domain.Capability{domain.CapUseSecret},
				CreatedBy:      principal.ID,
				Reason:         "Automatically granted to the project creator on development.",
			}); err != nil {
				return err
			}
		}

		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventProjectCreated,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      &project.ID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       map[string]any{"slug": slug, "environments": len(environments)},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	response := toProjectResponse(*project)
	for _, e := range environments {
		response.Environments = append(response.Environments, toEnvironmentResponse(e))
	}
	writeJSON(w, r, s.logger, http.StatusCreated, response)
}

// handleGetProject returns one project with its environments.
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{ProjectID: projectID}) {
		return
	}

	project, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}
	environments, err := s.projects.ListEnvironments(r.Context(), principal.OrganizationID, projectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	response := toProjectResponse(*project)
	for _, e := range environments {
		response.Environments = append(response.Environments, toEnvironmentResponse(e))
	}
	writeJSON(w, r, s.logger, http.StatusOK, response)
}

// handleListEnvironments returns a project's environments.
func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapReadMetadata, authzTarget{ProjectID: projectID}) {
		return
	}

	// The project is resolved before its children are listed. Relying on the
	// organization filter in the listing query alone would answer a request for
	// another tenant's project with an empty list and a 200, which is a
	// different reply from the 404 every other endpoint gives — and it would
	// leave the tenancy check implicit for anyone writing the next handler.
	if _, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	environments, err := s.projects.ListEnvironments(r.Context(), principal.OrganizationID, projectID)
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	out := make([]environmentResponse, 0, len(environments))
	for _, e := range environments {
		out = append(out, toEnvironmentResponse(e))
	}
	writeJSON(w, r, s.logger, http.StatusOK, map[string]any{"environments": out})
}

type createEnvironmentRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// handleCreateEnvironment adds a custom environment to a project.
func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	projectID, valid := pathUUID(r, "projectID")
	if !valid {
		writeError(w, r, s.logger, ErrNotFound, nil)
		return
	}
	if !s.allow(w, r, principal, domain.CapManageProject, authzTarget{ProjectID: projectID}) {
		return
	}

	// Confirm the project is in the caller's organization before writing a
	// child row against it.
	if _, err := s.projects.GetProject(r.Context(), principal.OrganizationID, projectID); err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	var req createEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, s.logger, ErrBadRequest, err)
		return
	}

	v := newValidator()
	name := v.requireName("name", req.Name)
	slug := v.requireSlug("slug", req.Slug)
	if !v.ok() {
		writeError(w, r, s.logger, v.err(), nil)
		return
	}

	var environment *domain.Environment
	err := store.InTx(r.Context(), s.db, func(tx pgx.Tx) error {
		var err error
		environment, err = s.projects.CreateEnvironment(r.Context(), tx, projectID, name, slug)
		if err != nil {
			return err
		}
		return s.audit.RecordTx(r.Context(), tx, audit.Event{
			OrganizationID: principal.OrganizationID,
			Type:           audit.EventEnvironmentCreated,
			Outcome:        audit.OutcomeSuccess,
			ActorType:      principal.ActorType,
			ActorID:        &principal.ID,
			ActorLabel:     principal.DisplayName,
			CredentialID:   &principal.CredentialID,
			ProjectID:      &projectID,
			EnvironmentID:  &environment.ID,
			IPAddress:      ClientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent:      r.UserAgent(),
			Metadata:       map[string]any{"slug": slug},
		})
	})
	if err != nil {
		writeError(w, r, s.logger, translateError(err), err)
		return
	}

	writeJSON(w, r, s.logger, http.StatusCreated, toEnvironmentResponse(*environment))
}
