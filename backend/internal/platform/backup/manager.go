package backup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
)

type Operation struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Message     *string    `json:"message,omitempty"`
}

type Overview struct {
	Available       bool       `json:"available"`
	ActiveOperation *Operation `json:"activeOperation,omitempty"`
	Backups         []Summary  `json:"backups"`
	TotalBackups    int        `json:"totalBackups"`
	PageSize        int        `json:"pageSize"`
	NextCursor      *string    `json:"nextCursor,omitempty"`
}

var ErrOperationInProgress = errors.New("a backup operation is already running")

const agentAvailabilityTimeout = 5 * time.Second

type Agent interface {
	List(context.Context) ([]Summary, error)
	Create(context.Context, AgentCreateRequest) (Summary, error)
	Download(context.Context, string) (*http.Response, error)
	Delete(context.Context, string) error
	Available(context.Context) bool
}

type Manager struct {
	mu                sync.Mutex
	agent             Agent
	maintenance       *maintenance.Controller
	checkpoint        func(context.Context) error
	onComplete        func(context.Context, Summary) error
	gromVersion       string
	deploymentProfile string
	database          string
	lastOperation     *Operation
	deleting          bool
}

func NewManager(
	agent Agent,
	controller *maintenance.Controller,
	checkpoint func(context.Context) error,
	gromVersion, deploymentProfile string,
	onComplete func(context.Context, Summary) error,
) *Manager {
	return NewManagerWithDatabase(agent, controller, checkpoint, gromVersion, deploymentProfile, "sqlite", onComplete)
}

func NewManagerWithDatabase(
	agent Agent,
	controller *maintenance.Controller,
	checkpoint func(context.Context) error,
	gromVersion, deploymentProfile, database string,
	onComplete func(context.Context, Summary) error,
) *Manager {
	return &Manager{
		agent: agent, maintenance: controller, checkpoint: checkpoint,
		gromVersion: gromVersion, deploymentProfile: deploymentProfile, database: database, onComplete: onComplete,
	}
}

func (manager *Manager) Overview(ctx context.Context, cursor string) (Overview, error) {
	backups, err := manager.agent.List(ctx)
	available := err == nil
	if backups == nil {
		backups = []Summary{}
	}
	page, pageErr := Paginate(backups, cursor)
	if pageErr != nil {
		return Overview{}, pageErr
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	var operation *Operation
	if manager.lastOperation != nil {
		copy := *manager.lastOperation
		operation = &copy
	}
	var nextCursor *string
	if page.NextCursor != "" {
		nextCursor = &page.NextCursor
	}
	return Overview{
		Available: available, ActiveOperation: operation, Backups: page.Items,
		TotalBackups: page.Total, PageSize: PageSize, NextCursor: nextCursor,
	}, nil
}

func (manager *Manager) Start() (Operation, error) {
	manager.mu.Lock()
	if manager.deleting || manager.lastOperation != nil &&
		manager.lastOperation.Status != "complete" && manager.lastOperation.Status != "failed" {
		manager.mu.Unlock()
		return Operation{}, ErrOperationInProgress
	}
	manager.mu.Unlock()

	availabilityContext, cancel := context.WithTimeout(context.Background(), agentAvailabilityTimeout)
	available := manager.agent.Available(availabilityContext)
	cancel()
	if !available {
		return Operation{}, fmt.Errorf("backup agent is unavailable")
	}

	manager.mu.Lock()
	if manager.deleting || manager.lastOperation != nil &&
		manager.lastOperation.Status != "complete" && manager.lastOperation.Status != "failed" {
		manager.mu.Unlock()
		return Operation{}, ErrOperationInProgress
	}
	operation := Operation{ID: uuid.NewString(), Status: "starting", StartedAt: time.Now().UTC()}
	storedOperation := operation
	manager.lastOperation = &storedOperation
	manager.mu.Unlock()
	go manager.run(operation.ID)
	return operation, nil
}

func (manager *Manager) Download(ctx context.Context, backupID string) (*http.Response, error) {
	return manager.agent.Download(ctx, backupID)
}

func (manager *Manager) Delete(ctx context.Context, backupID string) error {
	manager.mu.Lock()
	if manager.deleting || manager.lastOperation != nil &&
		manager.lastOperation.Status != "complete" && manager.lastOperation.Status != "failed" {
		manager.mu.Unlock()
		return ErrOperationInProgress
	}
	manager.deleting = true
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		manager.deleting = false
		manager.mu.Unlock()
	}()
	if err := manager.agent.Delete(ctx, backupID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.ErrNotExist
		}
		return fmt.Errorf("delete backup through agent: %w", err)
	}
	return nil
}

func (manager *Manager) run(operationID string) {
	manager.setStatus(operationID, "quiescing", nil, false)
	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	endMaintenance, err := manager.maintenance.Begin(drainCtx)
	cancel()
	if err != nil {
		manager.fail(operationID, "Could not enter maintenance mode")
		return
	}
	defer endMaintenance()
	if err := manager.checkpoint(context.Background()); err != nil {
		manager.fail(operationID, "Could not create a consistent database checkpoint")
		return
	}
	manager.setStatus(operationID, "creating", nil, false)
	summary, err := manager.agent.Create(context.Background(), AgentCreateRequest{
		GromVersion: manager.gromVersion, DeploymentProfile: manager.deploymentProfile,
		Consistency: "quiesced",
		Database:    manager.database,
	})
	if err != nil {
		manager.fail(operationID, "Backup creation failed")
		return
	}
	if manager.onComplete != nil {
		if err := manager.onComplete(context.Background(), summary); err != nil {
			manager.fail(operationID, "Backup completed but its audit event could not be recorded")
			return
		}
	}
	manager.setStatus(operationID, "complete", nil, true)
}

func (manager *Manager) fail(id, message string) {
	manager.setStatus(id, "failed", &message, true)
}

func (manager *Manager) setStatus(id, status string, message *string, complete bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.lastOperation == nil || manager.lastOperation.ID != id {
		return
	}
	manager.lastOperation.Status = status
	manager.lastOperation.Message = message
	if complete {
		now := time.Now().UTC()
		manager.lastOperation.CompletedAt = &now
	}
}
