package backup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
)

type fakeAgent struct {
	mu          sync.Mutex
	created     bool
	unavailable bool
	listErr     error
	createErr   error
	downloadErr error
	deleteErr   error
	availableAt chan struct{}
	availableOn chan struct{}
}

func (agent *fakeAgent) List(context.Context) ([]Summary, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.listErr != nil {
		return nil, agent.listErr
	}
	if !agent.created {
		return []Summary{}, nil
	}
	return []Summary{{BackupID: "backup-id", CreatedAt: time.Now().UTC().Format(time.RFC3339)}}, nil
}

func (agent *fakeAgent) Create(context.Context, AgentCreateRequest) (Summary, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.createErr != nil {
		return Summary{}, agent.createErr
	}
	agent.created = true
	return Summary{BackupID: "backup-id", GromVersion: "test", TotalBytes: 12}, nil
}

func (agent *fakeAgent) Download(context.Context, string) (*http.Response, error) {
	if agent.downloadErr != nil {
		return nil, agent.downloadErr
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("bundle"))}, nil
}

func (agent *fakeAgent) Delete(context.Context, string) error {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.deleteErr != nil {
		return agent.deleteErr
	}
	agent.created = false
	return nil
}

func (agent *fakeAgent) Available(ctx context.Context) bool {
	if agent.availableAt != nil {
		close(agent.availableAt)
		select {
		case <-agent.availableOn:
		case <-ctx.Done():
			return false
		}
	}
	return !agent.unavailable
}

func TestManagerQuiescesWritesBeforeCheckpointAndCreate(t *testing.T) {
	controller := maintenance.New()
	leave, ok := controller.Enter()
	if !ok {
		t.Fatal("could not register in-flight write")
	}
	checkpointed := make(chan struct{}, 1)
	completed := make(chan struct{}, 1)
	manager := NewManager(
		&fakeAgent{}, controller,
		func(context.Context) error {
			checkpointed <- struct{}{}
			return nil
		},
		"test", "development",
		func(context.Context, Summary) error {
			completed <- struct{}{}
			return nil
		},
	)
	if _, err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-checkpointed:
		t.Fatal("checkpoint ran before existing write drained")
	case <-time.After(20 * time.Millisecond):
	}
	leave()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("backup operation did not complete")
	}
	overview, err := manager.Overview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if overview.ActiveOperation == nil || overview.ActiveOperation.Status != "complete" || len(overview.Backups) != 1 {
		t.Fatalf("unexpected manager overview: %#v", overview)
	}
}

func TestManagerOverviewAndDelegatedOperations(t *testing.T) {
	agent := &fakeAgent{listErr: errors.New("offline")}
	manager := NewManager(agent, maintenance.New(), func(context.Context) error { return nil }, "test", "development", nil)
	overview, err := manager.Overview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if overview.Available || overview.Backups == nil || overview.PageSize != PageSize {
		t.Fatalf("unexpected unavailable overview: %#v", overview)
	}
	if _, err := manager.Overview(context.Background(), "invalid"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected invalid cursor error, got %v", err)
	}

	agent.listErr = nil
	response, err := manager.Download(context.Background(), "backup")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(raw) != "bundle" {
		t.Fatalf("unexpected download: %q error=%v", raw, err)
	}
	agent.downloadErr = errors.New("download failed")
	if _, err := manager.Download(context.Background(), "backup"); err == nil {
		t.Fatal("expected delegated download failure")
	}

	agent.downloadErr = nil
	if err := manager.Delete(context.Background(), "backup"); err != nil {
		t.Fatal(err)
	}
	agent.deleteErr = os.ErrNotExist
	if err := manager.Delete(context.Background(), "backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing backup, got %v", err)
	}
	agent.deleteErr = errors.New("delete failed")
	if err := manager.Delete(context.Background(), "backup"); err == nil ||
		!strings.Contains(err.Error(), "through agent") {
		t.Fatalf("expected wrapped delete failure, got %v", err)
	}
}

func TestManagerRejectsUnavailableAndConcurrentOperations(t *testing.T) {
	agent := &fakeAgent{unavailable: true}
	manager := NewManager(agent, maintenance.New(), func(context.Context) error { return nil }, "test", "development", nil)
	if _, err := manager.Start(); err == nil {
		t.Fatal("expected unavailable agent rejection")
	}

	manager.mu.Lock()
	manager.deleting = true
	manager.mu.Unlock()
	if _, err := manager.Start(); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected create/delete conflict, got %v", err)
	}
	if err := manager.Delete(context.Background(), "backup"); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected concurrent delete conflict, got %v", err)
	}

	manager.mu.Lock()
	manager.deleting = false
	manager.lastOperation = &Operation{ID: "active", Status: "creating"}
	manager.mu.Unlock()
	if _, err := manager.Start(); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected active create conflict, got %v", err)
	}
	if err := manager.Delete(context.Background(), "backup"); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected active operation delete conflict, got %v", err)
	}
}

func TestManagerDoesNotHoldMutexDuringAgentAvailabilityCheck(t *testing.T) {
	agent := &fakeAgent{availableAt: make(chan struct{}), availableOn: make(chan struct{})}
	manager := NewManager(agent, maintenance.New(), func(context.Context) error { return nil }, "test", "development", nil)
	result := make(chan error, 1)
	go func() {
		_, err := manager.Start()
		result <- err
	}()
	<-agent.availableAt

	acquired := make(chan struct{})
	go func() {
		manager.mu.Lock()
		manager.mu.Unlock()
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("manager mutex remained locked during agent availability check")
	}
	close(agent.availableOn)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestManagerRecordsCheckpointCreateAndAuditFailures(t *testing.T) {
	for _, test := range []struct {
		name       string
		agent      *fakeAgent
		checkpoint func(context.Context) error
		onComplete func(context.Context, Summary) error
		message    string
	}{
		{
			name:  "checkpoint",
			agent: &fakeAgent{},
			checkpoint: func(context.Context) error {
				return errors.New("checkpoint failed")
			},
			message: "consistent database checkpoint",
		},
		{
			name:       "create",
			agent:      &fakeAgent{createErr: errors.New("create failed")},
			checkpoint: func(context.Context) error { return nil },
			message:    "Backup creation failed",
		},
		{
			name:       "audit",
			agent:      &fakeAgent{},
			checkpoint: func(context.Context) error { return nil },
			onComplete: func(context.Context, Summary) error {
				return errors.New("audit failed")
			},
			message: "audit event",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager(
				test.agent,
				maintenance.New(),
				test.checkpoint,
				"test",
				"development",
				test.onComplete,
			)
			operation, err := manager.Start()
			if err != nil {
				t.Fatal(err)
			}
			failed := waitForManagerOperation(t, manager, operation.ID)
			if failed.Status != "failed" || failed.CompletedAt == nil ||
				failed.Message == nil || !strings.Contains(*failed.Message, test.message) {
				t.Fatalf("unexpected failed operation: %#v", failed)
			}
		})
	}
}

func TestManagerIgnoresStaleStatusUpdates(t *testing.T) {
	manager := NewManager(&fakeAgent{}, maintenance.New(), func(context.Context) error { return nil }, "test", "development", nil)
	manager.lastOperation = &Operation{ID: "current", Status: "starting"}
	manager.setStatus("stale", "complete", nil, true)
	if manager.lastOperation.Status != "starting" || manager.lastOperation.CompletedAt != nil {
		t.Fatalf("stale update changed current operation: %#v", manager.lastOperation)
	}
}

func waitForManagerOperation(t *testing.T, manager *Manager, operationID string) Operation {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		if manager.lastOperation != nil &&
			manager.lastOperation.ID == operationID &&
			(manager.lastOperation.Status == "complete" || manager.lastOperation.Status == "failed") {
			result := *manager.lastOperation
			manager.mu.Unlock()
			return result
		}
		manager.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("operation %s did not finish", operationID)
	return Operation{}
}
