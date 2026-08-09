package bunstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
)

type projectKeyset struct {
	at time.Time
	id string
}

func projectBoundary(request foundation.PageRequest) (*projectKeyset, error) {
	if request.Cursor == "" {
		return nil, nil
	}
	cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
	if err != nil || cursor.Timestamp == "" || cursor.ID == "" {
		return nil, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, cursor.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor")
	}
	return &projectKeyset{at: at, id: cursor.ID}, nil
}

func projectNext(scope string, at time.Time, id string) string {
	cursor, _ := foundation.EncodePageCursor(foundation.PageCursor{Scope: scope, Timestamp: at.UTC().Format(time.RFC3339Nano), ID: id})
	return cursor
}

func (r *Repository) ListProjectsPageForPrincipal(ctx context.Context, principal foundation.PrincipalRef, systemAdmin bool, request foundation.PageRequest) (foundation.PageResult[projectdomain.Project], error) {
	boundary, err := projectBoundary(request)
	if err != nil {
		return foundation.PageResult[projectdomain.Project]{}, err
	}
	models := make([]projectModel, 0, request.Limit+1)
	query := r.db.NewSelect().Model(&models)
	if !systemAdmin {
		query = query.Join("JOIN project_memberships AS m ON m.project_id = p.id").Where("m.principal_kind = ?", principal.Kind).Where("m.principal_id = ?", principal.ID.String())
	}
	if boundary != nil {
		query = query.Where("(p.created_at < ? OR (p.created_at = ? AND p.id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("p.created_at DESC, p.id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[projectdomain.Project]{}, err
	}
	result := foundation.PageResult[projectdomain.Project]{Items: make([]projectdomain.Project, 0, projectMin(len(models), request.Limit))}
	for _, model := range models[:projectMin(len(models), request.Limit)] {
		result.Items = append(result.Items, *toProject(&model))
	}
	if len(models) > request.Limit {
		last := models[request.Limit-1]
		result.NextCursor = projectNext(request.Scope, last.CreatedAt, last.ID)
	}
	return result, nil
}

func (r *Repository) ListMembershipsPage(ctx context.Context, projectID foundation.ID, request foundation.PageRequest) (foundation.PageResult[projectdomain.Membership], error) {
	marker := ""
	if request.Cursor != "" {
		cursor, err := foundation.DecodePageCursor(request.Cursor, request.Scope)
		if err != nil || !strings.Contains(cursor.Marker, "\x00") {
			return foundation.PageResult[projectdomain.Membership]{}, fmt.Errorf("invalid cursor")
		}
		marker = cursor.Marker
	}
	models := make([]membershipModel, 0, request.Limit+1)
	query := r.db.NewSelect().Model(&models).Where("project_id = ?", projectID.String())
	if marker != "" {
		parts := strings.SplitN(marker, "\x00", 2)
		query = query.Where("(principal_kind > ? OR (principal_kind = ? AND principal_id > ?))", parts[0], parts[0], parts[1])
	}
	if err := query.OrderExpr("principal_kind ASC, principal_id ASC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[projectdomain.Membership]{}, err
	}
	result := foundation.PageResult[projectdomain.Membership]{Items: make([]projectdomain.Membership, 0, projectMin(len(models), request.Limit))}
	for _, model := range models[:projectMin(len(models), request.Limit)] {
		result.Items = append(result.Items, *toMembership(&model))
	}
	if len(models) > request.Limit {
		last := models[request.Limit-1]
		cursor, _ := foundation.EncodePageCursor(foundation.PageCursor{Scope: request.Scope, Marker: last.PrincipalKind + "\x00" + last.PrincipalID})
		result.NextCursor = cursor
	}
	return result, nil
}

func projectMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ projectdomain.PagedRepository = (*Repository)(nil)
