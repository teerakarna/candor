package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSend_PostsJSONBody(t *testing.T) {
	received := make(chan Event, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("decoding body: %v", err)
		}
		received <- event
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := Event{Kind: KindFindingCreated, Namespace: "team-a", Finding: "trivy-abc123", Severity: "CRITICAL", Summary: "3 critical vulns"}
	if err := Send(context.Background(), server.URL, event); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got != event {
			t.Errorf("received %+v, want %+v", got, event)
		}
	default:
		t.Fatal("handler was never invoked")
	}
}

func TestSend_NonHTTPURL_Rejected(t *testing.T) {
	err := Send(context.Background(), "ftp://example.com/hook", Event{})
	if err == nil {
		t.Fatal("expected an error for a non-http(s) URL")
	}
}

func TestSend_NonSuccessStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if err := Send(context.Background(), server.URL, Event{}); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestSend_Digest(t *testing.T) {
	received := make(chan Digest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var digest Digest
		if err := json.NewDecoder(r.Body).Decode(&digest); err != nil {
			t.Errorf("decoding body: %v", err)
		}
		received <- digest
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	digest := Digest{
		Kind: KindDigest, Namespace: "team-a", WindowSeconds: 86400,
		StillPresent: 2, Resolved: 1, Recurred: 0,
		BySeverity: map[string]int{"CRITICAL": 1, "HIGH": 1},
	}
	if err := Send(context.Background(), server.URL, digest); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got.Namespace != digest.Namespace || got.StillPresent != digest.StillPresent || got.BySeverity["CRITICAL"] != 1 {
			t.Errorf("received %+v, want %+v", got, digest)
		}
	default:
		t.Fatal("handler was never invoked")
	}
}
