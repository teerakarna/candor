package notify

import (
	"context"
	"fmt"
	"sync"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// asyncWorkers bounds how many Send calls run concurrently across every SendAsync caller in the
// process - internal/signal.Ingest (per accepted signal) and internal/controller.DigestRunnable
// (per policy, per tick) both queue onto the same pool, deliberately: one shared, small, fixed
// budget for "webhook sends in flight" is easier to reason about than a separate cap per caller,
// and this is genuinely one resource (outbound HTTP connections to notification endpoints), not
// two.
const asyncWorkers = 8

// asyncQueueSize bounds how many queued sends SendAsync will hold before it starts dropping new
// ones rather than growing without limit - see SendAsync's own doc comment for why dropping,
// not blocking the caller, is the right response to a full queue.
const asyncQueueSize = 256

type asyncJob struct {
	url     string
	payload any
	done    func(err error, dropped bool)
}

var (
	asyncOnce  sync.Once
	asyncQueue chan asyncJob
)

func startAsyncWorkers() {
	asyncQueue = make(chan asyncJob, asyncQueueSize)
	for range asyncWorkers {
		go func() {
			for job := range asyncQueue {
				runJob(job)
			}
		}()
	}
}

// runJob sends job and, no matter what happens, calls job.done exactly once - split into two
// independently-recovered steps, not one `job.done(sendAndRecover(job), false)` combined call:
// Go evaluates a call's arguments before invoking it, so a single combined call would let a panic
// during the send argument's own evaluation skip calling done entirely, even with a recover
// wrapping the whole thing - a caller relying on done always firing (a sync.WaitGroup, say) would
// hang forever with nothing but a log line to explain why. send's own recover guarantees an err
// value one way or another; callDone's separate recover then guards against the caller's own
// callback panicking too, on either that path.
func runJob(job asyncJob) {
	callDone(job.done, send(job), false)
}

// send runs job's actual webhook POST, recovering from and reporting any panic as an error rather
// than letting it propagate - these workers are long-lived, unsupervised background goroutines,
// unlike every other code path in this project, which already runs behind a per-call recover
// (net/http's own per-request recovery for the webhook receiver's HTTP handler, controller-runtime's
// own per-reconcile recovery for the Trivy reconciler's call into Ingest). An unrecovered panic on
// a bare goroutine crashes the whole process, not just this one notification - a far worse blast
// radius than the failure this async design exists to contain in the first place.
func send(job asyncJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic sending webhook notification: %v", r)
			logf.Log.Error(err, "recovered from a panic in an async webhook send")
		}
	}()
	// context.Background(), not any caller's own ctx: a webhook send must outlive whichever call
	// queued it (an HTTP request in the signal receiver's case, one digest tick in
	// DigestRunnable's), so it can't be tied to that caller's own context being canceled - this
	// also means a send already picked up by a worker no longer aborts early just because its
	// originating request or reconcile was itself canceled or disconnected, unlike the previous
	// synchronous design. Send bounds itself to its own sendTimeout regardless of what context
	// it's given, so this still can't run forever either way.
	return Send(context.Background(), job.url, job.payload)
}

// callDone invokes done, recovering separately from send's own recovery above: a panic in the
// caller-supplied callback itself must not crash the caller, whether that's one of this pool's own
// worker goroutines or, for a dropped job, whichever goroutine called SendAsync directly - neither
// has a recover of its own to fall back on the way controller-runtime's reconcile dispatch does.
func callDone(done func(err error, dropped bool), err error, dropped bool) {
	defer func() {
		if r := recover(); r != nil {
			logf.Log.Error(fmt.Errorf("panic: %v", r), "recovered from a panic in an async webhook send's done callback")
		}
	}()
	done(err, dropped)
}

// SendAsync queues payload for best-effort delivery to url on a small, fixed-size worker pool
// (asyncWorkers), and calls done with the result once it's actually sent - never blocking the
// caller on network I/O the way a direct Send call would. Workers start lazily on first use and
// run for the life of the process; there's no drain on shutdown, matching this package's existing
// stance that every send here is best-effort, not something a caller can rely on completing (see
// the package doc comment) - a notification queued moments before the process exits can be lost.
// This is a strictly quieter loss than the previous synchronous design's own shutdown behavior,
// not merely an equivalent one: a synchronous send canceled by a shutting-down context still
// returned an error its caller logged and counted before returning, where a queued-but-undrained
// send here produces no log line and no metric at all - nothing observes it never happened.
//
// If the queue is already full - asyncQueueSize queued sends still waiting for a free worker -
// this drops payload immediately and calls done(nil, true) instead of queuing a
// asyncQueueSize+1'th job or blocking until room frees up. A worker pool with an unbounded queue
// would only move the resource-growth problem from goroutines to queued jobs; blocking the caller
// until room frees up would reintroduce the exact "the caller waits on notification delivery"
// problem this asynchronous design exists to avoid (issue #63).
func SendAsync(url string, payload any, done func(err error, dropped bool)) {
	asyncOnce.Do(startAsyncWorkers)
	select {
	case asyncQueue <- asyncJob{url: url, payload: payload, done: done}:
	default:
		// Routed through callDone, not called directly: this runs on the caller's own goroutine
		// (DigestRunnable's ticker loop, or the webhook receiver's HTTP handler), neither of which
		// has a per-call recover of its own the way controller-runtime's reconcile dispatch does -
		// a panic in done here would otherwise crash the whole process, exactly the blast radius
		// the queued path was hardened against across several review passes, just left open on
		// this one synchronous branch until it wasn't.
		callDone(done, nil, true)
	}
}
