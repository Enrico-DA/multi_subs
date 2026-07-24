# Command contract

This document defines the public `multisubs` command surface.

## Exit behavior

- `0`: command completed successfully.
- `1`: an operational command or doctor check failed.
- `2`: invalid command, invalid arguments, unsafe setup, or legacy product environment.

Unknown commands and rejected arguments must not create product state.

## Top level

### `multisubs init`

Creates private shared product directories and the Codex profile registry under `MULTISUBS_HOME`, which defaults to `~/multisubs`. It does not change either default provider account.

No extra arguments are accepted.

### `multisubs doctor [--json] [--timeout 8s]`

Runs an aggregate read-only report with these sections:

1. `shared/base`: product state, config, resource policy, repository path isolation, ignore coverage, and tracked-sensitive-file checks.
2. `Codex`: Codex binary, default Codex home, managed profile paths, config, auth shape, and login status.
3. `Claude`: Claude binary, provider registry, managed paths, authentication status, and duplicate-organization checks.

The JSON result has `base`, `codex`, and `claude` objects. Each contains a `checks` array.

### `multisubs completion <bash|zsh|fish>`

Prints completion for both provider namespaces, all nested help and monitor topics, and dynamic Codex and Claude profile names. It is read-only and does not create config.

### `multisubs version`

Prints `multisubs <version>`. `--version` and `-v` are accepted aliases. Extra arguments are rejected.

### `multisubs help [topic]`

Prints global help or a topic with up to three nested words, such as:

```text
multisubs help codex exec
multisubs help codex monitor doctor
multisubs help claude usage
```

Help is read-only.

## Codex namespace

### `multisubs codex init`

Calls the same initialization path as `multisubs init`. No extra arguments are accepted.

### `multisubs codex add <name>`

Creates and registers one isolated profile under `MULTISUBS_HOME/profiles/<name>/codex-home`. It applies the configured resource policy without overwriting regular user files.

The name `default` is reserved for the built-in default Codex account. Add rejects it with exit code 2 before creating state. Stored Codex registries using that managed profile name are invalid.

### `multisubs codex login <name> [codex login args...]`

Runs official `codex login` with the profile-local `CODEX_HOME`. User arguments keep their order. The managed file-backed-auth override is appended.

### `multisubs codex login-all`

Runs login for every configured Codex profile in sorted order. No extra arguments are accepted.

### `multisubs codex cli <name> [codex args...]`

Runs the official interactive Codex CLI with the named profile. The child receives the profile-local `CODEX_HOME` and active profile marker. Product controls and account override variables are removed first.

### `multisubs codex exec [codex exec args...]`

Runs official `codex exec` after weekly-only account selection.

- The default account and managed profiles have equal selection priority.
- Accounts with unavailable or exhausted weekly usage are skipped.
- Known weekly resets are tried soonest first.
- A requested Spark model requires that account's Spark weekly bucket.
- Managed profile children receive file-backed-auth isolation.
- Default-account execution uses the default Codex home without a managed file-auth override or product mutation.
- Exact provider help requests pass through without config or state creation.
- Optional selected-profile metadata is confined to `MULTISUBS_HOME/run`.

### `multisubs codex status`

Shows safe, profile-local authentication state. It is read-only and accepts no extra arguments.

### `multisubs codex reconcile`

Applies the current guidance and skill resource policy to every Codex profile. It does not inspect auth or launch Codex. It accepts no extra arguments.

### `multisubs codex heartbeat`

Sends a small ephemeral, read-only `codex exec` request to each logged-in managed profile. It uses a non-blocking private lock under `MULTISUBS_HOME`.

Settings:

- `MULTISUBS_HEARTBEAT_TIMEOUT_SECONDS`
- `MULTISUBS_HEARTBEAT_RETRIES`
- `MULTISUBS_HEARTBEAT_BACKOFF_SECONDS`
- `MULTISUBS_HEARTBEAT_LOCK_PATH`

The lock override must resolve inside `MULTISUBS_HOME`.

### `multisubs codex monitor [flags]`

Runs the Codex usage terminal interface.

Nested topics:

- `multisubs codex monitor tui [flags]`
- `multisubs codex monitor doctor [flags]`
- `multisubs codex monitor completion [shell]`
- `multisubs codex monitor help`

The monitor uses official weekly data. Validated managed profiles try the Codex app server first and use the existing narrow OAuth fallback. Default and active homes follow their explicit inclusion rules.

`MULTISUBS_MONITOR_ACCOUNTS_FILE` may point to an explicit monitor account file.

### `multisubs codex doctor [--json] [--timeout 8s]`

Runs only the focused Codex checks. It does not include Claude checks or create state.

### `multisubs codex dry-run [operation]`

Prints planned Codex work without changing files or launching Codex. The supported operation-specific form is:

```text
multisubs codex dry-run login <name>
```

### `multisubs codex help [command]`

Prints Codex namespace or command help without state mutation.

## Claude namespace

### `multisubs claude add <name>`

Creates one managed Claude profile under `MULTISUBS_HOME/providers/claude/profiles/<name>/config` and saves provider metadata in the separate Claude registry.

The name `default` is reserved for the built-in default Claude account and cannot be used for a managed profile.

### `multisubs claude login <name> [claude auth login args...]`

Runs official `claude auth login --claudeai` with the managed profile's derived `CLAUDE_CONFIG_DIR`. It verifies subscription auth and rejects duplicate organizations.

### `multisubs claude cli <name|default> [claude args...]`

Runs the official interactive Claude CLI. A managed target receives its derived `CLAUDE_CONFIG_DIR`. The `default` target receives no `CLAUDE_CONFIG_DIR`.

### `multisubs claude exec [claude -p args...]`

Runs official Claude print mode after fresh target-scoped auth and usage checks.

- The default account and usable managed profiles share one candidate list.
- Candidates rank by their worst applicable session, weekly all-model, or Fable percentage, then by name.
- Duplicate organizations are removed before execution.
- A busy candidate is skipped while the next candidate is tried.
- If every eligible candidate is busy, the command returns the normal busy error. If none is usable, it returns one no-usable-account error.
- Default-account execution has no `CLAUDE_CONFIG_DIR`.

### `multisubs claude status`

Uses official `claude auth status --json` for the default account and each managed profile. It accepts no extra arguments.

### `multisubs claude usage`

Uses the free official `/usage` command and reports session, weekly all-model, and Fable windows. It accepts no extra arguments.

### `multisubs claude doctor`

Runs only the focused Claude binary, registry, path, authentication, and duplicate-organization checks. It is read-only and accepts no extra arguments.

### `multisubs claude help [command]`

Prints Claude namespace or command help without state mutation. Exact official provider help requests remain non-mutating.

## Removed bare Codex commands

The following top-level routes are rejected before state access:

```text
add
login
login-all
cli
exec
status
reconcile
heartbeat
monitor
dry-run
```

Each exits with code 2 and points to the matching `multisubs codex ...` route.

## Legacy environment rejection

Startup checks the environment before path resolution. If any `MULTICODEX_*` variable is present, the command exits with code 2 and tells the user to clear it.

Runtime never reads the old environment namespace or the old `~/multicodex` state root. Known old variables remain on provider child-environment denylists to prevent account-routing leakage.
