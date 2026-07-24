# Security and privacy contract

## Trust boundary

`multisubs` manages local routing metadata and isolated provider directories. It does not own either normal default provider account.

- The default Codex account is a read-only monitor source and a normal routing candidate.
- The default Claude account is a normal routing candidate.
- No command changes, copies, restores, backs up, links, or migrates either default account.

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

## Filesystem rules

Product state directories, profile directories, provider config directories, locks, routing metadata, and sensitive files must be private regular filesystem entries.

- Unsafe symlinks and hard links fail closed.
- Product-controlled runtime paths stay below `MULTISUBS_HOME`.
- Resource reconciliation does not overwrite regular user guidance, config, or skill entries.
- Only documented product-owned links may be created, changed, or removed.
- Runtime-managed `.system` skills remain profile-local.

A managed Codex `config.toml` is allowed only as a regular non-symlink with a verifiable hard-link count of one, or as a symlink whose resolved path exactly matches the resolved default Codex config and whose target is regular. One shared filesystem-only validator enforces this before any managed caller reads TOML. Hard-linked configs are rejected without automatic repair. Raw symlink targets may be shown in safe doctor diagnostics, but config contents are not exposed.

The default Codex account and its config remain unmanaged. This managed-config boundary does not copy, rewrite, or take ownership of default-account state.

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

The child then receives exactly the provider home required for its selected context. A managed Codex child also receives exactly one product variable: the selected `MULTISUBS_ACTIVE_PROFILE` marker added by multisubs. It does not inherit a caller-supplied marker. Default-account Codex, neutral provider help, and every Claude child receive no `MULTISUBS_*` variable. Default Codex execution receives no managed auth override, and neutral or default Claude execution receives no `CLAUDE_CONFIG_DIR`.

## Legacy-sensitive rejection

The old product namespace and state root are sensitive but unsupported.

- Any `MULTICODEX_*` variable rejects top-level startup before state access.
- Runtime never reads `MULTICODEX_HOME`.
- Runtime never defaults to `~/multicodex`.
- Monitor discovery prunes `~/multicodex`, `~/.multicodex`, and candidates canonically inside either root before reading usage signals.
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
- all combined and provider-only usage reports;
- aggregate and focused doctors;
- Codex dry run;
- exact provider help passthrough, including target-scoped Codex CLI help and target-scoped login help for both providers without requiring a configured profile.

## Usage and routing

Codex routing and monitoring use weekly usage only. Before automatic routing selection, fetched physical Codex targets are reconciled into logical subscriptions. All successful duplicates must agree on the requested bucket, availability, exhaustion, used percentage, and reset meaning or the whole group is skipped. A deterministic physical home is chosen only after agreement. A failed duplicate may use a consistent success only when protected official email identity safely groups them. The default account and managed profiles use the same weekly, model, and reset policy. Unavailable, exhausted, model-ineligible, identity-unverified, or conflicting fallback targets are skipped. Duplicate homes cannot add routing probability or capacity. The existing narrow fallback for older official responses remains limited to weekly-compatible data.

The unified usage report reads exactly the managed profiles in the two provider registries plus both normal default accounts. Shared typed target enumeration keeps the Codex default present exactly once even when a stored managed home is unsafe; that unsafe managed entry fails only its own row. Claude usage derives its targets from the same target owner as the other Claude commands. It does not read monitor account files, active-home overrides, discovered accounts, or observed-token estimates. It does not create directories, sidecars, sessions, threads, or other persistent provider state, and it does not persist identity in config.

Codex normalization retains one declared short/session window for reporting while routing and the live monitor continue to consume weekly fields only. A declared 300-minute window is the five-hour session. Otherwise only one unambiguous declared non-weekly duration is accepted; response position is never used to guess session meaning. Session-only primary data never suppresses weekly fallback or enables routing, selection metadata, or monitor window cards. The report-only source may merge that session into a fallback weekly result. Without weekly data the account remains partial and exit code 1 applies.

The user explicitly requested account identities in local usage output. The three usage commands therefore print a full validated account email beside each identified logical row by default. This exception is limited to local usage output. Account ID, user ID, organization ID, tokens, paths, raw identity conflicts, provider bodies, subprocess failures, cleanup errors, and unknown provider limit names are never printed there. Codex extra limits use only fixed product-owned labels, currently `Spark weekly`.

Codex reconciliation uses non-empty account ID as the strongest opaque internal key. A strict normalized email is the only fallback; user ID alone is never used. Different non-empty account IDs never merge merely because email matches. One email-only record may join one strong record with the same email. If it could match several strong IDs, it is conflicted and cannot add routing capacity. A failed Codex usage probe may recover only that normalized email through the existing protected auth-file reader. Claude usage identity requires official logged-in status, non-empty organization ID, and validated email without applying routing-only Max, provider, or method rules. One deadline covers auth, usage, and auth again. Different non-empty organization IDs never merge by email, and identity that changes or fails the second check remains ungrouped.

Duplicate logical subscriptions produce one usage row and one availability count. Managed aliases sort deterministically. A group containing the default account renders `(also default)` and stays last. One deterministic successful quota snapshot is selected without averaging. Missing, malformed, or conflicting display identity does not discard valid quota: the row says `identity unavailable`, is partial, and causes exit code 1.

Presentation aliases are allocated across the whole provider target set, remain unique, and put email-shaped profile names outside the valid profile-name alphabet. Codex reset instants stay UTC in the internal report and are converted only for local display. Claude reset text must be printable ASCII and match a strict supported reset grammar; arbitrary prose, controls, escapes, paths, key-like text, and malformed timezones become `reset unknown`. Per-account failures are reduced to fixed categories.

Claude routing scores the default account and managed profiles together using fresh official session and weekly all-model usage. It includes the Fable window only when that candidate's effective CLI and settings state says Fable is applicable or possible. The three-state policy fails closed per candidate: uncertainty requires Fable capacity but does not fail routing for other candidates.

To classify a candidate, routing inspects only `model`, `fallbackModel`, `env.ANTHROPIC_MODEL`, and the default Opus, Sonnet, Haiku, and Fable model mappings. It streams those fields from regular settings files capped at 2 MiB and does not retain or report unrelated settings. Read and parse failures are reduced to safe source categories; output never includes a settings path, content, value, or underlying error.

The default and managed user settings roots stay separate. Selected project and local settings, explicit `--settings`, and local macOS managed files are merged for the candidate without reading credentials or executing policy helpers. Server-managed, account, organization, or operating-system policy values that cannot be proved locally stay uncertain at field level. A conclusive higher-precedence CLI value can make an unknown lower value irrelevant.

Usage probe failure excludes only the affected candidate. Organization deduplication and reservation locking apply to every candidate. The tool does not infer usage from credential contents.

Both default accounts remain outside product ownership. Routing never changes their auth, config, or state.

## Repository leak protection

The repository keeps current and legacy-sensitive state patterns in `.gitignore` and doctor leak checks. It also checks for tracked credential-shaped paths and sensitive text.

Tests and examples use synthetic values and dummy paths. Upstream attribution to `olliecrow/multicodex` is not a runtime compatibility reference.
