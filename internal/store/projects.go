package store

import (
	"context"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/google/uuid"
)

// ProjectRepo covers projects and environments.
//
// Every read is filtered by organization in the SQL itself rather than checked
// after loading. A query that cannot return another tenant's row is a stronger
// guarantee than one that returns it and then discards it.
type ProjectRepo struct{ db *DB }

// NewProjectRepo constructs the repository.
func NewProjectRepo(db *DB) *ProjectRepo { return &ProjectRepo{db: db} }

// CreateProject inserts a project.
func (r *ProjectRepo) CreateProject(ctx context.Context, q Queryer, orgID uuid.UUID, name, slug string) (*domain.Project, error) {
	if q == nil {
		q = r.db.Pool
	}
	p := &domain.Project{OrganizationID: orgID, Name: name, Slug: slug}
	err := q.QueryRow(ctx, `
		INSERT INTO projects (organization_id, name, slug) VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		orgID, name, slug,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return p, nil
}

// ListProjects returns every project in an organization.
func (r *ProjectRepo) ListProjects(ctx context.Context, orgID uuid.UUID) ([]domain.Project, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, slug, created_at, updated_at
		FROM projects WHERE organization_id = $1 ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		p := domain.Project{OrganizationID: orgID}
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, translate(err)
		}
		projects = append(projects, p)
	}
	return projects, translate(rows.Err())
}

// GetProject loads a project scoped to its organization.
func (r *ProjectRepo) GetProject(ctx context.Context, orgID, projectID uuid.UUID) (*domain.Project, error) {
	p := &domain.Project{ID: projectID, OrganizationID: orgID}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT name, slug, created_at, updated_at
		FROM projects WHERE id = $1 AND organization_id = $2`, projectID, orgID,
	).Scan(&p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return p, nil
}

// GetProjectBySlug resolves the human-facing identifier used by the CLI.
func (r *ProjectRepo) GetProjectBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Project, error) {
	p := &domain.Project{OrganizationID: orgID, Slug: slug}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, name, created_at, updated_at
		FROM projects WHERE organization_id = $1 AND slug = $2`, orgID, slug,
	).Scan(&p.ID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return p, nil
}

// CreateEnvironment inserts an environment under a project.
func (r *ProjectRepo) CreateEnvironment(ctx context.Context, q Queryer, projectID uuid.UUID, name, slug string) (*domain.Environment, error) {
	if q == nil {
		q = r.db.Pool
	}
	e := &domain.Environment{ProjectID: projectID, Name: name, Slug: slug}
	err := q.QueryRow(ctx, `
		INSERT INTO environments (project_id, name, slug) VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		projectID, name, slug,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return e, nil
}

// ListEnvironments returns the environments of a project, verifying the project
// belongs to the organization in the same statement.
func (r *ProjectRepo) ListEnvironments(ctx context.Context, orgID, projectID uuid.UUID) ([]domain.Environment, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT e.id, e.name, e.slug, e.created_at, e.updated_at
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE e.project_id = $1 AND p.organization_id = $2
		ORDER BY e.created_at ASC`, projectID, orgID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()

	var envs []domain.Environment
	for rows.Next() {
		e := domain.Environment{ProjectID: projectID}
		if err := rows.Scan(&e.ID, &e.Name, &e.Slug, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, translate(err)
		}
		envs = append(envs, e)
	}
	return envs, translate(rows.Err())
}

// GetEnvironment loads an environment, confirming its place in the organization.
func (r *ProjectRepo) GetEnvironment(ctx context.Context, orgID, envID uuid.UUID) (*domain.Environment, error) {
	e := &domain.Environment{ID: envID}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT e.project_id, e.name, e.slug, e.created_at, e.updated_at
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE e.id = $1 AND p.organization_id = $2`, envID, orgID,
	).Scan(&e.ProjectID, &e.Name, &e.Slug, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, translate(err)
	}
	return e, nil
}

// GetEnvironmentBySlug resolves project and environment slugs together, which
// is the form the CLI and runtime tokens use.
func (r *ProjectRepo) GetEnvironmentBySlug(ctx context.Context, orgID uuid.UUID, projectSlug, envSlug string) (*domain.Project, *domain.Environment, error) {
	p := &domain.Project{OrganizationID: orgID, Slug: projectSlug}
	e := &domain.Environment{Slug: envSlug}

	err := r.db.Pool.QueryRow(ctx, `
		SELECT p.id, p.name, e.id, e.name
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE p.organization_id = $1 AND p.slug = $2 AND e.slug = $3`,
		orgID, projectSlug, envSlug,
	).Scan(&p.ID, &p.Name, &e.ID, &e.Name)
	if err != nil {
		return nil, nil, translate(err)
	}
	e.ProjectID = p.ID
	return p, e, nil
}
