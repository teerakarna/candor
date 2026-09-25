package notify

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSendAsync_DeliversSuccessfully(t *testing.T) {
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	done := make(chan struct{}, 1)
	var gotErr error
	var gotDropped bool
	SendAsync(server.URL, Event{Kind: KindFindingCreated}, func(err error, dropped bool) {
		gotErr, gotDropped = err, dropped
		close(done)
	})

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the request")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done callback never ran")
	}
	if gotErr != nil || gotDropped {
		t.Errorf("done(err=%v, dropped=%v), want (nil, false)", gotErr, gotDropped)
	}
}

func TestSendAsync_SendFailure_ReportedNotDropped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	done := make(chan struct{}, 1)
	var gotErr error
	var gotDropped bool
	SendAsync(server.URL, Event{}, func(err error, dropped bool) {
		gotErr, gotDropped = err, dropped
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done callback never ran")
	}
	if gotErr == nil {
		t.Error("done(err) = nil, want an error for a 500 response")
	}
	if gotDropped {
		t.Error("done(dropped) = true, want false - a send that actually ran and failed is not the same as one dropped for a full queue")
	}
}

// panicMarshalPayload panics from MarshalJSON, which json.Marshal invokes synchronously - used to
// make Send itself panic (rather than the done callback) without needing a real HTTP call at all.
type panicMarshalPayload struct{}

func (panicMarshalPayload) MarshalJSON() ([]byte, error) {
	panic("boom")
}

// TestSendAsync_SendPanics_DoneStillCalled guards against runJob calling
// job.done(Send(...), false) as one combined expression: Go evaluates Send's return value before
// invoking done, so a panic during that evaluation (inside Send itself, not the done callback)
// would skip calling done entirely, even with the whole expression wrapped in a recover. A caller
// relying on done always firing exactly once (a sync.WaitGroup, for instance - see
// TestSendAsync_QueueFull_Drops below) would hang forever with nothing but a log line to explain
// why.
func TestSendAsync_SendPanics_DoneStillCalled(t *testing.T) {
	done := make(chan struct{}, 1)
	var gotErr error
	SendAsync("http://example.invalid", panicMarshalPayload{}, func(err error, dropped bool) {
		gotErr = err
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("done callback never ran after Send panicked")
	}
	if gotErr == nil {
		t.Error("done(err) = nil, want a non-nil error synthesized from the recovered panic")
	}
}

// TestSendAsync_PanicInDoneCallback_WorkerRecoversAndKeepsProcessing is the regression test for a
// real finding from /code-review high: these worker goroutines are long-lived and unsupervised,
// unlike every other code path in this project (net/http recovers per HTTP request,
// controller-runtime recovers per reconcile) - an unrecovered panic here would crash the whole
// process, not just this one notification. Reaching the end of this test at all is part of the
// proof: without runJob's own recover, the panics below would have killed this entire test binary.
func TestSendAsync_PanicInDoneCallback_WorkerRecoversAndKeepsProcessing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// One panicking send per worker, so every worker in the pool experiences a panic at least once.
	for range asyncWorkers {
		SendAsync(server.URL, Event{}, func(error, bool) {
			panic("boom")
		})
	}

	// The real assertion: the pool still has its full capacity afterward, not silently down a
	// worker for every panic it absorbed.
	var wg sync.WaitGroup
	for range asyncWorkers {
		wg.Add(1)
		SendAsync(server.URL, Event{}, func(err error, dropped bool) {
			defer wg.Done()
			if err != nil || dropped {
				t.Errorf("done(err=%v, dropped=%v) after a prior panic, want (nil, false) - the worker that panicked must still be back in service", err, dropped)
			}
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sends after the panics - a worker may be wedged or lost")
	}
}

// TestSendAsync_QueueFull_Drops guards against a bare `go func()` per notification growing without
// bound under a sustained burst against a slow endpoint. SendAsync's fixed worker pool plus bounded
// queue caps that instead - this proves the cap is real and exact, not just "eventually slows
// down": every one of exactly asyncWorkers+asyncQueueSize concurrently-blocked sends is accepted,
// and the very next one is dropped, deterministically.
//
// Three subtle bugs showed up in this test itself while writing it, worth keeping in mind for any
// future test shaped like this one:
//  1. `defer close(release)` registered before `defer blocked.Close()` deadlocks on cleanup -
//     defers run LIFO, so Close() (which blocks until every outstanding request completes) would
//     run BEFORE release is closed, waiting forever on the handlers still parked on <-release.
//     Fixed by not using defer for either at all - both are called explicitly, in the correct
//     order, once everything they'd wait on has actually finished (see the end of this test).
//  2. asyncOnce.Do(startAsyncWorkers) only launches the worker goroutines - it returns as soon as
//     the `go func(){...}` statements are issued, not once those goroutines have actually reached
//     their receive loop. Sending asyncWorkers+asyncQueueSize jobs immediately therefore isn't
//     guaranteed to fill exactly "asyncWorkers in flight + asyncQueueSize buffered": if a worker
//     hasn't been scheduled yet, its "slot" is just more buffer capacity instead, so the buffer
//     alone could fill after only asyncQueueSize sends, dropping sends this test expected to
//     succeed. Fixed by explicitly warming up and confirming every worker has actually reached the
//     handler (entered) before counting on the buffer's own capacity for the rest of the sends.
//  3. The buffered sends' own done callbacks (which call t.Errorf on an unexpected drop) were
//     never awaited before the test returned - httptest.Server.Close() only waits for the
//     server's own in-flight handlers, not for the async worker goroutines' client-side Send
//     calls to return and invoke done. A callback firing after the test function had already
//     returned would panic ("call to t.Errorf after test has completed") instead of failing
//     cleanly. Fixed with a WaitGroup covering every one of those callbacks, waited on before
//     this test does anything else.
func TestSendAsync_QueueFull_Drops(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, asyncWorkers)
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	// Warm-up: saturate every worker first, and wait for confirmation that all of them have
	// actually reached the handler (not just been sent a job) before relying on the pool's total
	// capacity being exactly asyncWorkers+asyncQueueSize for the rest of this test.
	for range asyncWorkers {
		SendAsync(blocked.URL, Event{}, func(error, bool) {})
	}
	for range asyncWorkers {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("workers never reached the blocking handler during warm-up")
		}
	}

	// Every worker is now confirmed busy and the queue starts empty, so exactly asyncQueueSize
	// more sends should fill it - SendAsync's own enqueue decision (the select against
	// asyncQueue) is synchronous, so a dropped job's done callback runs immediately, inline,
	// inside this same call, safe to assert on directly from this loop on this goroutine. An
	// accepted job's callback, by contrast, only runs once release is closed below - wg tracks
	// those so this test can wait for every one of them before returning (see bug 3 above).
	var wg sync.WaitGroup
	for i := range asyncQueueSize {
		wg.Add(1)
		SendAsync(blocked.URL, Event{}, func(err error, dropped bool) {
			defer wg.Done()
			if dropped {
				t.Errorf("buffered send %d of %d was dropped, want every send within capacity accepted", i, asyncQueueSize)
			}
		})
	}

	overflowDone := make(chan struct{}, 1)
	var overflowDropped bool
	SendAsync(blocked.URL, Event{}, func(err error, dropped bool) {
		overflowDropped = dropped
		close(overflowDone)
	})
	select {
	case <-overflowDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the one send past capacity should have been dropped synchronously, but its done callback never ran")
	}
	if !overflowDropped {
		t.Error("the send past asyncWorkers+asyncQueueSize capacity was not dropped, want dropped = true")
	}

	// Explicit, ordered cleanup, not defer (see bug 1 above): release the still-blocked handlers,
	// wait for every buffered send's own callback to actually run (bug 3), then close the server -
	// by then nothing is still in flight for Close() to wait on.
	close(release)
	wg.Wait()
	blocked.Close()
}

// TestSendAsync_PanicInDoneCallback_OnDroppedSend_DoesNotCrash guards against the queue-full branch
// calling the caller-supplied done(nil, true) directly, with no panic recovery, unlike the queued
// path (runJob/send/callDone), which has two independent layers of it. That branch runs on the
// caller's own goroutine - DigestRunnable's ticker loop or the webhook receiver's HTTP handler -
// neither of which has a recover of its own the way controller-runtime's reconcile dispatch does,
// so a panicking callback there would crash the whole process. Reaching the end of this test at all
// is part of the proof.
func TestSendAsync_PanicInDoneCallback_OnDroppedSend_DoesNotCrash(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, asyncWorkers)
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer blocked.Close()

	// Saturate every worker and fill the queue, exactly as TestSendAsync_QueueFull_Drops does, so
	// the next send below is guaranteed to take the dropped, direct-call path rather than being
	// queued.
	for range asyncWorkers {
		SendAsync(blocked.URL, Event{}, func(error, bool) {})
	}
	for range asyncWorkers {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("workers never reached the blocking handler during warm-up")
		}
	}
	var wg sync.WaitGroup
	for range asyncQueueSize {
		wg.Add(1)
		SendAsync(blocked.URL, Event{}, func(error, bool) { wg.Done() })
	}

	SendAsync(blocked.URL, Event{}, func(err error, dropped bool) {
		if !dropped {
			t.Error("expected this send to be dropped (queue full), want dropped = true")
		}
		panic("boom")
	})

	close(release)
	wg.Wait()
}
