// Package notify is Candor's generic webhook sink (docs/design.md: "one code path covers
// Slack/Teams/PagerDuty/anything"). It knows nothing about any particular chat or paging vendor -
// it POSTs a JSON payload to a URL and nothing more. Anything vendor-specific (formatting for
// Slack blocks, PagerDuty's event schema, etc.) is the receiving end's problem, not this
// package's - that's the whole point of a generic sink.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// sendTimeout bounds every webhook POST. A flaky or unreachable notification endpoint must never
// become a reason a Finding reconcile (internal/signal.Ingest) or the digest loop
// (internal/controller.DigestRunnable) gets stuck - both callers treat Send as best-effort.
const sendTimeout = 10 * time.Second

// Event kinds. See the Event and Digest payload types below for which kind carries which shape.
const (
	KindFindingCreated  = "FindingCreated"
	KindFindingResolved = "FindingResolved"
	KindFindingRecurred = "FindingRecurred"
	KindDigest          = "Digest"
)

// Event is the payload for a single Finding notification.
type Event struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Finding   string `json:"finding"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
}

// Digest is the payload for the periodic per-namespace summary
// (internal/controller.DigestRunnable) - docs/design.md's "AIOps: Prove It!" artifact: what
// happened over the window, not just what's currently open.
type Digest struct {
	Kind          string         `json:"kind"`
	Namespace     string         `json:"namespace"`
	WindowSeconds int64          `json:"windowSeconds"`
	StillPresent  int            `json:"stillPresent"`
	Resolved      int            `json:"resolved"`
	Recurred      int            `json:"recurred"`
	BySeverity    map[string]int `json:"bySeverity"`
}

// Send POSTs payload as JSON to url. Callers decide whether a failure here should block anything
// - by convention in this codebase, it never does (see the package doc comment).
func Send(ctx context.Context, url string, payload any) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("webhook url must be http:// or https://, got %q", url)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling webhook payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook to %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook endpoint %s returned status %d", url, resp.StatusCode)
	}
	return nil
}
