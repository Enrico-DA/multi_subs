# Durable decisions

## Use one product identity

Decision: The fork is named `multisubs`.

The executable is `multisubs`, the repository is `Enrico-DA/multi_subs`, the Go module is `github.com/Enrico-DA/multi_subs`, the entrypoint is `cmd/multisubs`, and the core package is `internal/multisubs`.

Why: One identity avoids ambiguous installs, linker targets, help, state paths, and support instructions.

Enforcement: Release publication is allowed only from `Enrico-DA/multi_subs`. A non-publishing CI check ties together the repository guard, module, linker target, command directory, and binary/archive name.

## Keep upstream attribution

Decision: Preserve attribution and license references to `olliecrow/multicodex`.

Why: The fork comes from Ollie's project even though its product identity and public command tree now differ.

Enforcement: `LICENSE`, README attribution, repository guidance, and the upstream translation map retain the upstream name.

## Use symmetric provider namespaces

Decision: Codex commands live under `multisubs codex`; Claude commands live under `multisubs claude`.

Why: A symmetric tree makes provider ownership clear and leaves room for later product-wide commands.

Enforcement: Bare Codex routes fail with code 2 before state access and point to the matching namespaced route. `multisubs init` remains the shared initializer, and `multisubs codex init` calls the same path.

## Keep aggregate and focused doctors

Decision: `multisubs doctor` reports shared/base, Codex, and Claude sections. Provider doctors report only their provider.

Why: The product needs one full read-only health check without losing fast, focused diagnosis.

Enforcement: Aggregate JSON has `base`, `codex`, and `claude` reports. All doctor startup remains non-mutating.

## Use one new state root and environment namespace

Decision: Persistent product state defaults to `~/multisubs`, with `MULTISUBS_HOME` as the override. Product controls use `MULTISUBS_*`.

Why: A hard rename is safer than two homes or two environment namespaces that can disagree.

Enforcement: Runtime path resolution reads only the new variables. Active heartbeat, routing metadata, Claude metadata, and selected-profile markers use the new namespace.

## Reject legacy controls

Decision: Old `MULTICODEX_*` variables cause startup to fail before state access.

Why: Silently accepting old controls would create hidden compatibility and could route a provider child with stale account metadata.

Enforcement: Top-level startup rejects any old product-prefixed variable. Provider child environments still strip old controls. The old `~/multicodex` and `.multicodex` patterns remain only as legacy-sensitive ignore and leak protection.

## Keep provider stores isolated

Decision: Codex and Claude use separate registries, profile roots, provider variables, and routing logic.

Why: The official tools have different auth and usage models. Combining their stores would weaken account boundaries.

Enforcement: Codex profiles use profile-local `CODEX_HOME`. Claude profiles use derived `CLAUDE_CONFIG_DIR`. Neither default account is product-owned.

## Protect local credentials and paths

Decision: Fail closed when a profile path, sensitive file, lock, or routing metadata path is unsafe.

Why: Symlinks, hard links, broad permissions, or paths outside product state can cross account boundaries or leak credentials.

Enforcement: State and profile paths are private. Sensitive files and locks reject unsafe links. Selected-profile metadata and heartbeat lock overrides stay below `MULTISUBS_HOME`.

## Preserve no-clobber resource reconciliation

Decision: Regular profile guidance, config, and skill entries are user overrides.

Why: The product should not erase local work while keeping shared resources convenient.

Enforcement: Only documented product-owned links may change. Desired resource state is validated before links move. Runtime-managed `.system` skills remain profile-local.

## Keep usage rules provider-specific

Decision: Codex usage and routing stay weekly-only. Claude routing uses fresh session, weekly all-model, and Fable usage. Each provider's default account competes normally with its managed profiles.

Why: Those are the provider signals used by the current safe routing contracts.

Enforcement: Codex applies one weekly, model, reset, and eligibility policy to default and managed candidates, while keeping default execution unmanaged. Claude puts every valid default or managed target in one score-sorted, organization-deduplicated, reservation-locked candidate set. Claude reads usage through the official CLI without reading credential contents.

## Prefer plain English

Decision: Use short, direct language in output, docs, comments, tests, reviews, and change records.

Why: Clear names and messages reduce mistakes in a security-sensitive local tool.

Enforcement: Explain necessary technical terms once and avoid vague names when touching code.
