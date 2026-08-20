# Blocking technical decisions

This is intentionally not a general policy backlog. It contains only choices
that change a public contract, schema, security boundary, or deployable unit.
The proposed default lets implementation continue unless a maintainer selects a
different option before the named deliverable starts.

| ID | Decision needed by | Proposed default | Closure artifact |
| --- | --- | --- | --- |
| `TDR-01` Product and protocol namespace | M01-D04 | Keep the Go module path as `github.com/CORTA-11/core-api` for now; use `synodus` for new cookie, Redis, telemetry, image, and bucket namespaces. Treat a module rename as a separate compatibility change. | Short ADR listing canonical names and retained aliases. |
| `TDR-02` Prototype API compatibility | M03-D04 | Make a clean `/api/v1` cutover because the current API is an unauthenticated prototype. Do not maintain `X-Org-ID` or unversioned compatibility routes. | Approved route inventory stating whether any external consumer requires a transition window. |
| `TDR-03` Confidential-file key custody | M05-D05 | Ship managed files only in the core release. Add confidential mode later using client-side envelope encryption; the API stores ciphertext and wrapped keys but never plaintext team keys. | Threat model covering recovery, revocation, rotation, device loss, and AI/search exclusions. |
| `TDR-04` External component ownership | M05-D04 | Keep API, worker, and provisioner commands in this repository. Keep web, Centrifugo deployment, and any Python AI service in separately owned repositories. | Component map naming the repository and owner for each deployable. |
| `TDR-05` Recovery and availability targets | M05-D07 | Do not advertise HA. Provide single-node Compose recovery with measured backup/restore results; set production RPO/RTO only after the first drill. | Recorded restore drill and approved target values. |
| `TDR-06` Identity provider and account linking | M03-D01 | Use Keycloak OIDC authorization-code flow with PKCE. Link by immutable `(issuer, subject)` and migrate existing password users only through an explicit one-time account-link flow. | Identity ADR plus tested mapping for existing users. |
| `TDR-07` AI deployment and provider data handling | M08-D01 | AI is an optional external adapter with no database credentials. Core API performs retrieval and authorization and submits only minimized context to an approved provider. | Provider/data-flow review and capability allowlist. |

## Decision handling

- Record the chosen option; do not create a separate governance milestone.
- A decision is closed when the artifact names the chosen interface and the
  affected milestone is updated in the same change.
- If no maintainer objects before implementation begins, use the proposed
  default for an internal or pre-release interface.
- `TDR-03`, `TDR-06`, and `TDR-07` require explicit security approval because
  they govern cryptographic key material, identity, or external data transfer.
- New questions belong here only when two plausible answers would materially
  change code that is about to be built.
