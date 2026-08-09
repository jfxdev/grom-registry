package bunstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
)

type identityKeyset struct {
	at time.Time
	id string
}

func identityBoundary(request foundation.PageRequest) (*identityKeyset, error) {
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
	return &identityKeyset{at: at, id: cursor.ID}, nil
}

func identityNext(scope string, at time.Time, id string) string {
	cursor, _ := foundation.EncodePageCursor(foundation.PageCursor{Scope: scope, Timestamp: at.UTC().Format(time.RFC3339Nano), ID: id})
	return cursor
}

func (r *Repository) ListUsersPage(ctx context.Context, rawQuery string, request foundation.PageRequest) (foundation.PageResult[identity.User], error) {
	boundary, err := identityBoundary(request)
	if err != nil {
		return foundation.PageResult[identity.User]{}, err
	}
	models := make([]userModel, 0, request.Limit+1)
	query := r.db.NewSelect().Model(&models)
	if value := strings.ToLower(strings.TrimSpace(rawQuery)); value != "" {
		like := "%" + value + "%"
		query = query.Where("(LOWER(email) LIKE ? OR LOWER(username) LIKE ?)", like, like)
	}
	if boundary != nil {
		query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("created_at DESC, id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[identity.User]{}, err
	}
	result := foundation.PageResult[identity.User]{Items: make([]identity.User, 0, min(len(models), request.Limit))}
	for _, model := range models[:min(len(models), request.Limit)] {
		result.Items = append(result.Items, *toUser(&model))
	}
	if len(models) > request.Limit {
		last := models[request.Limit-1]
		result.NextCursor = identityNext(request.Scope, last.CreatedAt, last.ID)
	}
	return result, nil
}

func (r *Repository) ListServiceAccountsPage(ctx context.Context, rawQuery, status string, request foundation.PageRequest) (foundation.PageResult[identity.ServiceAccount], error) {
	boundary, err := identityBoundary(request)
	if err != nil {
		return foundation.PageResult[identity.ServiceAccount]{}, err
	}
	models := make([]serviceAccountModel, 0, request.Limit+1)
	query := r.db.NewSelect().Model(&models)
	switch status {
	case "active":
		query = query.Where("disabled_at IS NULL")
	case "disabled":
		query = query.Where("disabled_at IS NOT NULL")
	case "all":
	default:
		return foundation.PageResult[identity.ServiceAccount]{}, fmt.Errorf("invalid status")
	}
	if value := strings.ToLower(strings.TrimSpace(rawQuery)); value != "" {
		like := "%" + value + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(username) LIKE ? OR LOWER(description) LIKE ?)", like, like, like)
	}
	if boundary != nil {
		query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("created_at DESC, id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[identity.ServiceAccount]{}, err
	}
	result := foundation.PageResult[identity.ServiceAccount]{Items: make([]identity.ServiceAccount, 0, min(len(models), request.Limit))}
	for _, model := range models[:min(len(models), request.Limit)] {
		result.Items = append(result.Items, *toServiceAccount(&model))
	}
	if len(models) > request.Limit {
		last := models[request.Limit-1]
		result.NextCursor = identityNext(request.Scope, last.CreatedAt, last.ID)
	}
	return result, nil
}

func (r *Repository) ListServiceAccountAPITokensPage(ctx context.Context, serviceAccountID foundation.ID, request foundation.PageRequest) (foundation.PageResult[identity.APIToken], error) {
	boundary, err := identityBoundary(request)
	if err != nil {
		return foundation.PageResult[identity.APIToken]{}, err
	}
	models := make([]apiTokenModel, 0, request.Limit+1)
	query := r.db.NewSelect().Model(&models).Where("principal_kind = ?", constants.PrincipalServiceAccount).Where("principal_id = ?", serviceAccountID.String())
	if boundary != nil {
		query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", boundary.at, boundary.at, boundary.id)
	}
	if err := query.OrderExpr("created_at DESC, id DESC").Limit(request.Limit + 1).Scan(ctx); err != nil {
		return foundation.PageResult[identity.APIToken]{}, err
	}
	result := foundation.PageResult[identity.APIToken]{Items: make([]identity.APIToken, 0, min(len(models), request.Limit))}
	for _, model := range models[:min(len(models), request.Limit)] {
		result.Items = append(result.Items, *toAPIToken(&model))
	}
	if len(models) > request.Limit {
		last := models[request.Limit-1]
		result.NextCursor = identityNext(request.Scope, last.CreatedAt, last.ID)
	}
	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ identity.PagedRepository = (*Repository)(nil)
