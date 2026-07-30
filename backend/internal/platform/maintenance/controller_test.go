package maintenance

import (
	"context"
	"testing"
	"time"
)

func TestBeginBlocksNewWorkAndWaitsForExistingWork(t *testing.T) {
	controller := New()
	leave, ok := controller.Enter()
	if !ok {
		t.Fatal("initial work was rejected")
	}
	result := make(chan func(), 1)
	go func() {
		end, err := controller.Begin(context.Background())
		if err != nil {
			result <- nil
			return
		}
		result <- end
	}()
	deadline := time.Now().Add(time.Second)
	for !controller.Active() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, allowed := controller.Enter(); allowed {
		t.Fatal("new work was accepted during maintenance")
	}
	select {
	case <-result:
		t.Fatal("maintenance began before existing work drained")
	default:
	}
	leave()
	end := <-result
	if end == nil {
		t.Fatal("maintenance failed to begin")
	}
	end()
	if !func() bool { _, allowed := controller.Enter(); return allowed }() {
		t.Fatal("work remained blocked after maintenance")
	}
}
