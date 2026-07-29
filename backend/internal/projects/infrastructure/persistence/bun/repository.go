package bunstore

import (
	"context"

	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	"github.com/uptrace/bun"
)

type Repository struct {
	db *bun.DB
}

func New(db *bun.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateProject(ctx context.Context, project *projectdomain.Project, membership *projectdomain.Membership) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		p := fromProject(project)
		if _, err := tx.NewInsert().Model(p).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewInsert().Model(fromMembership(membership)).Exec(ctx)
		return err
	})
}

func (r *Repository) DeleteProject(ctx context.Context, projectID foundation.ID) error {
	_, err := r.db.NewDelete().Model((*projectModel)(nil)).
		Where("id = ?", projectID.String()).Exec(ctx)
	return err
}

func (r *Repository) ListProjectsForPrincipal(ctx context.Context, principal foundation.PrincipalRef, systemAdmin bool) ([]projectdomain.Project, error) {
	var models []projectModel
	query := r.db.NewSelect().Model(&models)
	if !systemAdmin {
		query = query.Join("JOIN project_memberships AS m ON m.project_id = p.id").
			Where("m.principal_kind = ?", principal.Kind).
			Where("m.principal_id = ?", principal.ID.String())
	}
	if err := query.OrderExpr("p.created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]projectdomain.Project, 0, len(models))
	for i := range models {
		result = append(result, *toProject(&models[i]))
	}
	return result, nil
}

func (r *Repository) FindProjectBySlug(ctx context.Context, slug string) (*projectdomain.Project, error) {
	model := new(projectModel)
	if err := r.db.NewSelect().Model(model).Where("slug = ?", slug).Scan(ctx); err != nil {
		return nil, err
	}
	return toProject(model), nil
}

func (r *Repository) ListMemberships(ctx context.Context, projectID foundation.ID) ([]projectdomain.Membership, error) {
	var models []membershipModel
	if err := r.db.NewSelect().Model(&models).Where("project_id = ?", projectID.String()).
		OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]projectdomain.Membership, 0, len(models))
	for i := range models {
		result = append(result, *toMembership(&models[i]))
	}
	return result, nil
}

func (r *Repository) UpsertMembership(ctx context.Context, membership *projectdomain.Membership) error {
	model := fromMembership(membership)
	_, err := r.db.NewInsert().Model(model).
		On("CONFLICT (project_id, principal_kind, principal_id) DO UPDATE").
		Set("role = EXCLUDED.role").Exec(ctx)
	return err
}

func (r *Repository) DeleteMembership(ctx context.Context, projectID foundation.ID, principal foundation.PrincipalRef) error {
	_, err := r.db.NewDelete().Model((*membershipModel)(nil)).
		Where("project_id = ?", projectID.String()).
		Where("principal_kind = ?", principal.Kind).
		Where("principal_id = ?", principal.ID.String()).Exec(ctx)
	return err
}

func (r *Repository) FindMembership(ctx context.Context, projectID foundation.ID, principal foundation.PrincipalRef) (*projectdomain.Membership, error) {
	model := new(membershipModel)
	if err := r.db.NewSelect().Model(model).
		Where("project_id = ?", projectID.String()).
		Where("principal_kind = ?", principal.Kind).
		Where("principal_id = ?", principal.ID.String()).Scan(ctx); err != nil {
		return nil, err
	}
	return toMembership(model), nil
}

func fromProject(project *projectdomain.Project) *projectModel {
	return &projectModel{
		ID: project.ID.String(), Slug: project.Slug, Name: project.Name,
		CreatedBy: project.CreatedBy.String(), CreatedAt: project.CreatedAt,
	}
}

func toProject(model *projectModel) *projectdomain.Project {
	return &projectdomain.Project{
		ID: foundation.ID(model.ID), Slug: model.Slug, Name: model.Name,
		CreatedBy: foundation.ID(model.CreatedBy), CreatedAt: model.CreatedAt,
	}
}

func fromMembership(membership *projectdomain.Membership) *membershipModel {
	return &membershipModel{
		ProjectID: membership.ProjectID.String(), PrincipalKind: membership.PrincipalKind,
		PrincipalID: membership.PrincipalID.String(), Role: membership.Role, CreatedAt: membership.CreatedAt,
	}
}

func toMembership(model *membershipModel) *projectdomain.Membership {
	return &projectdomain.Membership{
		ProjectID: foundation.ID(model.ProjectID), PrincipalKind: model.PrincipalKind,
		PrincipalID: foundation.ID(model.PrincipalID), Role: model.Role, CreatedAt: model.CreatedAt,
	}
}

var _ projectdomain.Repository = (*Repository)(nil)
