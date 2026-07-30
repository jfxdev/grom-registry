package bunstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identity "github.com/jfxdev/grom/backend/internal/identity/domain"
	"github.com/uptrace/bun"
)

type Repository struct {
	db *bun.DB
}

func New(db *bun.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	return r.db.NewSelect().Model((*userModel)(nil)).Count(ctx)
}

func (r *Repository) CreateUser(ctx context.Context, user *identity.User) error {
	_, err := r.db.NewInsert().Model(fromUser(user)).Exec(ctx)
	return err
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	model := new(userModel)
	err := r.db.NewSelect().Model(model).Where("email = ?", email).Where("disabled_at IS NULL").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return toUser(model), nil
}

func (r *Repository) FindUserByUsername(ctx context.Context, username string) (*identity.User, error) {
	model := new(userModel)
	err := r.db.NewSelect().Model(model).Where("username = ?", username).Where("disabled_at IS NULL").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return toUser(model), nil
}

func (r *Repository) FindUserByID(ctx context.Context, id foundation.ID) (*identity.User, error) {
	model := new(userModel)
	err := r.db.NewSelect().Model(model).Where("id = ?", id.String()).Where("disabled_at IS NULL").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return toUser(model), nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]identity.User, error) {
	var models []userModel
	if err := r.db.NewSelect().Model(&models).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]identity.User, 0, len(models))
	for i := range models {
		result = append(result, *toUser(&models[i]))
	}
	return result, nil
}

func (r *Repository) UpdateUserPassword(ctx context.Context, id foundation.ID, passwordHash string) error {
	result, err := r.db.NewUpdate().Model((*userModel)(nil)).
		Set("password_hash = ?", passwordHash).
		Where("id = ?", id.String()).
		Where("disabled_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) CreatePasswordReset(ctx context.Context, reset *identity.PasswordReset) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model((*passwordResetModel)(nil)).
			Set("used_at = ?", reset.CreatedAt).
			Where("user_id = ?", reset.UserID.String()).
			Where("used_at IS NULL").
			Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewInsert().Model(&passwordResetModel{
			ID: reset.ID.String(), PublicID: reset.PublicID, UserID: reset.UserID.String(),
			SecretHash: reset.SecretHash, CreatedAt: reset.CreatedAt, ExpiresAt: reset.ExpiresAt,
		}).Exec(ctx)
		return err
	})
}

func (r *Repository) FindPasswordResetByPublicID(ctx context.Context, publicID string) (*identity.PasswordReset, error) {
	model := new(passwordResetModel)
	if err := r.db.NewSelect().Model(model).Where("public_id = ?", publicID).Scan(ctx); err != nil {
		return nil, err
	}
	return &identity.PasswordReset{
		ID: foundation.ID(model.ID), PublicID: model.PublicID, UserID: foundation.ID(model.UserID),
		SecretHash: model.SecretHash, CreatedAt: model.CreatedAt, ExpiresAt: model.ExpiresAt, UsedAt: model.UsedAt,
	}, nil
}

func (r *Repository) ConsumePasswordReset(
	ctx context.Context,
	resetID, userID foundation.ID,
	passwordHash string,
	usedAt time.Time,
) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewUpdate().Model((*passwordResetModel)(nil)).
			Set("used_at = ?", usedAt).
			Where("id = ?", resetID.String()).
			Where("user_id = ?", userID.String()).
			Where("used_at IS NULL").
			Where("expires_at > ?", usedAt).
			Exec(ctx)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return sql.ErrNoRows
		}
		result, err = tx.NewUpdate().Model((*userModel)(nil)).
			Set("password_hash = ?", passwordHash).
			Where("id = ?", userID.String()).
			Where("disabled_at IS NULL").
			Exec(ctx)
		if err != nil {
			return err
		}
		affected, _ = result.RowsAffected()
		if affected == 0 {
			return sql.ErrNoRows
		}
		_, err = tx.NewDelete().Model((*sessionModel)(nil)).Where("user_id = ?", userID.String()).Exec(ctx)
		return err
	})
}

func (r *Repository) CreateSession(ctx context.Context, session *identity.Session) error {
	model := &sessionModel{
		ID: session.ID.String(), PublicID: session.PublicID, UserID: session.UserID.String(),
		SecretHash: session.SecretHash, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt,
	}
	_, err := r.db.NewInsert().Model(model).Exec(ctx)
	return err
}

func (r *Repository) FindSessionByPublicID(ctx context.Context, publicID string) (*identity.Session, error) {
	model := new(sessionModel)
	if err := r.db.NewSelect().Model(model).Where("public_id = ?", publicID).Scan(ctx); err != nil {
		return nil, err
	}
	return &identity.Session{
		ID: foundation.ID(model.ID), PublicID: model.PublicID, UserID: foundation.ID(model.UserID),
		SecretHash: model.SecretHash, CreatedAt: model.CreatedAt, ExpiresAt: model.ExpiresAt,
	}, nil
}

func (r *Repository) DeleteSession(ctx context.Context, publicID string) error {
	_, err := r.db.NewDelete().Model((*sessionModel)(nil)).Where("public_id = ?", publicID).Exec(ctx)
	return err
}

func (r *Repository) InvalidateEphemeralCredentials(ctx context.Context) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*sessionModel)(nil)).Where("1 = 1").Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewDelete().Model((*passwordResetModel)(nil)).Where("1 = 1").Exec(ctx)
		return err
	})
}

func (r *Repository) CreateServiceAccount(ctx context.Context, account *identity.ServiceAccount) error {
	model := &serviceAccountModel{
		ID: account.ID.String(), Name: account.Name, Username: account.Username,
		Description: account.Description, CreatedAt: account.CreatedAt,
	}
	_, err := r.db.NewInsert().Model(model).Exec(ctx)
	return err
}

func (r *Repository) ListServiceAccounts(ctx context.Context, includeDisabled bool) ([]identity.ServiceAccount, error) {
	var models []serviceAccountModel
	query := r.db.NewSelect().Model(&models)
	if !includeDisabled {
		query = query.Where("disabled_at IS NULL")
	}
	if err := query.OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]identity.ServiceAccount, 0, len(models))
	for i := range models {
		result = append(result, *toServiceAccount(&models[i]))
	}
	return result, nil
}

func (r *Repository) FindServiceAccountByID(ctx context.Context, id foundation.ID) (*identity.ServiceAccount, error) {
	model := new(serviceAccountModel)
	if err := r.db.NewSelect().Model(model).Where("id = ?", id.String()).Where("disabled_at IS NULL").Scan(ctx); err != nil {
		return nil, err
	}
	return toServiceAccount(model), nil
}

func (r *Repository) FindServiceAccountByUsername(ctx context.Context, username string) (*identity.ServiceAccount, error) {
	model := new(serviceAccountModel)
	if err := r.db.NewSelect().Model(model).Where("username = ?", username).Where("disabled_at IS NULL").Scan(ctx); err != nil {
		return nil, err
	}
	return toServiceAccount(model), nil
}

func (r *Repository) DisableServiceAccount(ctx context.Context, id foundation.ID) error {
	now := time.Now().UTC()
	result, err := r.db.NewUpdate().Model((*serviceAccountModel)(nil)).
		Set("disabled_at = ?", now).Where("id = ?", id.String()).Where("disabled_at IS NULL").Exec(ctx)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) CreateServiceAccountAPIToken(ctx context.Context, token *identity.APIToken) error {
	model := &apiTokenModel{
		ID: token.ID.String(), PublicID: token.PublicID, PrincipalKind: constants.PrincipalServiceAccount,
		PrincipalID: token.ServiceAccountID.String(), Name: token.Name, SecretHash: token.SecretHash,
		CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt,
	}
	_, err := r.db.NewInsert().Model(model).Exec(ctx)
	return err
}

func (r *Repository) ListServiceAccountAPITokens(ctx context.Context, serviceAccountID foundation.ID) ([]identity.APIToken, error) {
	var models []apiTokenModel
	if err := r.db.NewSelect().Model(&models).
		Where("principal_kind = ?", constants.PrincipalServiceAccount).
		Where("principal_id = ?", serviceAccountID.String()).
		OrderExpr("created_at DESC").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]identity.APIToken, 0, len(models))
	for i := range models {
		result = append(result, *toAPIToken(&models[i]))
	}
	return result, nil
}

func (r *Repository) FindAPITokenByPublicID(ctx context.Context, publicID string) (*identity.APIToken, error) {
	model := new(apiTokenModel)
	err := r.db.NewSelect().Model(model).Where("public_id = ?", publicID).
		Where("revoked_at IS NULL").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return toAPIToken(model), nil
}

func (r *Repository) RevokeServiceAccountAPIToken(ctx context.Context, serviceAccountID, tokenID foundation.ID) error {
	now := time.Now().UTC()
	result, err := r.db.NewUpdate().Model((*apiTokenModel)(nil)).
		Set("revoked_at = ?", now).
		Where("id = ?", tokenID.String()).
		Where("principal_kind = ?", constants.PrincipalServiceAccount).
		Where("principal_id = ?", serviceAccountID.String()).
		Where("revoked_at IS NULL").Exec(ctx)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) TouchAPIToken(ctx context.Context, id foundation.ID) error {
	_, err := r.db.NewUpdate().Model((*apiTokenModel)(nil)).
		Set("last_used_at = ?", time.Now().UTC()).Where("id = ?", id.String()).Exec(ctx)
	return err
}

func fromUser(user *identity.User) *userModel {
	return &userModel{
		ID: user.ID.String(), Email: user.Email, Username: user.Username, Password: user.PasswordHash,
		SystemAdmin: user.SystemAdmin, CreatedAt: user.CreatedAt, DisabledAt: user.DisabledAt,
	}
}

func toUser(model *userModel) *identity.User {
	return &identity.User{
		ID: foundation.ID(model.ID), Email: model.Email, Username: model.Username, PasswordHash: model.Password,
		SystemAdmin: model.SystemAdmin, CreatedAt: model.CreatedAt, DisabledAt: model.DisabledAt,
	}
}

func toServiceAccount(model *serviceAccountModel) *identity.ServiceAccount {
	return &identity.ServiceAccount{
		ID: foundation.ID(model.ID), Name: model.Name, Username: model.Username,
		Description: model.Description, CreatedAt: model.CreatedAt, DisabledAt: model.DisabledAt,
	}
}

func toAPIToken(model *apiTokenModel) *identity.APIToken {
	return &identity.APIToken{
		ID: foundation.ID(model.ID), PublicID: model.PublicID, ServiceAccountID: foundation.ID(model.PrincipalID),
		Name: model.Name, SecretHash: model.SecretHash,
		CreatedAt: model.CreatedAt, ExpiresAt: model.ExpiresAt, LastUsedAt: model.LastUsedAt, RevokedAt: model.RevokedAt,
	}
}

var _ identity.Repository = (*Repository)(nil)
