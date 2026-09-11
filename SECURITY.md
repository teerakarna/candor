# Security Policy

## Reporting a Vulnerability

Please report security issues privately using
[GitHub Security Advisories](https://github.com/teerakarna/candor/security/advisories/new) for this
repository, rather than opening a public issue.

If you're unable to use Security Advisories, email 21040807+teerakarna@users.noreply.github.com
with a description of the issue and steps to reproduce.

Please do not disclose the issue publicly until it has been addressed.

## Scope

Candor ingests untrusted signal data (log lines, alert payloads, image metadata) that feeds into
LLM prompts. The main areas of security interest are:

- **Prompt injection**: anything that lets ingested telemetry alter the selected action rather than
  only the finding's narrative content. Model output must only ever select from the fixed action
  catalog (see the design doc) — never emit or influence a free-form action.
- **RBAC scope creep**: the operator's ServiceAccount is read-only by default; any change that
  grants it broader permissions is a security-relevant change, not a routine one.
- **Secrets handling**: LLM API keys, webhook URLs, and GitOps repo credentials.
- **The GitOps write path**: anything that could cause a proposed pull request to be merged or
  applied without human review.
