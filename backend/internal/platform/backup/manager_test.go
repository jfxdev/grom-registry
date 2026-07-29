package backup

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/platform/maintenance"
)

type fakeAgent struct {
	mu      sync.Mutex
	created bool
}

func (agent *fakeAgent) List(context.Context) ([]Summary, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if !agent.created {
		return []Summary{}, nil
	}
	return []Summary{{BackupID: "backup-id", CreatedAt: time.Now().UTC().Format(time.RFC3339)}}, nil
}

func (agent *fakeAgent) Create(context.Context, AgentCreateRequest) (Summary, error) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	agent.created = true
	return Summary{BackupID: "backup-id", GromVersion: "test", TotalBytes: 12}, nil
}

func (agent *fakeAgent) Download(context.Context, string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("bundle"))}, nil
}

func (agent *fakeAgent) Delete(context.Context, string) error {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	agent.created = false
	return nil
}

func (*fakeAgent) Available(context.Context) bool { return true }

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
