#!/usr/bin/env python3
"""Fake candor_* metrics for previewing charts/chart/files/grafana-dashboard.json locally.

Serves /metrics in Prometheus text format with the same metric and label names
internal/metrics/metrics.go registers for real - not a fixture of the operator, just enough
variation over time that the dashboard's timeseries panels actually move instead of sitting flat.
Dev tool only: not part of the release build, never imported by any Go package.
"""

import random
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

state = {
    "llm_calls": {"success": 42, "error": 1},
    "skipped": {"not_needed": 850, "no_llm_configured": 0, "budget_exhausted": 12, "suppressed": 30},
    # Keyed by (outcome, severity), matching the real metric's label shape.
    "verification": {
        ("still_present", "CRITICAL"): 8,
        ("still_present", "HIGH"): 22,
        ("still_present", "MEDIUM"): 30,
        ("resolved", "CRITICAL"): 3,
        ("resolved", "HIGH"): 13,
        ("resolved", "MEDIUM"): 9,
        ("resolved", "LOW"): 5,
        ("recurred", "CRITICAL"): 1,
        ("recurred", "HIGH"): 2,
        ("recurred", "MEDIUM"): 1,
        ("recurred", "LOW"): 0,
    },
    "budget_used": {("team-a", "policy"): 8, ("team-b", "policy"): 45},
    "budget_limit": {("team-a", "policy"): 50, ("team-b", "policy"): 50},
    # candor_findings_current: a live gauge, not a counter - values wander up and down in tick()
    # rather than only increasing, unlike everything else in this file.
    "findings_current": {
        ("team-a", "CRITICAL", "StillPresent"): 2,
        ("team-a", "HIGH", "StillPresent"): 5,
        ("team-a", "MEDIUM", "StillPresent"): 8,
        ("team-b", "LOW", "StillPresent"): 3,
        ("team-a", "CRITICAL", "Recurred"): 1,
        ("team-a", "HIGH", "Resolved"): 12,
    },
}
lock = threading.Lock()


def tick():
    """Advance the counters a little every few seconds, so rate()/increase() queries in the
    dashboard show real movement instead of a flat line."""
    while True:
        with lock:
            state["llm_calls"]["success"] += random.choice([0, 0, 1])
            if random.random() < 0.05:
                state["llm_calls"]["error"] += 1
            state["skipped"]["not_needed"] += random.randint(3, 9)
            state["skipped"]["suppressed"] += random.choice([0, 0, 1])
            for key in state["verification"]:
                outcome = key[0]
                chance = {"still_present": 0.3, "resolved": 0.1, "recurred": 0.03}[outcome]
                if random.random() < chance:
                    state["verification"][key] += 1
            state["budget_used"][("team-a", "policy")] = min(
                50, state["budget_used"][("team-a", "policy")] + random.choice([0, 1])
            )
            for key in state["findings_current"]:
                if random.random() < 0.15:
                    delta = random.choice([-1, 1])
                    state["findings_current"][key] = max(0, state["findings_current"][key] + delta)
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
        for (outcome, severity), value in state["verification"].items():
            lines.append(f'candor_verification_transitions_total{{outcome="{outcome}",severity="{severity}"}} {value}')

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
        for (namespace, severity, outcome), value in state["findings_current"].items():
            lines.append(
                f'candor_findings_current{{namespace="{namespace}",severity="{severity}",outcome="{outcome}"}} {value}'
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
