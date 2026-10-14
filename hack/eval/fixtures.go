/*
Copyright 2026 Albert Asawaroengchai.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	candorv1alpha1 "github.com/teerakarna/candor/api/v1alpha1"
	"github.com/teerakarna/candor/internal/llm"
	"github.com/teerakarna/candor/internal/provider/trivy"
	"github.com/teerakarna/candor/internal/provider/webhook"
	"github.com/teerakarna/candor/internal/signal"
)

// Fixture pairs a real-shaped llm.Request with the answer a correct enrichment should produce.
// Every fixture is grounded in a real, named CVE or a real class of finding, not synthetic text -
// so a result from running these against a model is defensible if it's ever cited the way
// docs/design.md already cites the arXiv 2509.04191 benchmark for other models.
type Fixture struct {
	// Name identifies the fixture in the published report.
	Name string

	Request llm.Request

	// WantAction is the RecommendedAction a correct top-ranked hypothesis must carry. Scored
	// against Hypotheses[0] specifically, not the highest-confidence entry - that's exactly what
	// FindingReconciler.tryProposePullRequest itself reads
	// (internal/controller/finding_controller.go), so this fixture suite measures the actual gate,
	// not an independent notion of "correct".
	WantAction string

	// WantCauseContains are lowercase substrings; the top-ranked hypothesis's Cause must contain
	// at least one to count as correct. Deliberately permissive (substring, not exact match,
	// any-of not all-of) - a free-text Cause has many valid phrasings for the same real cause.
	WantCauseContains []string
}

// kindDeployment is the Kind most fixtures below use - factored out only because goconst flags
// the literal once it repeats, not because it means anything beyond "a realistic Kind value".
const kindDeployment = "Deployment"

// Fixtures is the suite: four real CVEs where a single fixed version makes ProposePullRequest the
// right recommendation, and four findings - two unfixable CVE shapes, two non-dependency findings
// in the generic-webhook shape (SonarQube/Falco-style) - where Notify is the right call. See
// README.md for why this split and why these specific cases.
var Fixtures = []Fixture{
	{
		Name: "log4shell",
		Request: llm.Request{
			Provider: trivy.ProviderName, Kind: kindDeployment, Name: "checkout-api", Severity: signal.SeverityCritical,
			Summary: "CVE-2021-44228: log4j-core 2.14.1 allows remote code execution via JNDI " +
				"lookups in log messages (Log4Shell). Fixed in log4j-core 2.17.1.",
		},
		WantAction:        candorv1alpha1.ActionProposePullRequest,
		WantCauseContains: []string{"jndi", "log4j", "deserial"},
	},
	{
		Name: "struts-ognl",
		Request: llm.Request{
			Provider: trivy.ProviderName, Kind: kindDeployment, Name: "claims-portal", Severity: signal.SeverityCritical,
			Summary: "CVE-2017-5638: Apache Struts 2.3.5-2.3.31 and 2.5-2.5.10 Jakarta Multipart " +
				"parser mishandles the Content-Type header, allowing remote code execution via " +
				"OGNL injection. Fixed in Struts 2.3.32 and 2.5.10.1.",
		},
		WantAction:        candorv1alpha1.ActionProposePullRequest,
		WantCauseContains: []string{"ognl", "multipart", "content-type"},
	},
	{
		Name: "heartbleed",
		Request: llm.Request{
			Provider: trivy.ProviderName, Kind: "StatefulSet", Name: "payments-gateway", Severity: signal.SeverityCritical,
			Summary: "CVE-2014-0160: OpenSSL 1.0.1 through 1.0.1f TLS heartbeat extension lacks a " +
				"bounds check, allowing an out-of-bounds read of up to 64KB of process memory " +
				"(Heartbleed). Fixed in OpenSSL 1.0.1g.",
		},
		WantAction:        candorv1alpha1.ActionProposePullRequest,
		WantCauseContains: []string{"heartbeat", "bounds", "buffer", "over-read"},
	},
	{
		Name: "curl-socks5-overflow",
		Request: llm.Request{
			Provider: trivy.ProviderName, Kind: kindDeployment, Name: "image-resizer", Severity: signal.SeverityHigh,
			Summary: "CVE-2023-38545: curl's SOCKS5 proxy handshake has a heap buffer overflow " +
				"when the hostname exceeds 255 bytes. Fixed in curl 8.4.0.",
		},
		WantAction:        candorv1alpha1.ActionProposePullRequest,
		WantCauseContains: []string{"socks5", "heap", "buffer", "overflow"},
	},
	{
		Name: "no-fix-available",
		Request: llm.Request{
			Provider: trivy.ProviderName, Kind: "DaemonSet", Name: "log-forwarder", Severity: signal.SeverityHigh,
			Summary: "5 vulnerabilities found in base image, including CVE-2024-XXXXX in " +
				"libexample (HIGH, no fixed version available upstream yet).",
		},
		WantAction:        candorv1alpha1.ActionNotify,
		WantCauseContains: []string{"libexample", "vulnerab"},
	},
	{
		Name: "conflicting-fix-versions",
		Request: llm.Request{
			Provider: trivy.ProviderName, Kind: kindDeployment, Name: "report-generator", Severity: signal.SeverityHigh,
			Summary: "12 vulnerabilities found across 3 packages. Recommended fixed versions " +
				"disagree by package: libfoo needs >=2.1, libbar needs >=3.0, no single upgrade " +
				"resolves all of them.",
		},
		WantAction:        candorv1alpha1.ActionNotify,
		WantCauseContains: []string{"disagree", "multiple", "conflict", "vulnerab"},
	},
	{
		Name: "rbac-cluster-admin-binding",
		Request: llm.Request{
			Provider: webhook.ProviderName, Kind: "ClusterRoleBinding", Name: "default-sa-admin", Severity: signal.SeverityHigh,
			Summary: "ClusterRoleBinding grants cluster-admin to the default ServiceAccount in " +
				"namespace billing. Any pod in that namespace without an explicit ServiceAccount " +
				"inherits full cluster-admin privileges.",
		},
		WantAction:        candorv1alpha1.ActionNotify,
		WantCauseContains: []string{"cluster-admin", "rbac", "service account", "serviceaccount"},
	},
	{
		Name: "runtime-shell-anomaly",
		Request: llm.Request{
			Provider: webhook.ProviderName, Kind: "Pod", Name: "nginx-7d9f4c8b6-x2kqp", Severity: signal.SeverityCritical,
			Summary: "Falco alert: an interactive shell (/bin/sh) was spawned inside container " +
				"nginx-7d9f4c8b6-x2kqp immediately after an outbound connection to an IP with no " +
				"prior history for this workload.",
		},
		WantAction:        candorv1alpha1.ActionNotify,
		WantCauseContains: []string{"shell", "runtime", "process", "anomal", "suspicious"},
	},
}
