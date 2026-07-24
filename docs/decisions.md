# Durable decisions

## Use one product identity

Decision: The fork is named `multisubs`.

The executable is `multisubs`, the repository is `Enrico-DA/multi_subs`, the Go module is `github.com/Enrico-DA/multi_subs`, the entrypoint is `cmd/multisubs`, and the core package is `internal/multisubs`.

Why: One identity avoids ambiguous installs, linker targets, help, state paths, and support instructions.

Enforcement: Release publication is allowed only from `Enrico-DA/multi_subs`. A non-publishing CI check ties together the repository guard, module, linker target, command directory, and binary/archive name.

## Keep upstream attribution

Decision: Preserve the Apache 2.0 license terms and attribution to `olliecrow/multicodex`.

Why: The fork comes from Ollie's project even though its product identity and public command tree now differ.

Enforcement: `LICENSE` preserves the Apache 2.0 terms. README attribution, `AGENTS.md` repository guidance, and `docs/upstream-sync.md` preserve the upstream name.

## Use symmetric provider namespaces

Decision: Codex commands live under `multisubs codex`; Claude commands live under `multisubs claude`.

Why: A symmetric tree makes provider ownership clear and leaves room for later product-wide commands.

Enforcement: Bare Codex routes fail with code 2 before state access and point to the matching namespaced route. `multisubs init` remains the shared initializer, and `multisubs codex init` calls the same path.

## Keep aggregate and focused doctors

Decision: `multisubs doctor` reports shared/base, Codex, and Claude sections. Provider doctors report only their provider.

Why: The product needs one full read-only health check without losing fast, focused diagnosis.

Enforcement: Aggregate JSON has `base`, `codex`, and `claude` reports. All doctor startup remains non-mutating.

An invalid Codex registry becomes a failed aggregate base check while the aggregate doctor continues safe base, Codex, and Claude checks and emits every section.

## Use one new state root and environment namespace

Decision: Persistent product state defaults to `~/multisubs`, with `MULTISUBS_HOME` as the override. Product controls use `MULTISUBS_*`.

Why: A hard rename is safer than two homes or two environment namespaces that can disagree.

Enforcement: Runtime path resolution reads only the new variables. Active heartbeat, routing metadata, Claude metadata, and selected-profile markers use the new namespace.

## Reject legacy controls

Decision: Old `MULTICODEX_*` variables cause startup to fail before state access.

Why: Silently accepting old controls would create hidden compatibility and could route a provider child with stale account metadata.

Enforcement: Top-level startup rejects any old product-prefixed variable. Provider child environments still strip old controls. The old `~/multicodex` and `.multicodex` patterns remain only as legacy-sensitive ignore and leak protection.

Monitor filesystem discovery prunes both legacy home roots and their canonical targets so an alias cannot reactivate old credentials.

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

## Use one managed Codex config boundary

Decision: A managed Codex config has only two valid filesystem forms: a regular non-symlink with a verifiable hard-link count of exactly one, or a symlink whose resolved path exactly equals the resolved default Codex config and whose target is regular.

Why: Setup, execution, status, doctor, model inspection, and monitoring must agree on which config owns managed profile behavior. Path equality also prevents a hard-link alias of the default config from being treated as the default path.

Enforcement: Every managed caller uses the shared filesystem-only validator before TOML parsing or provider launch. Hard-linked configs fail without automatic repair. Valid single-link manual overrides remain untouched, and only the exact old generated regular config may be replaced during setup. The default account and default config remain unmanaged.

Migration impact: Existing valid default-config symlinks and single-link manual overrides continue to work. Arbitrary symlinks, broken links, hard-linked configs, and non-regular entries require a manual fix; no background migration changes them.

## Keep usage rules provider-specific

Decision: Present Codex and Claude quota through one provider-neutral report model while keeping collection and routing rules provider-specific. Codex routing and the live monitor stay weekly-only. Claude routing uses fresh session and weekly all-model usage, plus Fable usage for each candidate whose effective model or fallback is applicable to Fable or cannot be classified conclusively. Each provider's default account competes normally with its managed profiles.

Why: Users need one quick quota snapshot without hiding account-level differences. A shared formatter keeps labels, partial failures, and reset display consistent, but combining provider collectors or routing scores would weaken their different safety rules. Claude model settings can differ between the default account and each isolated managed profile. A single invocation-wide Fable decision can either exclude a valid account or spend against a window that the selected account does not need.

Enforcement: `multisubs usage` and both provider usage commands share one presentation model and renderer. Codex exec and usage share typed managed/default target enumeration; Claude usage reorders and redacts the shared Claude targets. They put the default account last and keep it present once even when a managed registry entry is unsafe. Presentation names are allocated across the whole provider target set so email aliases and unexpected duplicate names cannot collide. Codex retains declared session data for this report, while every routing selector and the monitor keep using weekly fields. A session-only primary still triggers weekly fallback. Only the report source can merge the primary session with safe fallback weekly fields, and missing weekly data still makes the account partial. Claude parses shared CLI intent once for routing, resolves relevant settings per candidate, and classifies Fable as not applicable, applicable, or possible. Applicable and possible both require and score the Fable window. Unknown local, managed, server, account, and organization state fails closed only for the fields and candidate it can affect.

The usage report is read-only, has bounded per-account collection, closes each Codex source once after fetch cancellation, treats cleanup failure as a safe partial failure, and has no JSON form in this release. It excludes monitor-only, active-home, discovered, and observed-token sources. It never aggregates account percentages. Claude session duration comes only from an explicit bounded parenthesized heading fragment. Claude reset text is rendered only through a strict allow-list grammar.

Claude settings inspection is data-minimizing. It reads only routing fields from regular files capped at 2 MiB, does not execute policy helpers, and never reports paths, contents, values, or raw read and parse failures. Values that cannot be proved locally remain uncertain instead of being replaced with a guessed default.

Claude then puts every valid default or managed target in one score-sorted, organization-deduplicated, reservation-locked candidate set. Claude reads usage through the official CLI without reading credential contents.

## Prefer plain English

Decision: Use short, direct language in output, docs, comments, tests, reviews, and change records.

Why: Clear names and messages reduce mistakes in a security-sensitive local tool.

Enforcement: Explain necessary technical terms once and avoid vague names when touching code.
