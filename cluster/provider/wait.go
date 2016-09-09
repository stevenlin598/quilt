package provider

import (
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// A waiter blocks execution until some condition is satisfied.
type waiter interface {
	wait() error
}

type awsWaitRequest struct {
	session EC2Client
	ids     []string
}

type spotBooted awsWaitRequest
type instanceBooted awsWaitRequest
type instanceStopped awsWaitRequest

func (w spotBooted) wait() error {
	return w.session.WaitUntilSpotInstanceRequestFulfilled(
		&ec2.DescribeSpotInstanceRequestsInput{
			SpotInstanceRequestIds: aws.StringSlice(w.ids),
		})
}

func (w instanceBooted) wait() error {
	return w.session.WaitUntilInstanceExists(
		&ec2.DescribeInstancesInput{
			InstanceIds: aws.StringSlice(w.ids),
		})
}

func (w instanceStopped) wait() error {
	return w.session.WaitUntilInstanceTerminated(
		&ec2.DescribeInstancesInput{
			InstanceIds: aws.StringSlice(w.ids),
		})
}

// batchWaiter allows waiting on multiple waiters in parallel.
// The `waiters` channel must be closed in order for the listener to exit.
type batchWaiter struct {
	err       chan error
	waiters   chan waiter
	waitGroup *sync.WaitGroup
}

// wait waits until all waiters have returned, and returns the first error,
// if any.
func (bw *batchWaiter) wait() error {
	bw.waitGroup.Wait()
	select {
	case err := <-bw.err:
		return err
	default:
		return nil
	}
}

func (bw *batchWaiter) add(w waiter) {
	bw.waitGroup.Add(1)
	bw.waiters <- w
}

// listener spawns a Go routine that runs each waiter in parallel, and writes
// the result to the error channel.
func (bw *batchWaiter) listener() {
	for req := range bw.waiters {
		go func(req waiter) {
			defer bw.waitGroup.Done()
			if err := req.wait(); err != nil {
				// Only write the error if we're the first one.
				select {
				case bw.err <- err:
				default:
				}
			}
		}(req)
	}
}

// newWaiter creates a new batchWaiter, and starts the listener.
func newWaiter() batchWaiter {
	w := batchWaiter{
		err:       make(chan error, 1),
		waiters:   make(chan waiter),
		waitGroup: new(sync.WaitGroup),
	}
	go w.listener()
	return w
}
