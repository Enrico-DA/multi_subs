# Security and privacy contract

## Trust boundary

`multisubs` manages local routing metadata and isolated provider directories. It does not own either normal default provider account.

- The default Codex account is a normal usage and routing candidate. Its official app-server usage probe may write non-credential operational state in the default Codex home.
- The default Claude account is a normal routing candidate.
- Multisubs never changes, copies, restores, backs up, links, or migrates either default account's credentials or configuration. The official Codex app server may write logs, caches, database files, and database write-ahead files while reading default-account usage. It does not change credentials.

## State isolation

The default state root is `~/multisubs`. `MULTISUBS_HOME` may replace it.

Each managed Codex profile keeps auth, sessions, threads, `/goal`, and related state inside:

```text
MULTISUBS_HOME/profiles/<name>/codex-home
```

Each managed Claude profile keeps official CLI state inside:

```text
MULTISUBS_HOME/providers/claude/profiles/<name>/config
```

The Codex and Claude registries remain separate. Their auth and routing stores are never merged.

## Credentials

- Never copy, sync, transmit, transfer, or share provider auth files or auth details between machines.
- Use official `codex login` and `claude auth login` flows.
- Managed Codex profiles require file-backed auth so each profile has an isolated `auth.json`.
- The product does not read, copy, or write Claude credential contents.
- Output must not include raw credentials or raw provider failure text.
- Provider-wrapper failures must not repeat caller-supplied argument values.

## Filesystem rules

Product state directories, profile directories, provider config directories, locks, routing metadata, and sensitive files must be private regular filesystem entries.

- Unsafe symlinks and hard links fail closed.
- Codex and Claude account configuration locks have a five-second acquisition deadline and never proceed unlocked.
- Product-controlled runtime paths stay below `MULTISUBS_HOME`, except the documented `multisubs install` path. That command may write `MULTISUBS_HOME/install.env`, write or replace one marked `GOPRIVATE`/`GOBIN` block in the login shell profile, and delete leftover regular `multisubs` files at the default Go bin path. It never deletes the running binary and never changes provider credentials.
- Resource reconciliation does not overwrite regular user guidance, config, or skill entries.
- Only documented product-owned links may be created, changed, or removed.
- Runtime-managed `.system` skills remain profile-local.

A managed Codex `config.toml` is allowed only as a regular non-symlink with a verifiable hard-link count of one, or as a symlink whose resolved path exactly matches the resolved default Codex config and whose target is regular. One shared filesystem-only validator enforces this before any managed caller reads TOML. Hard-linked configs are rejected without automatic repair. Raw symlink targets may be shown in safe doctor diagnostics, but config contents are not exposed.

The default Codex account and its config remain unmanaged. This managed-config boundary does not copy, rewrite, or take ownership of default-account state.

Doctor treats group- or world-readable managed Codex credentials as a failed check and does not start that profile's dependent login-status probe. Other doctor checks continue. The finding does not expose the credential path, permission bits, or contents.

## Environment rules

Official provider variables remain:

- `CODEX_HOME`
- `CLAUDE_CONFIG_DIR`

Active product controls use `MULTISUBS_*`.

Before a provider child starts, the environment removes:

- stale provider home overrides;
- API keys, tokens, base URL overrides, and provider selectors;
- every inherited `MULTISUBS_*` variable, including unknown future controls;
- all legacy `MULTICODEX_*` controls.

The child then receives exactly the provider home required for its selected context. A managed Codex child also receives exactly one product variable: the selected `MULTISUBS_ACTIVE_PROFILE` marker added by multisubs. It does not inherit a caller-supplied marker. Default-account Codex, neutral provider help, and every Claude child receive no `MULTISUBS_*` variable. Default Codex execution, automatic `multisubs codex cli`, `multisubs codex generate`, and the unmanaged app-server usage probe receive no managed auth override, and neutral or default Claude execution receives no `CLAUDE_CONFIG_DIR`. Before launching a selected default Codex account, `multisubs codex exec`, automatic `multisubs codex cli`, and `multisubs codex generate` make two bounded official `codex login status` attempts in the default Codex home. The check honors the credential store configured for that CLI. It does not treat a missing `auth.json` as proof of logout. It must not mutate default auth state or expose default auth details or raw subprocess output.

## Legacy-sensitive rejection

The old product namespace and state root are sensitive but unsupported.

- Any `MULTICODEX_*` variable rejects top-level startup before state access.
- Runtime never reads `MULTICODEX_HOME`.
- Runtime never defaults to `~/multicodex`.
- Monitor discovery prunes `~/multicodex`, `~/.multicodex`, and candidates canonically inside either root before reading usage signals.
- Monitor discovery never adds candidates canonically inside `MULTISUBS_HOME`; registered managed profiles come from `config.json`.
- There is no old executable alias or compatibility command.
- `.multicodex`, `multicodex` state paths, and old environment names remain only in ignore, leak, denylist, and rejection tests so old credentials cannot be committed or inherited.

This rename phase does not migrate live state or an installed binary.

## Read-only commands

These paths must not create product state:

- help and nested provider help;
- version;
- completion and dynamic profile completion;
- unknown commands and rejected arguments;
- `multisubs codex status`;
- aggregate and focused doctors;
- Codex dry run;
- exact provider help passthrough, including target-scoped Codex CLI help and target-scoped login help for both providers without requiring a configured profile.

Usage reports create no multisubs product state and do not change credentials. The official default Codex app-server fallback may write non-credential operational state in the default home. A monitor-doctor run explicitly asked to use the app server can do the same.

## Usage and routing

Codex routing and monitoring use weekly usage only. Before automatic routing selection, fetched physical Codex targets are reconciled into logical subscriptions. All successful duplicates must agree on the requested bucket, availability, exhaustion, used percentage, and reset meaning or the whole group is skipped. A deterministic physical home is chosen only after agreement. A failed duplicate may use a consistent success only when protected official email identity safely groups them. The default account and managed profiles use the same weekly, model, and reset policy once usable usage is available. The default account uses OAuth directly when a private regular `auth.json` contains `tokens.access_token`; no app-server process starts in that case. If that file also contains a safe account id, the request sends it as `ChatGPT-Account-Id` so the usage endpoint can scope the same ChatGPT account. The id may come from `tokens.account_id`, top-level `account_id`, or sanitized token claims. The identifier, tokens, and claims are never printed. Without a usable file, the official app server runs against the default home in unmanaged mode. The app-server source does not read or fingerprint credential material and does not add the managed file-store override. It must return real weekly data. Other unavailable, exhausted, model-ineligible, identity-unverified, or conflicting fallback targets are skipped during scoring. Duplicate homes cannot add routing probability or capacity. The existing narrow fallback for older official responses remains limited to weekly-compatible data.

OAuth eligibility is authoritative when present. An explicit `allowed: false` makes that rate-limit bucket unavailable, and an explicit `limit_reached: true` makes it exhausted. Omitted flags preserve the older-response compatibility path without creating a guessed eligibility signal.

The live monitor never keeps fetching a target set after its scheduled reload fails. It closes and clears that set, exposes the reload problem through the normal fetch-error state, and keeps only the monitor loop alive so a later verified reload can recover. If a reload can verify some safe targets, it replaces the old set and excludes rejected targets.

The selected default account has a separate launch gate after scoring. Only an explicit logged-in result from the official Codex CLI passes. Multisubs tries twice, with a short pause between attempts. If login is still not confirmed and another candidate exists, it prints a prominent stderr warning that names default, states a fixed safe cause, gives the exact fix `codex login`, excludes default for that command, and selects once more from the remaining candidates. This makes every quota redirect visible. When no other candidate exists, or replacement selection fails, it exits with code 1 and one blocked message carrying the same cause and fix. Neither message exposes raw subprocess output or raw selection failures.

`multisubs codex generate` uses the selected home's existing managed ChatGPT authentication through Codex App Server and pins Codex's built-in OpenAI provider. It requires Codex CLI 0.147.0, ignores unrelated custom model providers, disables notification hooks, fails closed if config replaces the built-in provider or its endpoint or loads a configuration lockfile, requests no token refresh, rejects API-key authentication, discards App Server standard error, and never reads or prints credential values. Generation workspaces and model catalogs are private temporary files. Generation input files are read only from paths selected by the caller and are bounded independently. File-read errors do not print those paths. JSON result mode buffers a bounded response and exposes only response text, model, effective effort, elapsed milliseconds, and numeric token usage.

Claude usage treats an official not-logged-in auth status as `not logged in` even when the `/usage` probe returns unreadable text. A first official logged-out result skips `/usage`. That keeps status and doctor aligned and points Next at `claude auth login` instead of `multisubs doctor`.

The unified usage report reads exactly the managed profiles in the two provider registries plus both normal default accounts. Shared typed target enumeration keeps the Codex default present exactly once even when a stored managed home is unsafe; that unsafe managed entry fails only its own row. Claude usage derives its targets from the same target owner as the other Claude commands. It does not read monitor account files, active-home overrides, discovered accounts, or observed-token estimates. It does not create multisubs directories or persist identity in config. The official unmanaged default Codex app server may write non-credential logs, caches, database files, and database write-ahead files in the default home.

Codex normalization retains one declared short/session window for reporting while routing and the live monitor continue to consume weekly fields only. A declared 300-minute window is the five-hour session. Otherwise only one unambiguous declared non-weekly duration is accepted; response position is never used to guess session meaning. Session-only primary data never suppresses weekly fallback or enables routing, selection metadata, or monitor window cards. The report-only source may merge that session into a fallback weekly result. Without weekly data the account remains partial and exit code 1 applies.

The user explicitly requested account identities in local usage output. The three usage commands therefore print a full validated account email beside each identified logical row by default. This exception is limited to local usage output. Account ID, user ID, organization ID, tokens, paths, raw identity conflicts, provider bodies, subprocess failures, cleanup errors, and unknown provider limit names are never printed there. Codex extra limits use only fixed product-owned labels, currently `Spark weekly`. When a usage or status snapshot is not complete, a Next section may print one exact command per failed account from a closed allow-list. Profile names in those commands are validated registry names. Default-account login commands are the official provider CLIs. Next-step reasons are the same fixed safe failure phrases already used in the quota rows.

Codex reconciliation uses non-empty account ID as the strongest opaque internal key. A strict normalized email is the only fallback; user ID alone is never used. Different non-empty account IDs never merge merely because email matches. One email-only record may join one strong record with the same email. If it could match several strong IDs, it is conflicted and cannot add routing capacity. A failed Codex usage probe may recover only that normalized email through the existing protected auth-file reader, except after the unmanaged default app-server path is chosen. That path performs no later auth-file identity read. Claude usage identity requires official logged-in status, non-empty organization ID, and validated email without applying routing-only Max, provider, or method rules. One deadline covers auth, usage, and auth again. Different non-empty organization IDs never merge by email, and identity that changes or fails the second check remains ungrouped.

Duplicate logical subscriptions produce one usage row and one availability count. Managed aliases sort deterministically. A group containing the default account renders `(also default)` and stays last. One deterministic successful quota snapshot is selected without averaging. Missing, malformed, or conflicting display identity does not discard valid quota: the row says `identity unavailable`, is partial, and causes exit code 1.

Presentation aliases are allocated across the whole provider target set, remain unique, and put email-shaped profile names outside the valid profile-name alphabet. Codex reset instants stay UTC in the internal report and are converted only for local display. Claude reset text must be printable ASCII and match a strict supported reset grammar; arbitrary prose, controls, escapes, paths, key-like text, and malformed timezones become `reset unknown`. Per-account failures are reduced to fixed categories.

Claude routing scores the default account and managed profiles together using fresh official session and weekly all-model usage. It includes the Fable window only when that candidate's effective CLI and settings state says Fable is applicable or possible. The three-state policy fails closed per candidate: uncertainty requires Fable capacity but does not fail routing for other candidates.

To classify a candidate, routing inspects only `model`, `fallbackModel`, `env.ANTHROPIC_MODEL`, and the default Opus, Sonnet, Haiku, and Fable model mappings. It streams those fields from regular settings files capped at 2 MiB and does not retain or report unrelated settings. Read and parse failures are reduced to safe source categories; output never includes a settings path, content, value, or underlying error.

The default and managed user settings roots stay separate. Selected project and local settings, explicit `--settings`, and local macOS managed files are merged for the candidate without reading credentials or executing policy helpers. Server-managed, account, organization, or operating-system policy values that cannot be proved locally stay uncertain at field level. A conclusive higher-precedence CLI value can make an unknown lower value irrelevant.

Usage probe failure excludes only the affected candidate. Organization deduplication and reservation locking apply to every candidate. The tool does not infer usage from credential contents.

Timed Codex and Claude probes cap captured output at 1,000,000 bytes and bound output-pipe draining after cancellation. Truncated output is treated as incomplete, is not parsed or reported as provider content, and cannot establish routing or authentication state.

Both default accounts remain outside product ownership. Routing never changes their credentials or configuration. The official default Codex app server may write only its normal non-credential operational state while reporting usage.

## Repository leak protection

The repository keeps current and legacy-sensitive state patterns in `.gitignore` and doctor leak checks. It also checks for tracked credential-shaped paths and sensitive text.

Tests and examples use synthetic values and dummy paths. Upstream attribution to `olliecrow/multicodex` is not a runtime compatibility reference.

No real person's email address or name belongs in this repository. Test and example identities use reserved domains, currently `example.com`, `example.net`, `example.org`, and any `.test`, `.example`, `.invalid`, or `.localhost` name, with synthetic local parts. The sensitive-text policy rejects any other email domain in changed files, commit messages, patches, and pull request text, and redacts the address in its own output. Real names cannot be detected automatically and remain a review responsibility.
