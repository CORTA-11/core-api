# M08 — Authorization-aware AI

| Field | Value |
| --- | --- |
| Status | `not started` |
| Outcome | Optional AI capabilities receive only authorization-filtered context and can propose, but never silently perform, consequential actions. |
| Depends on | M07 complete; TDR-07; TDR-03 for any confidential processing |
| Release | Optional AI release |

## Boundary

The AI adapter/service receives no PostgreSQL, MinIO, Redis, session, or tenant
credentials. Core API resolves and minimizes context, invokes a named bounded
capability, validates structured output, and separately authorizes any proposed
action. Prompt text is not an authorization control.

## Deliverables

### M08-D01 — Capability registry and outbound adapter

**Artifacts:** `internal/ai/registry.go`, bounded provider/service client,
configuration and threat/data-flow record.

- [ ] Register each capability's model/provider and prompt version, input/output
  schema, allowed data classes, token/time/concurrency/cost bounds, retention,
  tools, and fallback.
- [ ] Restrict outbound destinations and credentials per capability; deny
  arbitrary URLs, headers, SQL, object keys, shell, or recursive agent access.
- [ ] Fail closed when capability, policy, output validation, or provider
  approval is unavailable.
- [ ] Emit bounded metadata telemetry without prompts, source content, secrets,
  or raw generated sensitive output.

**Acceptance:** unknown capability/model/tool, SSRF destination, timeout, token
or cost bound, invalid schema, provider outage, and telemetry-redaction tests
pass.

### M08-D02 — Authorization-filtered context builder

**Artifacts:** `internal/ai/contextbuilder/`, domain retrieval adapters, tests.

- [ ] Resolve every requested task, artifact version, experiment run, message,
  document version, or file through existing domain authorization before adding
  it to context.
- [ ] Apply team, lifecycle, privacy mode, deletion/retraction, consent, and field
  rules before retrieval; minimize and provenance-tag the selected content.
- [ ] Bound source count, bytes/tokens, traversal depth, and excerpt size with a
  deterministic truncation report.
- [ ] Recheck current authorization when a delayed job runs and before protected
  references are shown.

**Acceptance:** cross-team ID, revoked membership, stale permission, deleted or
retracted source, confidential source without consent, misleading reference,
and oversized context tests fail before provider invocation.

### M08-D03 — Summaries, extraction, and comparison

**Artifacts:** capability schemas, handlers/jobs, result/provenance storage.

- [ ] Implement a small initial set: summarize managed content, extract candidate
  tasks, and compare exact experiment/artifact versions.
- [ ] Validate all identifiers, enum values, bounds, relationships, and source
  references against current domain state.
- [ ] Store model/prompt version, exact source public IDs/versions, actor, result
  status, and bounded output; label generated interpretation separately from
  source facts.
- [ ] Keep outputs draft/derived and regenerable; never mutate published research
  history.

**Acceptance:** hallucinated ID, unsupported citation, invalid field, stale
source, conflicting inputs, duplicate job, cancellation, and version/provenance
tests pass.

### M08-D04 — Typed proposals and human approval

**Artifacts:** proposal migrations/domain/API, approved action executors.

- [ ] Represent actions as closed typed proposals with exact target, expected
  version, bounded arguments, source context, expiry, and idempotency key.
- [ ] Require a separate explicit approval for task/file/research/booking or other
  consequential mutations and show the exact proposed change.
- [ ] At execution, revalidate approver permission, tenant/team, target version,
  invariants, consent, and expiry inside the domain transaction.
- [ ] Audit proposal, model/prompt, approval/rejection, executor, and result; the
  AI adapter never calls mutation repositories directly.

**Acceptance:** malformed, cross-team, expired, stale, duplicate, changed-target,
revoked-approver, approval-bypass, reject/cancel, and audit atomicity tests pass.

### M08-D05 — Adversarial and release suite

**Artifacts:** versioned injection/privacy/authorization evaluation corpus,
failure tests, release configuration.

- [ ] Test direct and indirect prompt injection that attempts tenant selection,
  data exfiltration, consent override, tool expansion, secret access, and
  recursive/budget exhaustion.
- [ ] Test provider timeout/outage, invalid/partial streaming output,
  cancellation, queue saturation, model/prompt rollback, and deletion requests.
- [ ] Gate model/prompt/tool changes on the same security, schema, and provenance
  corpus; record only redacted results.
- [ ] Keep AI disabled by configuration unless provider/data handling and the
  capability registry are approved for the deployment.

**Acceptance:** the M08 demonstration in `verification.md` passes; every attempt
to expand authorization fails before retrieval/tool execution, and M01-M07
regressions remain green.

## Merge order

D01 → D02 → D03 → D04 → D05. Do not start with a free-form chat endpoint or give
the model direct infrastructure credentials.

**Implementation links:** _none yet_.
