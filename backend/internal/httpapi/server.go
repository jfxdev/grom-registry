package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jfxdev/grom/backend/api"
	auditapp "github.com/jfxdev/grom/backend/internal/audit/application"
	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	identitydomain "github.com/jfxdev/grom/backend/internal/identity/domain"
	platformbackup "github.com/jfxdev/grom/backend/internal/platform/backup"
	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
	"github.com/jfxdev/grom/backend/internal/platform/registrymaintenance"
	projectapp "github.com/jfxdev/grom/backend/internal/projects/application"
	projectdomain "github.com/jfxdev/grom/backend/internal/projects/domain"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/distribution"
	"github.com/jfxdev/grom/backend/internal/webassets"
)

type Server struct {
	router              chi.Router
	identity            *identityapp.Service
	audit               *auditapp.Service
	projects            *projectapp.Service
	repositories        *registryapp.RepositoryService
	inventory           *registryapp.InventoryService
	artifactDeletions   *registryapp.ArtifactDeletionService
	lifecycle           *registryapp.LifecycleService
	registryTokens      *registryapp.TokenService
	distributionClient  *distribution.Client
	gateway             http.Handler
	logger              *slog.Logger
	publicURL           *url.URL
	secureCookies       bool
	enableDocs          bool
	deploymentProfile   string
	insecureHTTP        bool
	trustedProxies      []netip.Prefix
	loginLimiter        *authenticationFailureLimiter
	registryLimiter     *authenticationFailureLimiter
	backups             *platformbackup.Manager
	maintenance         *maintenance.Controller
	databaseKind        string
	registryMaintenance *registrymaintenance.Client
}

type currentUserKey struct{}

type OperationalOptions struct {
	Backups             *platformbackup.Manager
	Maintenance         *maintenance.Controller
	Database            string
	RegistryMaintenance *registrymaintenance.Client
}

func New(
	identity *identityapp.Service,
	audit *auditapp.Service,
	projects *projectapp.Service,
	repositories *registryapp.RepositoryService,
	inventory *registryapp.InventoryService,
	artifactDeletions *registryapp.ArtifactDeletionService,
	lifecycle *registryapp.LifecycleService,
	registryTokens *registryapp.TokenService,
	distributionClient *distribution.Client,
	gateway http.Handler,
	logger *slog.Logger,
	publicURL string,
	secureCookies bool,
	enableDocs bool,
	deploymentProfile string,
	insecureHTTP bool,
	securityOptions SecurityOptions,
	operationalOptions OperationalOptions,
) (*Server, error) {
	if securityOptions.AuthFailureLimit <= 0 ||
		securityOptions.AuthFailureWindow <= 0 ||
		securityOptions.AuthBlockDuration <= 0 {
		return nil, fmt.Errorf("authentication failure limiter settings must be positive")
	}
	if operationalOptions.Database != "sqlite" && operationalOptions.Database != "postgres" {
		return nil, fmt.Errorf("operational database must be sqlite or postgres")
	}
	parsedPublicURL, err := url.Parse(publicURL)
	if err != nil {
		return nil, fmt.Errorf("parse public URL: %w", err)
	}
	server := &Server{
		identity: identity, audit: audit, projects: projects, repositories: repositories,
		inventory: inventory, artifactDeletions: artifactDeletions,
		lifecycle: lifecycle, registryTokens: registryTokens,
		distributionClient: distributionClient, gateway: gateway, logger: logger,
		publicURL: parsedPublicURL, secureCookies: secureCookies, enableDocs: enableDocs,
		deploymentProfile: deploymentProfile, insecureHTTP: insecureHTTP,
		trustedProxies:      securityOptions.TrustedProxies,
		loginLimiter:        newAuthenticationFailureLimiter(securityOptions),
		registryLimiter:     newAuthenticationFailureLimiter(securityOptions),
		backups:             operationalOptions.Backups,
		maintenance:         operationalOptions.Maintenance,
		databaseKind:        operationalOptions.Database,
		registryMaintenance: operationalOptions.RegistryMaintenance,
	}
	if server.maintenance == nil {
		server.maintenance = maintenance.New()
	}
	server.router = server.routes()
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) routes() chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(s.trustedRealIP)
	router.Use(middleware.Recoverer)
	router.Use(s.accessLog)
	router.Use(s.originGuard)
	router.Use(s.maintenanceGate)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	router.Get("/api/v1/deployment", s.getDeployment)
	router.Get("/auth/token", s.exchangeRegistryToken)
	router.Handle("/v2/", s.gateway)
	router.Handle("/v2/*", s.gateway)

	if s.enableDocs {
		router.Get("/api/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write(api.OpenAPI)
		})
		router.Get("/api/docs", s.apiDocs)
	}

	router.Route("/api/v1", func(apiRouter chi.Router) {
		apiRouter.Post("/session", s.createSession)
		apiRouter.Delete("/session", s.deleteSession)
		apiRouter.Post("/password-resets", s.completePasswordReset)

		apiRouter.Group(func(protected chi.Router) {
			protected.Use(s.requireSession)
			protected.Use(s.viewerReadOnly)
			protected.Get("/me", s.currentUser)
			protected.Get("/settings/status", s.getInstallationStatus)
			protected.Post("/garbage-collections", s.runGarbageCollection)
			protected.Put("/me/password", s.changeCurrentUserPassword)
			protected.Get("/me/registry-tokens", s.listViewerRegistryTokens)
			protected.Post("/me/registry-tokens", s.createViewerRegistryToken)
			protected.Delete("/me/registry-tokens/{tokenId}", s.revokeViewerRegistryToken)
			protected.Get("/users", s.listUsers)
			protected.Post("/users", s.createUser)
			protected.Delete("/users/{id}", s.deleteUser)
			protected.Put("/users/{id}/administrator", s.promoteUserToSystemAdmin)
			protected.Put("/users/{id}/viewer", s.promoteUserToSystemViewer)
			protected.Post("/users/{id}/password-reset-link", s.createUserPasswordResetLink)

			protected.Get("/service-accounts", s.listServiceAccounts)
			protected.Post("/service-accounts", s.createServiceAccount)
			protected.Delete("/service-accounts/{id}", s.deleteServiceAccount)
			protected.Get("/service-accounts/{id}/tokens", s.listServiceAccountTokens)
			protected.Post("/service-accounts/{id}/tokens", s.createServiceAccountToken)
			protected.Delete("/service-accounts/{id}/tokens/{tokenId}", s.revokeServiceAccountToken)

			protected.Get("/projects", s.listProjects)
			protected.Post("/projects", s.createProject)
			protected.Get("/projects/{project}", s.getProject)
			protected.Delete("/projects/{project}", s.deleteProject)
			protected.Get("/projects/{project}/members", s.listMemberships)
			protected.Put("/projects/{project}/members/{principalKind}/{principalId}", s.setMembership)
			protected.Delete("/projects/{project}/members/{principalKind}/{principalId}", s.deleteMembership)
			protected.Get("/projects/{project}/repositories", s.listRepositories)
			protected.Post("/projects/{project}/repositories", s.createRepository)
			protected.Get("/projects/{project}/repositories/{repositoryId}", s.getRepository)
			protected.Post("/projects/{project}/repositories/{repositoryId}/archive", s.archiveRepository)
			protected.Delete("/projects/{project}/repositories/{repositoryId}/archive", s.unarchiveRepository)
			protected.Delete("/projects/{project}/repositories/{repositoryId}", s.removeRepository)
			protected.Get("/projects/{project}/repositories/{repositoryId}/policies", s.getRepositoryPolicies)
			protected.Put("/projects/{project}/repositories/{repositoryId}/policies", s.replaceRepositoryPolicies)
			protected.Get("/projects/{project}/repository-tags", s.listTags)
			protected.Post("/projects/{project}/artifact-deletion-previews", s.previewArtifactDeletion)
			protected.Get("/projects/{project}/artifact-deletions", s.listArtifactDeletions)
			protected.Post("/projects/{project}/artifact-deletions", s.deleteArtifact)
			protected.Get("/projects/{project}/repository-inventory", s.listRepositoryInventory)
			protected.Post("/projects/{project}/repository-inventory-reconciliations", s.reconcileRepositoryInventory)
			protected.Post("/projects/{project}/lifecycle-previews", s.createLifecyclePreview)
			protected.Get("/projects/{project}/lifecycle-previews/{previewId}", s.getLifecyclePreview)
			protected.Post("/projects/{project}/lifecycle-runs", s.createLifecycleRun)
			protected.Get("/projects/{project}/lifecycle-runs", s.listLifecycleRuns)
			protected.Get("/projects/{project}/lifecycle-runs/{runId}", s.getLifecycleRun)
			protected.Get("/registry-policy-presets", s.listRegistryPolicyPresets)

			protected.Get("/backups", s.listBackups)
			protected.Post("/backups", s.createBackup)
			protected.Delete("/backups/{backupId}", s.deleteBackup)
			protected.Get("/backups/{backupId}/download", s.downloadBackup)
		})
	})

	router.NotFound(s.spa)
	return router
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	cursor := r.URL.Query().Get("cursor")
	if len(cursor) > 512 {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Backup page cursor is invalid")
		return
	}
	if s.backups == nil {
		writeJSON(w, http.StatusOK, platformbackup.Overview{
			Available: false, Backups: []platformbackup.Summary{},
			TotalBackups: 0, PageSize: platformbackup.PageSize,
		})
		return
	}
	overview, err := s.backups.Overview(r.Context(), cursor)
	if errors.Is(err, platformbackup.ErrInvalidCursor) {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Backup page cursor is invalid")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if s.backups == nil {
		writeError(w, r, http.StatusServiceUnavailable, "backup_unavailable", "The integrated backup agent is unavailable")
		return
	}
	operation, err := s.backups.Start()
	if err != nil {
		if errors.Is(err, platformbackup.ErrOperationInProgress) {
			writeError(w, r, http.StatusConflict, "backup_active", "A backup operation is already running")
			return
		}
		writeError(w, r, http.StatusServiceUnavailable, "backup_unavailable", "The integrated backup agent is unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, operation)
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if s.backups == nil {
		writeError(w, r, http.StatusServiceUnavailable, "backup_unavailable", "The integrated backup agent is unavailable")
		return
	}
	backupID := chi.URLParam(r, "backupId")
	if uuid.Validate(backupID) != nil {
		writeError(w, r, http.StatusNotFound, "backup_not_found", "Backup not found")
		return
	}
	user := userFromContext(r.Context())
	if err := s.recordAudit(
		r, principalForUser(user), constants.AuditBackupDeleteRequested,
		constants.AuditResourceBackup, foundation.ID(backupID), map[string]any{},
	); err != nil {
		s.internalError(w, r, err)
		return
	}
	err := s.backups.Delete(r.Context(), backupID)
	switch {
	case errors.Is(err, platformbackup.ErrOperationInProgress):
		writeError(w, r, http.StatusConflict, "backup_active", "Another backup operation is already running")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, r, http.StatusNotFound, "backup_not_found", "Backup not found")
	case err != nil:
		writeError(w, r, http.StatusServiceUnavailable, "backup_unavailable", "The integrated backup agent could not delete this backup")
	default:
		if auditErr := s.audit.Record(
			r.Context(), principalForUser(user), constants.AuditBackupDeleted,
			constants.AuditResourceBackup, foundation.ID(backupID), map[string]any{},
		); auditErr != nil {
			s.logger.Error("record backup deletion audit event", "error", auditErr)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if s.backups == nil {
		writeError(w, r, http.StatusNotFound, "backup_not_found", "Backup not found")
		return
	}
	response, err := s.backups.Download(r.Context(), chi.URLParam(r, "backupId"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "backup_not_found", "Backup not found")
		return
	}
	defer func() { _ = response.Body.Close() }()
	w.Header().Set("Content-Type", "application/x-tar")
	if disposition := response.Header.Get("Content-Disposition"); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, response.Body)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	limiterKey := authenticationLimiterKey(r)
	if allowed, retryAfter := s.loginLimiter.allow(limiterKey, time.Now()); !allowed {
		writeRateLimitError(w, r, retryAfter)
		return
	}
	raw, user, err := s.identity.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		s.loginLimiter.failure(limiterKey, time.Now())
		_ = s.recordAudit(r, anonymousPrincipal(), constants.AuditLoginFailed, constants.AuditResourceAuthentication, "", map[string]any{"reason": "invalid_credentials"})
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Invalid email or password")
		return
	}
	s.loginLimiter.success(limiterKey)
	_ = s.recordAudit(r, principalForUser(user), constants.AuditLoginSucceeded, constants.AuditResourceUser, user.ID, nil)
	s.setSessionCookie(w, raw)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(constants.SessionCookieName); err == nil {
		_ = s.identity.Logout(r.Context(), cookie.Value)
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: constants.SessionCookieName, Value: value, Path: "/", HttpOnly: true,
		Secure: s.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: constants.DefaultSessionHours * 3600,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: constants.SessionCookieName, Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userFromContext(r.Context()))
}

func (s *Server) getDeployment(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":      s.deploymentProfile,
		"insecureHttp": s.insecureHTTP,
	})
}

func (s *Server) getInstallationStatus(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	distributionStatus := "unavailable"
	if s.distributionClient != nil && s.distributionClient.Available(r.Context()) {
		distributionStatus = "available"
	}
	result := map[string]any{"database": s.databaseKind, "distribution": distributionStatus, "storage": nil}
	if s.registryMaintenance != nil {
		if storage, err := s.registryMaintenance.Storage(r.Context()); err == nil {
			result["storage"] = storage
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) runGarbageCollection(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if s.registryMaintenance == nil {
		writeError(w, r, http.StatusServiceUnavailable, "garbage_collection_unavailable", "Registry maintenance agent is unavailable")
		return
	}
	actor := principalForUser(userFromContext(r.Context()))
	operationID := foundation.ID(uuid.NewString())
	end, err := s.maintenance.Begin(r.Context())
	if err != nil {
		w.Header().Set("Retry-After", "5")
		writeError(w, r, http.StatusServiceUnavailable, "maintenance_active", "Another maintenance operation is active")
		return
	}
	defer end()
	if err := s.recordAudit(r, actor, constants.AuditGarbageCollectionStarted, constants.AuditResourceGarbageCollection, operationID, nil); err != nil {
		s.internalError(w, r, err)
		return
	}
	result, err := s.registryMaintenance.Collect(r.Context())
	if err != nil {
		_ = s.recordAudit(r, actor, constants.AuditGarbageCollectionFailed, constants.AuditResourceGarbageCollection, operationID, map[string]any{"message": "registry garbage collection failed"})
		s.internalError(w, r, err)
		return
	}
	if err := s.recordAudit(r, actor, constants.AuditGarbageCollectionCompleted, constants.AuditResourceGarbageCollection, operationID, map[string]any{"startedAt": result.StartedAt, "completedAt": result.CompletedAt, "reclaimedBytes": result.ReclaimedBytes}); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) changeCurrentUserPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	err := s.identity.ChangePassword(r.Context(), userFromContext(r.Context()).ID, input.CurrentPassword, input.NewPassword)
	switch {
	case errors.Is(err, identityapp.ErrInvalidCurrentPassword):
		writeError(w, r, http.StatusBadRequest, "invalid_current_password", err.Error())
	case err != nil:
		writeError(w, r, http.StatusBadRequest, "invalid_password", err.Error())
	default:
		user := userFromContext(r.Context())
		if auditErr := s.audit.Record(
			r.Context(), principalForUser(user), constants.AuditUserPasswordChanged,
			constants.AuditResourceUser, user.ID, map[string]any{},
		); auditErr != nil {
			s.logger.Error("record password change audit event", "error", auditErr)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) listViewerRegistryTokens(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	tokens, err := s.identity.ListViewerRegistryTokens(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, identityapp.ErrViewerPermissionRequired) {
			writeError(w, r, http.StatusForbidden, "forbidden", "Installation viewer permission required")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "User not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) createViewerRegistryToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user := userFromContext(r.Context())
	created, err := s.identity.CreateViewerRegistryToken(r.Context(), user.ID, input.Name, input.ExpiresAt)
	if err != nil {
		if errors.Is(err, identityapp.ErrViewerPermissionRequired) {
			writeError(w, r, http.StatusForbidden, "forbidden", "Installation viewer permission required")
			return
		}
		if errors.Is(err, identitydomain.ErrViewerRegistryTokenAlreadyExists) {
			writeError(w, r, http.StatusConflict, "viewer_token_exists", "Revoke the active registry token before creating another one")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "User not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	_ = s.recordAudit(r, principalForUser(user), constants.AuditAccessKeyCreated, constants.AuditResourceUser, user.ID, map[string]any{"tokenId": created.Token.ID, "expiresAt": created.Token.ExpiresAt, "readOnly": true})
	writeJSON(w, http.StatusCreated, map[string]any{"token": identitydomain.ViewerRegistryToken{ID: created.Token.ID, PublicID: created.Token.PublicID, Name: created.Token.Name, CreatedAt: created.Token.CreatedAt, ExpiresAt: created.Token.ExpiresAt}, "secret": created.Secret})
}

func (s *Server) revokeViewerRegistryToken(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if err := s.identity.RevokeViewerRegistryToken(r.Context(), user.ID, foundation.ID(chi.URLParam(r, "tokenId"))); err != nil {
		if errors.Is(err, identityapp.ErrViewerPermissionRequired) {
			writeError(w, r, http.StatusForbidden, "forbidden", "Installation viewer permission required")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "Token not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	_ = s.recordAudit(r, principalForUser(user), constants.AuditAccessKeyRevoked, constants.AuditResourceUser, user.ID, map[string]any{"tokenId": chi.URLParam(r, "tokenId"), "readOnly": true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if !validListQuery(r, "cursor", "limit", "q") {
		writeError(w, r, http.StatusBadRequest, "invalid_query", "Query parameters are invalid")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	request, _, err := pageRequest(r, "users:q="+strings.ToLower(query))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	users, err := s.identity.ListUsersPage(r.Context(), query, request)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	var input struct {
		Email    string `json:"email"`
		Username string `json:"username"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	created, err := s.identity.CreateUser(r.Context(), input.Email, input.Username)
	if err != nil {
		if errors.Is(err, identitydomain.ErrUsernameAlreadyExists) {
			writeError(w, r, http.StatusConflict, "username_taken", "This username is already in use")
			return
		}
		if errors.Is(err, identitydomain.ErrEmailAlreadyExists) {
			writeError(w, r, http.StatusConflict, "email_taken", "This email address is already in use")
			return
		}
		if errors.Is(err, identityapp.ErrInvalidUserInput) {
			writeError(w, r, http.StatusBadRequest, "invalid_user", "Email and username are required")
			return
		}
		s.internalError(w, r, err)
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditUserCreated, constants.AuditResourceUser, created.User.ID, map[string]any{"systemAdmin": false})
	writeJSON(w, http.StatusCreated, map[string]any{
		"user": created.User,
		"registrationLink": map[string]any{
			"url":       s.passwordResetURL(r, created.RegistrationLink.Token),
			"expiresAt": created.RegistrationLink.ExpiresAt,
		},
	})
}

func (s *Server) promoteUserToSystemAdmin(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	targetID := foundation.ID(chi.URLParam(r, "id"))
	user, err := s.identity.PromoteUserToSystemAdmin(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "User not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditUserPromotedToSystemAdmin, constants.AuditResourceUser, user.ID, nil)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) promoteUserToSystemViewer(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	targetID := foundation.ID(chi.URLParam(r, "id"))
	user, err := s.identity.PromoteUserToSystemViewer(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "User not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditUserPromotedToSystemViewer, constants.AuditResourceUser, user.ID, nil)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	actor := userFromContext(r.Context())
	targetID := foundation.ID(chi.URLParam(r, "id"))
	if targetID == actor.ID {
		writeError(w, r, http.StatusConflict, "cannot_disable_self", "You cannot disable your own account")
		return
	}
	target, err := s.identity.FindUser(r.Context(), targetID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err := s.identity.DisableUser(r.Context(), targetID); err != nil {
		if target.SystemAdmin {
			writeError(w, r, http.StatusConflict, "cannot_disable_last_admin", "The last administrator cannot be disabled")
			return
		}
		writeError(w, r, http.StatusNotFound, "not_found", "User not found")
		return
	}
	_ = s.recordAudit(r, principalForUser(actor), constants.AuditUserDisabled, constants.AuditResourceUser, targetID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createUserPasswordResetLink(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	targetID := foundation.ID(chi.URLParam(r, "id"))
	created, err := s.identity.CreatePasswordReset(
		r.Context(),
		targetID,
	)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "User not found")
		return
	}
	actor := userFromContext(r.Context())
	if auditErr := s.audit.Record(
		r.Context(), principalForUser(actor), constants.AuditUserPasswordResetLinkCreated,
		constants.AuditResourceUser, targetID, map[string]any{"expiresAt": created.ExpiresAt},
	); auditErr != nil {
		s.logger.Error("record password reset link audit event", "error", auditErr)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":       s.passwordResetURL(r, created.Token),
		"expiresAt": created.ExpiresAt,
	})
}

func (s *Server) passwordResetURL(r *http.Request, token string) string {
	resetURLBase := s.publicURL.String()
	if requestOrigin, parseErr := url.Parse(r.Header.Get("Origin")); parseErr == nil && requestOrigin.Scheme != "" && requestOrigin.Host != "" {
		resetURLBase = requestOrigin.Scheme + "://" + requestOrigin.Host
	}
	return resetURLBase + "/reset-password#token=" + url.QueryEscape(token)
}

func (s *Server) completePasswordReset(w http.ResponseWriter, r *http.Request) {
	if cookie, cookieErr := r.Cookie(constants.SessionCookieName); cookieErr == nil {
		if _, authErr := s.identity.AuthenticateSession(r.Context(), cookie.Value); authErr == nil {
			writeError(w, r, http.StatusForbidden, "signed_in", "Sign out before using a password reset link")
			return
		}
	}
	var input struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	userID, err := s.identity.CompletePasswordReset(r.Context(), input.Token, input.NewPassword)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_password_reset", err.Error())
		return
	}
	if auditErr := s.audit.Record(
		r.Context(), foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: userID},
		constants.AuditUserPasswordResetCompleted, constants.AuditResourceUser, userID, map[string]any{},
	); auditErr != nil {
		s.logger.Error("record completed password reset audit event", "error", auditErr)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listServiceAccounts(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if !validListQuery(r, "cursor", "limit", "q", "status") {
		writeError(w, r, http.StatusBadRequest, "invalid_query", "Query parameters are invalid")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "disabled" && status != "all" {
		writeError(w, r, http.StatusBadRequest, "invalid_status", "Status must be active, disabled, or all")
		return
	}
	request, _, err := pageRequest(r, "service-accounts:status="+status+":q="+strings.ToLower(query))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	accounts, err := s.identity.ListServiceAccountsPage(r.Context(), query, status, request)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (s *Server) createServiceAccount(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	var input struct {
		Name        string `json:"name"`
		Username    string `json:"username"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	account, err := s.identity.CreateServiceAccount(r.Context(), input.Name, input.Username, input.Description)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_service_account", err.Error())
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditServiceAccountCreated, constants.AuditResourceServiceAccount, account.ID, nil)
	writeJSON(w, http.StatusCreated, account)
}

func (s *Server) deleteServiceAccount(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if err := s.identity.DisableServiceAccount(r.Context(), foundation.ID(chi.URLParam(r, "id"))); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Service account not found")
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditServiceAccountDisabled, constants.AuditResourceServiceAccount, foundation.ID(chi.URLParam(r, "id")), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listServiceAccountTokens(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	accountID := foundation.ID(chi.URLParam(r, "id"))
	request, _, err := pageRequest(r, "service-account-tokens:"+accountID.String())
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	tokens, err := s.identity.ListServiceAccountAPITokensPage(r.Context(), accountID, request)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Service account not found")
		return
	}
	activeCount, err := s.identity.CountActiveServiceAccountAPITokens(r.Context(), accountID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items          []identitydomain.APIToken `json:"items"`
		NextCursor     string                    `json:"nextCursor,omitempty"`
		ActiveCount    int                       `json:"activeCount"`
		MaxActiveCount int                       `json:"maxActiveCount"`
	}{
		Items: tokens.Items, NextCursor: tokens.NextCursor,
		ActiveCount: activeCount, MaxActiveCount: constants.MaxActiveServiceAccountAccessKeys,
	})
}

func (s *Server) createServiceAccountToken(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	var input struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	created, err := s.identity.CreateServiceAccountAPIToken(
		r.Context(), foundation.ID(chi.URLParam(r, "id")), input.Name, input.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, identitydomain.ErrServiceAccountAccessKeyLimit) {
			writeError(w, r, http.StatusConflict, "access_key_limit_reached", err.Error())
			return
		}
		writeError(w, r, http.StatusBadRequest, "invalid_token", err.Error())
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditAccessKeyCreated, constants.AuditResourceServiceAccount, foundation.ID(chi.URLParam(r, "id")), map[string]any{"tokenId": created.Token.ID, "expiresAt": created.Token.ExpiresAt})
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) revokeServiceAccountToken(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	if err := s.identity.RevokeServiceAccountAPIToken(
		r.Context(),
		foundation.ID(chi.URLParam(r, "id")),
		foundation.ID(chi.URLParam(r, "tokenId")),
	); err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Token not found")
		return
	}
	_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditAccessKeyRevoked, constants.AuditResourceServiceAccount, foundation.ID(chi.URLParam(r, "id")), map[string]any{"tokenId": chi.URLParam(r, "tokenId")})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	request, _, err := pageRequest(r, "projects:"+user.ID.String())
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	projects, err := s.projects.ListPage(r.Context(), principalForUser(user), user.SystemAdmin, request)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	projectIDs := make([]foundation.ID, len(projects.Items))
	for i, project := range projects.Items {
		projectIDs[i] = project.ID
	}
	usages := s.projectUsages(r.Context(), projectIDs)
	for i := range projects.Items {
		projects.Items[i].AccountedUsage = usages[projects.Items[i].ID]
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user := userFromContext(r.Context())
	project, err := s.projects.Create(r.Context(), principalForUser(user), user.SystemAdmin, input.Name, input.Slug)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_project", err.Error())
		return
	}
	_ = s.recordAudit(r, principalForUser(user), constants.AuditProjectCreated, constants.AuditResourceProject, foundation.ID(project.Slug), map[string]any{"slug": project.Slug})
	writeJSON(w, http.StatusCreated, project)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	user := userFromContext(r.Context())
	if !s.projects.CanView(r.Context(), principalForUser(user), user.SystemAdmin, project) {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	project.AccountedUsage = s.projectUsage(r.Context(), project.ID)
	writeJSON(w, http.StatusOK, struct {
		*projectdomain.Project
		CanManage bool `json:"canManage"`
	}{Project: project, CanManage: s.projects.CanManage(r.Context(), principalForUser(user), user.SystemAdmin, project)})
}

func (s *Server) projectUsage(ctx context.Context, projectID foundation.ID) foundation.AccountedStorageUsage {
	if s.inventory == nil {
		return foundation.AccountedStorageUsage{Status: "unavailable"}
	}
	return s.inventory.ProjectUsage(ctx, projectID)
}

func (s *Server) projectUsages(ctx context.Context, projectIDs []foundation.ID) map[foundation.ID]foundation.AccountedStorageUsage {
	if s.inventory == nil {
		usages := make(map[foundation.ID]foundation.AccountedStorageUsage, len(projectIDs))
		for _, projectID := range projectIDs {
			usages[projectID] = foundation.AccountedStorageUsage{Status: "unavailable"}
		}
		return usages
	}
	return s.inventory.ProjectUsages(ctx, projectIDs)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if !requireSystemAdmin(w, r) {
		return
	}
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.recordAudit(
		r, principalForUser(userFromContext(r.Context())), constants.AuditProjectDeleteRequested,
		constants.AuditResourceProject, project.ID, map[string]any{"slug": project.Slug},
	); err != nil {
		s.internalError(w, r, err)
		return
	}
	err = s.projects.Delete(r.Context(), true, project.Slug)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
	case errors.Is(err, projectapp.ErrProjectNotEmpty):
		writeError(w, r, http.StatusConflict, "project_not_empty", "Remove every repository before deleting this project")
	case err != nil:
		s.internalError(w, r, err)
	default:
		_ = s.recordAudit(r, principalForUser(userFromContext(r.Context())), constants.AuditProjectDeleted, constants.AuditResourceProject, project.ID, map[string]any{"slug": project.Slug})
		w.WriteHeader(http.StatusNoContent)
	}
}

type membershipResponse struct {
	ProjectID       foundation.ID `json:"projectId"`
	PrincipalKind   string        `json:"principalKind"`
	PrincipalID     foundation.ID `json:"principalId"`
	PrincipalName   string        `json:"principalName"`
	PrincipalDetail string        `json:"principalDetail"`
	Role            string        `json:"role"`
	CreatedAt       time.Time     `json:"createdAt"`
}

type membershipPageResponse struct {
	Items      []membershipResponse `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

type membershipPrincipalDetails struct {
	name   string
	detail string
}

func membershipPrincipalKey(principal foundation.PrincipalRef) string {
	return principal.Kind + ":" + principal.ID.String()
}

func matchesMembershipQuery(query string, values ...string) bool {
	if query == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func (s *Server) membershipPrincipalDetails(ctx context.Context, query string) (map[string]membershipPrincipalDetails, []foundation.PrincipalRef, error) {
	users, err := s.identity.ListUsers(ctx)
	if err != nil {
		return nil, nil, err
	}
	accounts, err := s.identity.ListServiceAccounts(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	details := make(map[string]membershipPrincipalDetails, len(users)+len(accounts))
	matched := make([]foundation.PrincipalRef, 0, len(users)+len(accounts))
	for _, user := range users {
		principal := foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: user.ID}
		details[membershipPrincipalKey(principal)] = membershipPrincipalDetails{name: user.Username, detail: user.Email}
		if matchesMembershipQuery(query, user.Username, user.Email) {
			matched = append(matched, principal)
		}
	}
	for _, account := range accounts {
		principal := foundation.PrincipalRef{Kind: constants.PrincipalServiceAccount, ID: account.ID}
		details[membershipPrincipalKey(principal)] = membershipPrincipalDetails{name: account.Name, detail: account.Username}
		if matchesMembershipQuery(query, account.Name, account.Username) {
			matched = append(matched, principal)
		}
	}
	return details, matched, nil
}

func (s *Server) listMemberships(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.SystemViewer {
		writeError(w, r, http.StatusForbidden, "forbidden", "Installation viewer accounts cannot access user memberships")
		return
	}
	if !validListQuery(r, "cursor", "limit", "q") {
		writeError(w, r, http.StatusBadRequest, "invalid_query", "Query parameters are invalid")
		return
	}
	projectSlug := chi.URLParam(r, "project")
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	request, _, pageErr := pageRequest(r, "memberships:"+projectSlug+":q="+query)
	if pageErr != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	details, matched, err := s.membershipPrincipalDetails(r.Context(), query)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var filter *projectdomain.MembershipPrincipalFilter
	if query != "" {
		filter = &projectdomain.MembershipPrincipalFilter{Principals: matched}
	}
	memberships, err := s.projects.ListMembershipsPage(r.Context(), principalForUser(user), user.SystemAdmin, projectSlug, filter, request)
	if err != nil {
		writeError(w, r, http.StatusForbidden, "forbidden", err.Error())
		return
	}
	response := membershipPageResponse{Items: make([]membershipResponse, 0, len(memberships.Items)), NextCursor: memberships.NextCursor}
	for _, membership := range memberships.Items {
		principal := foundation.PrincipalRef{Kind: membership.PrincipalKind, ID: membership.PrincipalID}
		identity, ok := details[membershipPrincipalKey(principal)]
		if !ok {
			s.internalError(w, r, fmt.Errorf("membership principal %s is missing", membershipPrincipalKey(principal)))
			return
		}
		response.Items = append(response.Items, membershipResponse{
			ProjectID: membership.ProjectID, PrincipalKind: membership.PrincipalKind, PrincipalID: membership.PrincipalID,
			PrincipalName: identity.name, PrincipalDetail: identity.detail, Role: membership.Role, CreatedAt: membership.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) setMembership(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	principal := foundation.PrincipalRef{
		Kind: chi.URLParam(r, "principalKind"), ID: foundation.ID(chi.URLParam(r, "principalId")),
	}
	if !s.principalExists(r.Context(), principal) {
		writeError(w, r, http.StatusBadRequest, "invalid_principal", "Principal not found")
		return
	}
	user := userFromContext(r.Context())
	err := s.projects.SetMembership(r.Context(), principalForUser(user), user.SystemAdmin, chi.URLParam(r, "project"), principal, input.Role)
	if err != nil {
		writeError(w, r, http.StatusForbidden, "forbidden", err.Error())
		return
	}
	_ = s.recordAudit(r, principalForUser(user), constants.AuditMembershipUpserted, constants.AuditResourceMembership, principal.ID, map[string]any{"project": chi.URLParam(r, "project"), "role": input.Role, "principalKind": principal.Kind})
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) deleteMembership(w http.ResponseWriter, r *http.Request) {
	principal := foundation.PrincipalRef{
		Kind: chi.URLParam(r, "principalKind"), ID: foundation.ID(chi.URLParam(r, "principalId")),
	}
	user := userFromContext(r.Context())
	err := s.projects.DeleteMembership(r.Context(), principalForUser(user), user.SystemAdmin, chi.URLParam(r, "project"), principal)
	if err != nil {
		writeError(w, r, http.StatusForbidden, "forbidden", err.Error())
		return
	}
	_ = s.recordAudit(r, principalForUser(user), constants.AuditMembershipRemoved, constants.AuditResourceMembership, principal.ID, map[string]any{"project": chi.URLParam(r, "project"), "principalKind": principal.Kind})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRepositories(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	user := userFromContext(r.Context())
	if !s.projects.CanView(r.Context(), principalForUser(user), user.SystemAdmin, project) {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	scope := "repositories:" + project.Slug
	request, _, pageErr := pageRequest(r, scope)
	if pageErr != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	marker := ""
	if request.Cursor != "" {
		cursor, decodeErr := foundation.DecodePageCursor(request.Cursor, scope)
		if decodeErr != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
			return
		}
		marker = cursor.Marker
	}
	discovered := &distribution.ProjectRepositoryPage{}
	for len(discovered.Repositories) < request.Limit {
		page, err := s.distributionClient.ListProjectRepositoriesPage(r.Context(), project.Slug, request.Limit-len(discovered.Repositories), marker)
		if err != nil {
			s.logger.Warn("distribution catalog unavailable", "error", err)
			break
		}
		discovered.Repositories = append(discovered.Repositories, page.Repositories...)
		discovered.NextMarker = page.NextMarker
		if page.NextMarker == "" {
			break
		}
		marker = page.NextMarker
	}
	if err := s.repositories.ReconcileDiscoveredPage(r.Context(), project.ID, discovered.Repositories); err != nil {
		s.internalError(w, r, err)
		return
	}
	repositories, err := s.repositories.ListPage(r.Context(), project.ID, request)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if repositories.NextCursor != "" || discovered.NextMarker != "" {
		cursor := foundation.PageCursor{Scope: scope, Marker: discovered.NextMarker}
		if repositories.NextCursor != "" {
			decoded, _ := foundation.DecodePageCursor(repositories.NextCursor, scope)
			cursor.Name = decoded.Name
		} else if len(repositories.Items) > 0 {
			cursor.Name = repositories.Items[len(repositories.Items)-1].Name
		}
		repositories.NextCursor, _ = foundation.EncodePageCursor(cursor)
	}
	writeJSON(w, http.StatusOK, repositories)
}

func validListQuery(r *http.Request, allowed ...string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	for name, values := range r.URL.Query() {
		if _, ok := allowedSet[name]; !ok || len(values) != 1 {
			return false
		}
	}
	return utf8.RuneCountInString(r.URL.Query().Get("q")) <= 200
}

func (s *Server) getRepository(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	user := userFromContext(r.Context())
	if !s.projects.CanView(r.Context(), principalForUser(user), user.SystemAdmin, project) {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	repository, err := s.repositories.FindByID(r.Context(), foundation.ID(chi.URLParam(r, "repositoryId")))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if repository.ProjectID != project.ID {
		writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
		return
	}
	writeJSON(w, http.StatusOK, repository)
}

func (s *Server) createRepository(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	user := userFromContext(r.Context())
	if !s.projects.CanManage(r.Context(), principalForUser(user), user.SystemAdmin, project) {
		writeError(w, r, http.StatusForbidden, "forbidden", "Project administrator permission required")
		return
	}
	var input struct {
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Policies    []registrydomain.Policy `json:"policies"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	repository, err := s.repositories.Create(r.Context(), project.ID, input.Name, input.Description, input.Policies)
	if errors.Is(err, registryapp.ErrRepositoryExists) {
		writeError(w, r, http.StatusConflict, "repository_exists", "Repository path already exists in this project")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repository)
}

func (s *Server) archiveRepository(w http.ResponseWriter, r *http.Request) {
	project, actor, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	if err := s.repositories.Archive(r.Context(), project.ID, foundation.ID(chi.URLParam(r, "repositoryId")), actor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unarchiveRepository(w http.ResponseWriter, r *http.Request) {
	project, actor, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	if err := s.repositories.Unarchive(r.Context(), project.ID, foundation.ID(chi.URLParam(r, "repositoryId")), actor); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
			return
		}
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeRepository(w http.ResponseWriter, r *http.Request) {
	project, actor, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	repositoryID := foundation.ID(chi.URLParam(r, "repositoryId"))
	validate := func(ctx context.Context) bool {
		repository, err := s.repositories.ValidateRemoval(ctx, project.ID, repositoryID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
			return false
		}
		if errors.Is(err, registryapp.ErrRepositoryNotArchived) {
			writeError(w, r, http.StatusConflict, "repository_not_archived", err.Error())
			return false
		}
		if errors.Is(err, registryapp.ErrRepositoryNotEmpty) {
			writeError(w, r, http.StatusConflict, "repository_not_empty", err.Error())
			return false
		}
		if err != nil {
			s.internalError(w, r, err)
			return false
		}
		discovered, err := s.distributionClient.ListProjectRepositories(ctx, project.Slug)
		if err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "registry_unavailable", "Registry catalog must be available before removing a repository")
			return false
		}
		for _, name := range discovered {
			if name == repository.Name {
				writeError(w, r, http.StatusConflict, "repository_not_empty", "Remove all manifests before removing the logical repository")
				return false
			}
		}
		return true
	}
	if !validate(r.Context()) {
		return
	}

	// This handler is intentionally excluded from maintenanceGate's in-flight
	// count. It initiates the short drain below, then repeats validation while
	// new registry traffic and management mutations are blocked.
	drainCtx, drainCancel := context.WithTimeout(r.Context(), 2*time.Minute)
	endMaintenance, err := s.maintenance.Begin(drainCtx)
	drainCancel()
	if err != nil {
		w.Header().Set("Retry-After", "5")
		writeError(w, r, http.StatusServiceUnavailable, "maintenance_active", "Repository removal is waiting for active registry traffic to drain")
		return
	}
	defer endMaintenance()
	operationCtx, operationCancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer operationCancel()
	if !validate(operationCtx) {
		return
	}
	if err := s.repositories.Remove(operationCtx, project.ID, repositoryID, actor); err != nil {
		switch {
		case errors.Is(err, registryapp.ErrRepositoryNotArchived):
			writeError(w, r, http.StatusConflict, "repository_not_archived", err.Error())
		case errors.Is(err, registryapp.ErrRepositoryNotEmpty):
			writeError(w, r, http.StatusConflict, "repository_not_empty", err.Error())
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
		default:
			s.internalError(w, r, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRegistryPolicyPresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, registryapp.PolicyPresets())
}

func (s *Server) getRepositoryPolicies(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	repository, err := s.repositories.FindByID(
		r.Context(), foundation.ID(chi.URLParam(r, "repositoryId")),
	)
	if err != nil || repository.ProjectID != project.ID {
		writeError(w, r, http.StatusNotFound, "not_found", "Repository not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repositoryId": repository.ID,
		"version":      repository.PolicyVersion,
		"policies":     repository.Policies,
	})
}

func (s *Server) replaceRepositoryPolicies(w http.ResponseWriter, r *http.Request) {
	project, actor, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int                     `json:"expectedVersion"`
		Policies        []registrydomain.Policy `json:"policies"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	repository, err := s.repositories.ReplacePolicies(
		r.Context(), project.ID, foundation.ID(chi.URLParam(r, "repositoryId")),
		input.ExpectedVersion, input.Policies, actor,
	)
	if err != nil {
		writeError(w, r, http.StatusConflict, "repository_policies_not_updated", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repositoryId": repository.ID,
		"version":      repository.PolicyVersion,
		"policies":     repository.Policies,
	})
}

type artifactDeletionInput struct {
	Repository           string   `json:"repository"`
	Reference            string   `json:"reference"`
	Reason               string   `json:"reason"`
	ExpectedDigest       *string  `json:"expectedDigest"`
	ExpectedTags         []string `json:"expectedTags"`
	ExpectedChildDigests []string `json:"expectedChildDigests"`
}

func (s *Server) previewArtifactDeletion(w http.ResponseWriter, r *http.Request) {
	project, input, ok := s.authorizeArtifactDeletion(w, r)
	if !ok {
		return
	}
	preview, err := s.artifactDeletions.Preview(
		r.Context(), project.ID, project.Slug, input.Repository, input.Reference, input.Reason, false,
	)
	if err != nil {
		writeError(w, r, http.StatusConflict, "deletion_blocked", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	project, input, ok := s.authorizeArtifactDeletion(w, r)
	if !ok {
		return
	}
	deletion, err := s.artifactDeletions.Execute(
		r.Context(), project.ID, project.Slug, input.Repository, input.Reference,
		input.Reason, stringValue(input.ExpectedDigest), input.ExpectedTags,
		input.ExpectedChildDigests,
		principalForUser(userFromContext(r.Context())),
	)
	if err != nil {
		if deletion != nil {
			writeJSON(w, http.StatusInternalServerError, deletion)
			return
		}
		writeError(w, r, http.StatusConflict, "deletion_blocked", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deletion)
}

func (s *Server) listArtifactDeletions(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	repository := r.URL.Query().Get("repository")
	if repository == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", "Repository is required")
		return
	}
	request, _, err := pageRequest(r, "artifact-deletions:"+project.Slug+":"+repository)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	deletions, err := s.artifactDeletions.ListPage(r.Context(), project.ID, repository, request)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "artifact_deletions_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deletions)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Server) authorizeArtifactDeletion(
	w http.ResponseWriter,
	r *http.Request,
) (*projectdomain.Project, artifactDeletionInput, bool) {
	var input artifactDeletionInput
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return nil, input, false
	}
	user := userFromContext(r.Context())
	if !s.projects.CanManage(r.Context(), principalForUser(user), user.SystemAdmin, project) {
		writeError(w, r, http.StatusForbidden, "forbidden", "Project administrator permission required")
		return nil, input, false
	}
	if !decodeJSON(w, r, &input) {
		return nil, input, false
	}
	if input.Repository == "" || input.Reference == "" || strings.Contains(input.Repository, "..") {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Repository and reference are required")
		return nil, input, false
	}
	return project, input, true
}

func (s *Server) lifecycleProject(
	w http.ResponseWriter,
	r *http.Request,
	manage bool,
) (*projectdomain.Project, foundation.PrincipalRef, bool) {
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return nil, foundation.PrincipalRef{}, false
	}
	user := userFromContext(r.Context())
	actor := principalForUser(user)
	allowed := s.projects.CanView(r.Context(), actor, user.SystemAdmin, project)
	if manage {
		allowed = s.projects.CanManage(r.Context(), actor, user.SystemAdmin, project)
	}
	if !allowed {
		writeError(w, r, http.StatusForbidden, "forbidden", "Project administrator permission required")
		return nil, foundation.PrincipalRef{}, false
	}
	return project, actor, true
}

func (s *Server) listRepositoryInventory(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, false)
	if !ok {
		return
	}
	repository := r.URL.Query().Get("repository")
	if repository == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", "Repository is required")
		return
	}
	request, _, err := pageRequest(r, "repository-inventory:"+project.Slug+":"+repository)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	items, err := s.inventory.ListPage(r.Context(), project.ID, repository, request)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "inventory_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) reconcileRepositoryInventory(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Repository string `json:"repository"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Repository == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", "Repository is required")
		return
	}
	items, err := s.inventory.Reconcile(r.Context(), project.ID, project.Slug, input.Repository)
	if err != nil {
		writeError(w, r, http.StatusBadGateway, "inventory_reconciliation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createLifecyclePreview(w http.ResponseWriter, r *http.Request) {
	project, actor, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Repository string `json:"repository"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Repository == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", "Repository is required")
		return
	}
	preview, err := s.lifecycle.CreatePreview(r.Context(), project.ID, project.Slug, input.Repository, actor)
	if err != nil {
		writeError(w, r, http.StatusConflict, "lifecycle_preview_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, preview)
}

func (s *Server) getLifecyclePreview(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	preview, err := s.lifecycle.FindPreview(r.Context(), foundation.ID(chi.URLParam(r, "previewId")))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Lifecycle preview not found")
		return
	}
	repository, err := s.repositories.FindByID(r.Context(), preview.RepositoryID)
	if err != nil || repository.ProjectID != project.ID {
		writeError(w, r, http.StatusNotFound, "not_found", "Lifecycle preview not found")
		return
	}
	preview.Repository = repository.Name
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) createLifecycleRun(w http.ResponseWriter, r *http.Request) {
	project, actor, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	var input struct {
		PreviewID string `json:"previewId"`
		Reason    string `json:"reason"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.PreviewID == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_preview", "Preview ID is required")
		return
	}
	run, err := s.lifecycle.Execute(
		r.Context(), project.ID, project.Slug, foundation.ID(input.PreviewID), input.Reason, actor,
	)
	if err != nil {
		writeError(w, r, http.StatusConflict, "lifecycle_execution_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) listLifecycleRuns(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	repository := r.URL.Query().Get("repository")
	if repository == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", "Repository is required")
		return
	}
	request, _, err := pageRequest(r, "lifecycle-runs:"+project.Slug+":"+repository)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	runs, err := s.lifecycle.ListRunsPage(r.Context(), project.ID, repository, request)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "lifecycle_runs_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getLifecycleRun(w http.ResponseWriter, r *http.Request) {
	project, _, ok := s.lifecycleProject(w, r, true)
	if !ok {
		return
	}
	run, err := s.lifecycle.FindRun(r.Context(), foundation.ID(chi.URLParam(r, "runId")))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Lifecycle run not found")
		return
	}
	repository, err := s.repositories.FindByID(r.Context(), run.RepositoryID)
	if err != nil || repository.ProjectID != project.ID {
		writeError(w, r, http.StatusNotFound, "not_found", "Lifecycle run not found")
		return
	}
	run.Repository = repository.Name
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.Find(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	user := userFromContext(r.Context())
	if !s.projects.CanView(r.Context(), principalForUser(user), user.SystemAdmin, project) {
		writeError(w, r, http.StatusNotFound, "not_found", "Project not found")
		return
	}
	repository := r.URL.Query().Get("repository")
	if repository == "" || strings.HasPrefix(repository, "/") || strings.Contains(repository, "..") {
		writeError(w, r, http.StatusBadRequest, "invalid_repository", "Repository path is invalid")
		return
	}
	scope := "repository-tags:" + project.Slug + ":" + repository
	request, _, err := pageRequest(r, scope)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
		return
	}
	marker := ""
	if request.Cursor != "" {
		cursor, decodeErr := foundation.DecodePageCursor(request.Cursor, scope)
		if decodeErr != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
			return
		}
		marker = cursor.Marker
		if marker == "" {
			writeError(w, r, http.StatusBadRequest, "invalid_cursor", "Page cursor or limit is invalid")
			return
		}
	}
	tags, err := s.distributionClient.ListLiveTagsPage(r.Context(), project.Slug+"/"+repository, request.Limit, marker)
	if err != nil {
		writeError(w, r, http.StatusBadGateway, "registry_unavailable", "Registry metadata is unavailable")
		return
	}
	result := foundation.PageResult[string]{Items: tags.Tags}
	if tags.NextMarker != "" {
		result.NextCursor, _ = foundation.EncodePageCursor(foundation.PageCursor{Scope: scope, Marker: tags.NextMarker})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) exchangeRegistryToken(w http.ResponseWriter, r *http.Request) {
	limiterKey := authenticationLimiterKey(r)
	if allowed, retryAfter := s.registryLimiter.allow(limiterKey, time.Now()); !allowed {
		writeRateLimitError(w, r, retryAfter)
		return
	}
	username, rawToken, ok := r.BasicAuth()
	if !ok {
		s.registryLimiter.failure(limiterKey, time.Now())
		_ = s.recordAudit(r, anonymousPrincipal(), constants.AuditRegistryAuthFailed, constants.AuditResourceAuthentication, "", map[string]any{"reason": "missing_basic_auth"})
		w.Header().Set("WWW-Authenticate", `Basic realm="Grom Registry"`)
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "API token required")
		return
	}
	principal, err := s.identity.AuthenticateRegistry(r.Context(), username, rawToken)
	if err != nil {
		s.registryLimiter.failure(limiterKey, time.Now())
		_ = s.recordAudit(r, anonymousPrincipal(), constants.AuditRegistryAuthFailed, constants.AuditResourceAuthentication, "", map[string]any{"reason": "invalid_credentials", "username": username})
		w.Header().Set("WWW-Authenticate", `Basic realm="Grom Registry"`)
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Invalid registry credentials")
		return
	}
	s.registryLimiter.success(limiterKey)
	token, expiresIn, issuedAt, err := s.registryTokens.Issue(
		r.Context(), username, principal, r.URL.Query().Get("service"), r.URL.Query()["scope"],
	)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token, "access_token": token, "expires_in": expiresIn, "issued_at": issuedAt,
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(constants.SessionCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Sign in required")
			return
		}
		user, err := s.identity.AuthenticateSession(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Session expired")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentUserKey{}, user)))
	})
}

func (s *Server) viewerReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := userFromContext(r.Context())
		viewerOwnCredentialAction := r.URL.Path == "/api/v1/me/password" || strings.HasPrefix(r.URL.Path, "/api/v1/me/registry-tokens")
		if user != nil && user.SystemViewer && r.Method != http.MethodGet && r.Method != http.MethodHead && !viewerOwnCredentialAction {
			writeError(w, r, http.StatusForbidden, "forbidden", "Installation viewer accounts have read-only access")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recordAudit(r *http.Request, actor foundation.PrincipalRef, action, resourceKind string, resourceID foundation.ID, metadata map[string]any) error {
	if s.audit == nil {
		return nil
	}
	if err := s.audit.Record(r.Context(), actor, action, resourceKind, resourceID, metadata); err != nil {
		s.logger.Error("record audit event", "action", action, "error", err)
		return err
	}
	return nil
}

func anonymousPrincipal() foundation.PrincipalRef {
	return foundation.PrincipalRef{Kind: "anonymous", ID: "anonymous"}
}

func (s *Server) maintenanceGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresQuiescenceTracking(r) {
			next.ServeHTTP(w, r)
			return
		}
		leave, allowed := s.maintenance.Enter()
		if !allowed {
			w.Header().Set("Retry-After", "5")
			writeError(w, r, http.StatusServiceUnavailable, "maintenance_active", "Backup maintenance is in progress; retry this operation shortly")
			return
		}
		defer leave()
		next.ServeHTTP(w, r)
	})
}

func requiresQuiescenceTracking(r *http.Request) bool {
	normalizedPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	if r.Method == http.MethodPost && (normalizedPath == "/api/v1/backups" || normalizedPath == "/api/v1/garbage-collections") {
		return false
	}
	if r.Method == http.MethodDelete {
		segments := strings.Split(normalizedPath, "/")
		if len(segments) == 7 && segments[1] == "api" && segments[2] == "v1" &&
			segments[3] == "projects" && segments[5] == "repositories" &&
			segments[4] != "" && segments[6] != "" {
			return false
		}
	}
	if normalizedPath == "/auth/token" || normalizedPath == "/v2" ||
		strings.HasPrefix(normalizedPath, "/v2/") {
		return true
	}
	return r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
}

func (s *Server) principalExists(ctx context.Context, principal foundation.PrincipalRef) bool {
	switch principal.Kind {
	case constants.PrincipalUser:
		_, err := s.identity.FindUser(ctx, principal.ID)
		return err == nil
	case constants.PrincipalServiceAccount:
		_, err := s.identity.FindServiceAccount(ctx, principal.ID)
		return err == nil
	default:
		return false
	}
}

func (s *Server) originGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			if _, err := r.Cookie(constants.SessionCookieName); err == nil {
				writeError(w, r, http.StatusForbidden, "missing_origin", "Origin is required for authenticated browser mutations")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || !s.allowedOrigin(parsed, r) {
			writeError(w, r, http.StatusForbidden, "invalid_origin", "Request origin is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowedOrigin(origin *url.URL, r *http.Request) bool {
	if strings.EqualFold(origin.Scheme, s.publicURL.Scheme) &&
		strings.EqualFold(origin.Host, s.publicURL.Host) {
		return true
	}
	if !isLoopbackHostname(s.publicURL.Hostname()) ||
		!isLoopbackHostname(origin.Hostname()) ||
		!strings.EqualFold(origin.Scheme, requestScheme(r)) {
		return false
	}
	if strings.EqualFold(origin.Host, r.Host) {
		return true
	}
	// Vite's development proxy can rewrite the request Host to the backend
	// target. Keep the development exception narrowly limited to its fixed
	// loopback port rather than accepting arbitrary loopback origins.
	return s.deploymentProfile == "development" && origin.Port() == "5173"
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func isLoopbackHostname(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path,
			"status", wrapped.Status(), "duration", time.Since(started),
			"remote_ip", authenticationLimiterKey(r), "request_id", middleware.GetReqID(r.Context()))
	})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("request failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
	writeError(w, r, http.StatusInternalServerError, "internal_error", "The request could not be completed")
}

func (s *Server) apiDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><title>Grom API</title>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<style>body{margin:0;background:#0b0d10}</style></head><body>
<script id="api-reference" data-url="/api/openapi.yaml"></script>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script></body></html>`))
}

func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") || strings.HasPrefix(r.URL.Path, "/v2/") {
		writeError(w, r, http.StatusNotFound, "not_found", "Endpoint not found")
		return
	}
	dist, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if r.URL.Path != "/" {
		if file, openErr := dist.Open(strings.TrimPrefix(r.URL.Path, "/")); openErr == nil {
			_ = file.Close()
			http.FileServer(http.FS(dist)).ServeHTTP(w, r)
			return
		}
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

func principalForUser(user *identitydomain.User) foundation.PrincipalRef {
	return foundation.PrincipalRef{Kind: constants.PrincipalUser, ID: user.ID}
}

func userFromContext(ctx context.Context) *identitydomain.User {
	user, _ := ctx.Value(currentUserKey{}).(*identitydomain.User)
	return user
}

func requireSystemAdmin(w http.ResponseWriter, r *http.Request) bool {
	user := userFromContext(r.Context())
	if user == nil || !user.SystemAdmin {
		writeError(w, r, http.StatusForbidden, "forbidden", "Administrator permission required")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Request body is invalid")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]string{
		"code": code, "message": message, "requestId": middleware.GetReqID(r.Context()),
	})
}

var _ = errors.Is
var _ = sql.ErrNoRows
