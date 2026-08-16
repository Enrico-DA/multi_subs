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

Enforcement: Bare Codex routes fail with code 2 before state access and point to the matching namespaced route. Product-wide commands are `init`, `doctor`, `status`, and `usage`. `multisubs init` remains the shared initializer, and `multisubs codex init` calls the same path. `multisubs status` prints the same quota snapshot as `multisubs usage` and adds a Next section when any account is not ready.

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

## Keep the live monitor recoverable without stale targets

Decision: A failed scheduled account reload stops the current monitor targets, but not the monitor loop. The loop reports the fetch error and retries the account reload on its normal schedule.

Why: Continuing to fetch an old target set could spend or expose the wrong account after registry state changed. Exiting the whole interface would also require a manual restart after a short read failure. Keeping only the loop alive fails closed for provider access while allowing a later verified registry to recover.

Enforcement: A loader error closes and clears all current account fetchers before `Fetch` returns the existing no-accounts error. A reload that returns a verified safe set replaces the previous set, including with an empty set when every target was rejected.

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

Decision: Present Codex and Claude quota through one provider-neutral report model while keeping collection and routing rules provider-specific. Codex routing and the live monitor stay weekly-only. Successful monitor doctor fetch checks add only plan, source, and available weekly usage as structured usage fields. Claude routing uses fresh session and weekly all-model usage, plus Fable usage for each candidate whose effective model or fallback is applicable to Fable or cannot be classified conclusively. Each provider's default account competes normally with its managed profiles.

Why: Users need one quick quota snapshot without hiding account-level differences. Scripts also need monitor health values without parsing an English sentence, while unavailable data must not look like real usage. A shared formatter keeps labels, partial failures, and reset display consistent, but combining provider collectors or routing scores would weaken their different safety rules. Claude model settings can differ between the default account and each isolated managed profile. A single invocation-wide Fable decision can either exclude a valid account or spend against a window that the selected account does not need.

Enforcement: `multisubs usage` and both provider usage commands share one presentation model and renderer. Codex exec and usage share typed managed/default target enumeration and one account-ID-first identity reconciler from the usage package. A strictly validated normalized email is the Codex fallback, and user ID alone is never used. Automatic Codex routing compares requested weekly/model eligibility across every successful duplicate and fails the logical group closed on any bucket, availability, exhaustion, percentage, or reset disagreement before choosing a physical home. The report layer can recover only a normalized auth-file email after a Codex probe failure. It skips that read after choosing the unmanaged default app-server path. Claude usage uses a routing-independent logged-in organization/email validator and one deadline for official auth, usage, and auth again. It groups only unchanged identities. Different strong Codex account IDs and different Claude organization IDs remain separate even when email matches.

The usage report creates no multisubs product state, changes no credentials, has bounded per-account collection, closes each Codex source once after fetch cancellation, treats cleanup failure as a safe partial failure, and has no JSON form in this release. The official unmanaged default Codex app server may write non-credential logs, caches, database files, and database write-ahead files in the default home. The report excludes monitor-only, active-home, discovered, and observed-token sources. Duplicate logical subscriptions use one deterministically chosen successful quota snapshot and one availability count; percentages are never averaged. Managed aliases sort, a group containing default renders `(also default)`, and that row stays last. Codex extra-limit display uses fixed product-owned labels only, currently Spark. Claude session duration comes only from an explicit bounded parenthesized heading fragment. Claude reset text is rendered only through a strict allow-list grammar.

Monitor doctor JSON preserves the human details text. It adds structured usage fields only after a successful fetch, omits the weekly percentage when it is unavailable, and never exposes the internal unavailable value or a made-up zero. The new fields add no session windows or provider account identifiers to this weekly-only health contract.

The user explicitly chose full account emails for this local usage-only display. Email values pass one strict normalizer before display or fallback grouping. Opaque account, user, and organization IDs remain internal and are not persisted. Missing or conflicted identity retains valid quota as `identity unavailable`, marks the report partial, and exits with code 1.

Claude settings inspection is data-minimizing. It reads only routing fields from regular files capped at 2 MiB, does not execute policy helpers, and never reports paths, contents, values, or raw read and parse failures. Values that cannot be proved locally remain uncertain instead of being replaced with a guessed default.

Claude then puts every valid default or managed target in one score-sorted, organization-deduplicated, reservation-locked candidate set. Claude reads usage through the official CLI without reading credential contents.

## Measure and verify the default Codex account without blocking work silently

Decision: Routing and the unified usage report use one typed source policy for the default Codex account. A usable protected `auth.json` keeps direct OAuth as the only source, so the common path starts no process and adds no writes. Without a usable file, multisubs starts the official app server against the default home in unmanaged mode. The app-server source receives a sanitized environment, adds no managed file-store override, and does not read or fingerprint credentials. It still requires real weekly data.

Before launching a selected default account, `multisubs codex exec` also requires an explicit logged-in result from the official Codex CLI. It makes two bounded attempts separated by a short pause. If neither confirms login and another candidate exists, multisubs prints a prominent actionable stderr warning, excludes default for that command, and selects exactly once more from the remaining set. When no other candidate exists, it prints no reroute warning, because no reroute can happen; it exits with code 1 and one blocked message that states the same cause and fix. A replacement selection that then fails returns that same blocked message, because a selection failure can carry a local path or token-shaped text.

Why: The official login check honors the CLI's configured credential store, including a keyring, and does not confuse a missing `auth.json` with logout. The owner chose continued work over blocking the calling agent when another measured account can run the job. Every message names the default account and gives the exact fix `codex login`, so the calling agent surfaces the problem while the job still runs, and the quota redirect is visible instead of silently spending another subscription.

Each message states the cause with one of a fixed set of phrases chosen by observed login state: not logged in, check could not complete, or state not recognized. The provider's own status text is never included, because it can carry an account identity or a local path.

Trade-offs: The unmanaged app server may write its normal non-credential logs, caches, database files, and database write-ahead files in the default home. A failed default login can redirect work and spend quota on another usable account. Two status attempts and the pause between them add bounded delay, and the one fallback selection may measure the remaining accounts again. Fixed phrases mean the message says a check failed without saying how, so a novel provider failure still needs `codex login status` to diagnose. There is no loop, guessed usage, or unmeasured routing tier.

Enforcement: `internal/monitor/usage` owns the typed default source and keeps managed and unmanaged app-server modes distinct. The unmanaged source does not fingerprint auth and never receives `cli_auth_credentials_store="file"`. `internal/multisubs/exec.go` owns the two-attempt gate, fixed warning, typed default exclusion, and single reselection. `internal/multisubs/status.go` keeps account enrichment for status and doctor but gives exec a state-only probe. Tests cover the file-backed OAuth fast path, missing-file app-server path, sanitized unmanaged invocation, retry success, warned managed fallback, and exit code 1 when nothing remains.

## Prefer plain English

Decision: Use short, direct language in output, docs, comments, tests, reviews, and change records.

Why: Clear names and messages reduce mistakes in a security-sensitive local tool.

Enforcement: Explain necessary technical terms once and avoid vague names when touching code.
