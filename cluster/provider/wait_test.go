package provider

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type errorWaiter struct {
	counter int
	lock    *sync.Mutex
}

func (w errorWaiter) wait() error {
	w.lock.Lock()
	w.counter++
	errorNum := w.counter
	w.lock.Unlock()
	return fmt.Errorf("%d", errorNum)
}

func TestWaiterError(t *testing.T) {
	t.Parallel()

	batchWaiter := newWaiter()
	defer close(batchWaiter.waiters)

	testWaiter := errorWaiter{
		lock: &sync.Mutex{},
	}
	for i := 0; i < 10; i++ {
		batchWaiter.waiters <- testWaiter
	}

	err := batchWaiter.wait()
	if err.Error() != "1" {
		t.Errorf("BatchWaiter returned wrong error: %s", err.Error())
	}
}

type waitSecond struct{}

func (w waitSecond) wait() error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

func TestWaitsInParallel(t *testing.T) {
	t.Parallel()

	startTime := time.Now()
	batchWaiter := newWaiter()
	defer close(batchWaiter.waiters)
	for i := 0; i < 100; i++ {
		batchWaiter.waiters <- waitSecond{}
	}
	batchWaiter.wait()
	endTime := time.Now()

	assert.WithinDuration(t, startTime, endTime, time.Second,
		"The batch waiter should wait in parallel, "+
			"and thus finish in no more than 5 seconds.")
}
