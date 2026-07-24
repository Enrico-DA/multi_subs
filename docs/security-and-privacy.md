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
- aggregate and focused doctors;
- Codex dry run;
- exact provider help passthrough, including target-scoped Codex CLI help and target-scoped login help for both providers without requiring a configured profile.

## Usage and routing

Codex routing and monitoring use weekly usage only. The default account and managed profiles use the same weekly, model, and reset policy. Unavailable, exhausted, or model-ineligible accounts are skipped. The existing narrow fallback for older official responses remains limited to weekly-compatible data.

Claude routing scores the default account and managed profiles together using fresh official session, weekly all-model, and Fable usage. Probe failure excludes only the affected candidate. Organization deduplication and reservation locking apply to every candidate. The tool does not infer usage from credential contents.

Both default accounts remain outside product ownership. Routing never changes their auth, config, or state.

## Repository leak protection

The repository keeps current and legacy-sensitive state patterns in `.gitignore` and doctor leak checks. It also checks for tracked credential-shaped paths and sensitive text.

Tests and examples use synthetic values and dummy paths. Upstream attribution to `olliecrow/multicodex` is not a runtime compatibility reference.
