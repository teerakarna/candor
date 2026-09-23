#!/usr/bin/env python3
"""Fake candor_* metrics for previewing charts/chart/files/grafana-dashboard.json locally.

Serves /metrics in Prometheus text format with the same metric and label names
internal/metrics registers for real - not a fixture of the operator, just enough variation over
time that the dashboard's panels actually move instead of sitting flat.

Findings are simulated as an actual small state machine per (namespace, severity) - spawn, get
resolved, sometimes recur - rather than two independent random walks. Outstanding
(candor_findings_current) and the Resolved/Recurred tallies (candor_verification_transitions_total)
are derived from the same simulated events, so they stay causally connected: a real
Resolved/Recurred count can never wander around unrelated to what Outstanding is doing, matching
how the real metrics relate in production. Dev tool only: not part of the release build, never
imported by any Go package.
"""

import random
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

# One simulated finding population per (namespace, severity). "still_present" here means a
# transition INTO that state was recorded (matches internal/signal.Ingest: it counts transitions,
# not every reconcile of unchanged content) - seeded once for "created" plus whatever the sim
# ticks add.
findings = {
    ("team-a", "CRITICAL"): {"open": 1, "still_present": 1, "resolved": 0, "recurred": 0},
    ("team-a", "HIGH"): {"open": 3, "still_present": 3, "resolved": 0, "recurred": 0},
    ("team-a", "MEDIUM"): {"open": 5, "still_present": 5, "resolved": 0, "recurred": 0},
    ("team-b", "LOW"): {"open": 2, "still_present": 2, "resolved": 0, "recurred": 0},
}

state = {
    "llm_calls": {"success": 42, "error": 1},
    "skipped": {"not_needed": 850, "no_llm_configured": 0, "budget_exhausted": 12, "suppressed": 30},
    "budget_used": {("team-a", "policy"): 8, ("team-b", "policy"): 45},
    "budget_limit": {("team-a", "policy"): 50, ("team-b", "policy"): 50},
}
lock = threading.Lock()


def tick():
    """Advance the simulation a little every few seconds, so rate()/increase() queries in the
    dashboard show real movement instead of a flat line."""
    while True:
        with lock:
            state["llm_calls"]["success"] += random.choice([0, 0, 1])
            if random.random() < 0.05:
                state["llm_calls"]["error"] += 1
            state["skipped"]["not_needed"] += random.randint(3, 9)
            state["skipped"]["suppressed"] += random.choice([0, 0, 1])
            state["budget_used"][("team-a", "policy")] = min(
                50, state["budget_used"][("team-a", "policy")] + random.choice([0, 1])
            )

            for f in findings.values():
                # A new finding appears - a real "StillPresent" transition, the same event a
                # brand-new Finding gets in production.
                if random.random() < 0.05:
                    f["open"] += 1
                    f["still_present"] += 1
                # An open finding gets resolved. Can only reduce what's actually open, same
                # constraint the real system has (resolveIfOpen only fires on an existing Finding).
                if f["open"] > 0 and random.random() < 0.12:
                    f["open"] -= 1
                    f["resolved"] += 1
                # A previously-resolved finding recurs - can only happen if something has ever
                # been resolved, mirroring resolveIfOpen's own precondition in internal/signal.
                if f["resolved"] > 0 and random.random() < 0.04:
                    f["open"] += 1
                    f["recurred"] += 1
        time.sleep(5)


def render():
    with lock:
        lines = []

        lines.append("# HELP candor_llm_calls_total Total LLM enrichment calls made, by result (success|error).")
        lines.append("# TYPE candor_llm_calls_total counter")
        for result, value in state["llm_calls"].items():
            lines.append(f'candor_llm_calls_total{{result="{result}"}} {value}')

        lines.append(
            "# HELP candor_enrichment_skipped_total Total times enrichment was skipped without "
            "calling the LLM, by reason (not_needed|no_llm_configured|budget_exhausted|suppressed)."
        )
        lines.append("# TYPE candor_enrichment_skipped_total counter")
        for reason, value in state["skipped"].items():
            lines.append(f'candor_enrichment_skipped_total{{reason="{reason}"}} {value}')

        lines.append(
            "# HELP candor_verification_transitions_total Total verification outcome transitions "
            "recorded while ingesting signals, by outcome (still_present|resolved|recurred) and severity."
        )
        lines.append("# TYPE candor_verification_transitions_total counter")
        for (namespace, severity), f in findings.items():
            for outcome in ("still_present", "resolved", "recurred"):
                lines.append(
                    f'candor_verification_transitions_total{{outcome="{outcome}",severity="{severity}"}} {f[outcome]}'
                )

        lines.append(
            "# HELP candor_signalpolicy_budget_calls_used LLM calls used in the current budget "
            "window, per SignalPolicy."
        )
        lines.append("# TYPE candor_signalpolicy_budget_calls_used gauge")
        for (namespace, policy), value in state["budget_used"].items():
            lines.append(
                f'candor_signalpolicy_budget_calls_used{{namespace="{namespace}",signalpolicy="{policy}"}} {value}'
            )

        lines.append(
            "# HELP candor_signalpolicy_budget_calls_limit Configured max LLM calls per budget "
            "window, per SignalPolicy."
        )
        lines.append("# TYPE candor_signalpolicy_budget_calls_limit gauge")
        for (namespace, policy), value in state["budget_limit"].items():
            lines.append(
                f'candor_signalpolicy_budget_calls_limit{{namespace="{namespace}",signalpolicy="{policy}"}} {value}'
            )

        lines.append(
            "# HELP candor_findings_current Current number of Findings, by namespace, severity, "
            "and verification outcome."
        )
        lines.append("# TYPE candor_findings_current gauge")
        for (namespace, severity), f in findings.items():
            # Real steady-state behavior: a Finding sits at "Recurred" only for the brief window
            # before its next reconcile flips it back to StillPresent (see
            # internal/signal.Ingest - Recurred isn't sticky), so the live snapshot is
            # overwhelmingly StillPresent in practice. Simulating that one-reconcile window isn't
            # worth the complexity here - every currently-open finding is reported StillPresent.
            lines.append(
                f'candor_findings_current{{namespace="{namespace}",severity="{severity}",outcome="StillPresent"}} {f["open"]}'
            )

        return "\n".join(lines) + "\n"


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/metrics":
            self.send_response(404)
            self.end_headers()
            return
        body = render().encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        pass  # keep container logs quiet - this is a dev tool, not something to debug via logs


if __name__ == "__main__":
    threading.Thread(target=tick, daemon=True).start()
    HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
