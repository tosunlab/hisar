package agent

import (
	"sync"
)

// presenter runs window and event calls in order on its own goroutine, so a window that cannot be shown never holds up a request.
type presenter struct {
	mutex     sync.Mutex
	allAction []func()
	isRunning bool
	idle      sync.Cond
}

func (presenter *presenter) present(action func()) {
	presenter.mutex.Lock()
	defer presenter.mutex.Unlock()
	presenter.allAction = append(presenter.allAction, action)
	if !presenter.isRunning {
		presenter.isRunning = true
		go presenter.runAllAction()
	}
}

func (presenter *presenter) runAllAction() {
	for {
		presenter.mutex.Lock()
		if len(presenter.allAction) == 0 {
			presenter.isRunning = false
			presenter.idle.Broadcast()
			presenter.mutex.Unlock()
			return
		}
		action := presenter.allAction[0]
		presenter.allAction = presenter.allAction[1:]
		presenter.mutex.Unlock()
		action()
	}
}

func (presenter *presenter) waitUntilIdle() {
	presenter.mutex.Lock()
	defer presenter.mutex.Unlock()
	for presenter.isRunning {
		presenter.idle.Wait()
	}
}
