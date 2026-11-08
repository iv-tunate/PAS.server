package mailer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Job is one email waiting to go out. OnResult (optional) is called from the
// worker once the email has either been sent (nil) or given up on (error), so
// the caller can record "delivered" without waiting around for it.
type Job struct {
	Msg      Message
	OnResult func(err error)
}

// Outbox is the one place emails are sent from. Callers drop a Job onto a
// buffered channel and carry on; a small pool of workers reads from the
// channel, talks to Brevo, and retries a couple of times if it fails.
//
//	request / goroutine  --Job-->  [ channel ]  -->  worker x N  -->  Brevo
//
// Why a channel instead of "go send()": it caps how many emails are in flight
// at once (so a 2,000-person newsletter cannot open 2,000 connections), and
// it gives shutdown something to wait on so queued emails are not lost.
type Outbox struct {
	mailer Mailer
	jobs   chan Job
	log    *slog.Logger
	wg     sync.WaitGroup

	mu     sync.RWMutex // guards closed, so nothing sends on a closed channel
	closed bool
}

// retryDelays: how long a worker waits before each retry. Three tries total.
var retryDelays = []time.Duration{3 * time.Second, 12 * time.Second}

func NewOutbox(m Mailer, log *slog.Logger, workers, buffer int) *Outbox {
	o := &Outbox{mailer: m, jobs: make(chan Job, buffer), log: log}
	for i := 0; i < workers; i++ {
		o.wg.Add(1)
		go o.worker()
	}
	return o
}

// TrySend queues a job without ever blocking. It returns false when the queue
// is full, and the caller decides what to do (orders fall back to the
// delivery sweeper, which picks them up a few minutes later).
func (o *Outbox) TrySend(j Job) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.closed {
		return false
	}
	select {
	case o.jobs <- j:
		return true
	default:
		return false
	}
}

// Send queues a job and waits for room (or for ctx to end). Newsletter
// broadcasts use this: they run in their own goroutine, so waiting is fine
// and it spreads a big send out instead of dropping recipients.
func (o *Outbox) Send(ctx context.Context, j Job) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.closed {
		return false
	}
	select {
	case o.jobs <- j:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close stops accepting jobs, lets the workers finish what is queued, and
// returns when they are done or ctx ends, whichever comes first.
func (o *Outbox) Close(ctx context.Context) {
	o.mu.Lock()
	if !o.closed {
		o.closed = true
		close(o.jobs)
	}
	o.mu.Unlock()

	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		o.log.Warn("outbox did not finish before shutdown; some queued emails were dropped")
	}
}

func (o *Outbox) worker() {
	defer o.wg.Done()
	for job := range o.jobs {
		err := o.mailer.Send(job.Msg)
		for _, wait := range retryDelays {
			if err == nil || isPermanent(err) {
				break
			}
			o.log.Warn("email failed, will retry", "to", job.Msg.ToEmail, "error", err, "in", wait)
			time.Sleep(wait)
			err = o.mailer.Send(job.Msg)
		}
		if err != nil {
			o.log.Error("email gave up", "to", job.Msg.ToEmail, "subject", job.Msg.Subject, "error", err)
		}
		if job.OnResult != nil {
			job.OnResult(err)
		}
	}
}

// isPermanent is true for errors a retry cannot fix, such as Brevo rejecting
// the address (4xx). Rate limits (429) and server errors are worth retrying.
func isPermanent(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code >= 400 && se.Code < 500 && se.Code != 429
}
