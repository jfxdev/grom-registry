package maintenance

import (
	"context"
	"errors"
	"sync"
)

var ErrActive = errors.New("maintenance is already active")

type Controller struct {
	mu       sync.Mutex
	active   bool
	inFlight int
	zero     chan struct{}
}

func New() *Controller {
	zero := make(chan struct{})
	close(zero)
	return &Controller{zero: zero}
}

func (controller *Controller) Enter() (func(), bool) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.active {
		return nil, false
	}
	if controller.inFlight == 0 {
		controller.zero = make(chan struct{})
	}
	controller.inFlight++
	var once sync.Once
	return func() {
		once.Do(func() {
			controller.mu.Lock()
			defer controller.mu.Unlock()
			controller.inFlight--
			if controller.inFlight == 0 {
				close(controller.zero)
			}
		})
	}, true
}

func (controller *Controller) Begin(ctx context.Context) (func(), error) {
	controller.mu.Lock()
	if controller.active {
		controller.mu.Unlock()
		return nil, ErrActive
	}
	controller.active = true
	zero := controller.zero
	controller.mu.Unlock()

	select {
	case <-zero:
		var once sync.Once
		return func() {
			once.Do(func() {
				controller.mu.Lock()
				controller.active = false
				controller.mu.Unlock()
			})
		}, nil
	case <-ctx.Done():
		controller.mu.Lock()
		controller.active = false
		controller.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (controller *Controller) Active() bool {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return controller.active
}
