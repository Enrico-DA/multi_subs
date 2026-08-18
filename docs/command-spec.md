# Command contract

This document defines the public `multisubs` command surface.

## Exit behavior

- `0`: command completed successfully.
- `1`: an operational command or doctor check failed.
- `2`: invalid command, invalid arguments, unsafe setup, or legacy product environment.

Unknown commands and rejected arguments must not create product state.

Provider-wrapper failures identify the provider command or multisubs operation without repeating caller-supplied argument values.

Codex and Claude account registry writes wait at most five seconds for their private configuration lock. A timeout fails the command without running the locked operation.

## Managed Codex config contract

Every managed Codex `config.toml` must be either:

- a regular, non-symlink file with a verifiable hard-link count of exactly one; or
- a symlink whose fully resolved path is exactly the fully resolved default Codex `config.toml`, and whose target is regular.

All managed command setup, execution readiness, status, doctor, model inspection, and monitor account loading use the same filesystem validator before parsing TOML or starting a provider process. Hard-linked configs fail without automatic repair. The default Codex account and default config stay unmanaged and keep their separate default-account rules.

Existing valid default-config symlinks and single-link manual overrides need no migration. The exact old generated regular config may still become a default-config symlink during managed setup. Any hard-linked config, arbitrary symlink, broken link, or non-regular config must be fixed manually.

## Top level

### `multisubs init`

Creates private shared product directories and the Codex profile registry under `MULTISUBS_HOME`, which defaults to `~/multisubs`. It does not change either default provider account.

No extra arguments are accepted.

### `multisubs doctor [--json] [--timeout 8s]`

Runs an aggregate read-only report with these sections:

1. `shared/base`: binary version, product state, config, resource policy, repository path isolation, ignore coverage, and tracked-sensitive-file checks. The version check uses the same string as `multisubs version`. It warns when that string is still the compile-time `0.1.0-dev` default, because that cannot tell two development builds apart.
2. `Codex`: Codex binary (including a warning when the installed CLI cannot run `multisubs codex generate`), default Codex home, managed profile paths, config, auth shape, and login status.
3. `Claude`: Claude binary, provider registry, managed paths, authentication status, and duplicate-organization checks.

The JSON result has `base`, `codex`, and `claude` objects. Each contains a `checks` array.
After valid argument parsing, all three sections are emitted even when the Codex profile registry is malformed, uses an unsupported version, or contains invalid stored names. That registry error becomes a failed shared/base check, and safe Codex and independent Claude checks continue against an empty Codex profile set.
Any failed check makes the human summary `FAIL` and the command exit with code 1, regardless of check order or successful checks in another section.

### `multisubs status`

Prints the same Codex and Claude quota snapshot as `multisubs usage`. When the result is not complete, it also prints a `Next` section. Each next-step row names the account with a fixed safe reason and one exact command from a closed allow-list: `codex login`, `claude auth login`, `multisubs codex login <name>`, `multisubs claude login <name>`, `multisubs init`, or `multisubs doctor`. Profile names in those commands are validated registry names. Raw provider text, paths, and credentials never appear. Extra arguments exit with code 2.

### `multisubs usage`

Prints one quota report with a Codex section followed by a Claude section. `multisubs codex usage` and `multisubs claude usage` filter the same report model and formatter.

The physical target set is exact:

1. every managed Codex profile in name order, then the normal default Codex account;
2. every managed Claude profile in name order, then the normal default Claude account.

Monitor-only account-file entries, active-home overrides, and filesystem-discovered accounts are excluded. No usage command creates multisubs product state, changes provider credentials, or persists identity in config. If the default Codex account has no usable `auth.json`, its official app-server probe may write non-credential logs, caches, database files, and database write-ahead files in the default Codex home. It does not change credentials.

The physical targets are reconciled into logical subscriptions before output. A duplicate group has one row, one availability count, sorted managed aliases, and one deterministic successful quota snapshot; quota is never averaged. A failed Codex probe may still join a successful duplicate when its protected local auth file supplies the same strictly validated official email. The failed member keeps the logical row partial. A group that contains the default account ends with `(also default)` and stays in the provider's final position.

This local-only report prints a full, strictly validated, normalized account email beside every identified row. Codex uses non-empty account ID as its strongest internal key and validated email as fallback. User ID alone is never an identity. Different non-empty account IDs remain separate even when their emails match. An email-only record joins one strong record with the same email, but cannot join when that email matches several strong IDs. Claude uses the official organization ID as its strongest internal key; email is display-only, and different organizations never merge by email. Account, user, and organization IDs are never printed.

Percentages mean used quota. Structured Codex resets render a countdown and exact local time. Missing resets are `reset unknown`; expired resets are `reset due`. Codex prints only fixed product-owned extra-limit labels, currently `Spark weekly`; unknown provider limit names are suppressed. Claude reset text is printed only when it matches a supported countdown, weekday, month-and-day, or time-only grammar, with an optional safe IANA timezone. All other provider reset text becomes `reset unknown`; no timestamp or timezone is invented.

A missing optional window is `not reported`. A failed account is `unavailable` with a fixed safe reason. If identity is missing, malformed, or conflicted while quota succeeds, the quota row is retained as a separate row, its identity is `identity unavailable`, and it contributes one unavailable count because no safe logical collapse is proven. A safe Codex session can remain visible with a `partial` reason when required weekly data is unavailable. Source cleanup failure uses a fixed safe reason and fails that account. Every success is still printed. When the result is not complete, a `Next` section lists the exact command for each failed account using the same closed allow-list as `multisubs status`. Exit code 0 means every logical account probe and identity check succeeded, exit code 1 means at least one account, identity, or provider failed, and exit code 2 means invocation misuse. No arguments or flags are accepted. `--json` is not available in this release.

### `multisubs completion <bash|zsh|fish>`

Prints completion for both provider namespaces, all nested help and monitor topics, and dynamic Codex and Claude profile names. It is read-only and does not create config.

`multisubs codex help <command>` and `multisubs claude help <command>` are completion leaves after the one accepted command topic.

### `multisubs version`

Prints `multisubs <version>`. `--version` and `-v` are accepted aliases. Extra arguments are rejected. A release override wins. Otherwise the command prints the Go module version from `go install` or a short VCS revision. The compile-time `0.1.0-dev` default is last.

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

Exact `multisubs codex login <name> --help` and `multisubs codex login <name> -h` requests instead run neutral official `codex login` help as `codex login <flag>`. The named profile need not exist. These forms do not load config, create or reconcile product state, add managed auth or profile markers, inspect auth, or run post-login verification. Any help flag mixed with extra login arguments exits with code 2 before state access.

### `multisubs codex login-all`

Runs login for every configured Codex profile in sorted order. No extra arguments are accepted.

### `multisubs codex cli [<name>|--account <name>] [codex args...]`

Runs the official interactive Codex CLI. Without a profile name, it uses the same weekly-only account selection, identity reconciliation, equal default/managed priority, and default-login gate as `multisubs codex exec`. A leading profile name or `--account <name>` bypasses routing and launches that managed profile even when its weekly usage is exhausted or unavailable. Manual mode prepares only the named profile, so unrelated profile errors do not block it. Automatic mode prepares and validates every configured profile before selection and fails closed if any configured profile is unsafe.

Inherited product controls and account override variables are removed first. A managed child receives its profile-local `CODEX_HOME` and only the selected `MULTISUBS_ACTIVE_PROFILE` marker. A default-account launch uses the default Codex home with no managed file-auth override and no `MULTISUBS_*` variable.

Exact `multisubs codex cli <name> --help` and `multisubs codex cli <name> -h` requests instead run official Codex help with only the help flag and a neutral sanitized environment. The named profile need not exist. These requests do not load config, create product state, reconcile resources, add the active profile marker, or force managed auth. `cli --help` and `cli -h` are product help. A help flag mixed with any other Codex argument is rejected with exit code 2; `--` still ends option handling.

### `multisubs codex exec [--search] [codex exec args...]`

Runs official `codex exec` after weekly-only account selection.

- `--search` is moved before the `exec` subcommand because Codex defines it as a global flag. `--search` after `--` stays prompt text.

- The default account and managed profiles have equal selection priority.
- Fetched physical targets are reconciled by official Codex account identity before selection. Successful duplicates must agree on requested-bucket presence, weekly availability, exhaustion, used percentage, and reset meaning. When both snapshots carry an absolute reset timestamp, those timestamps must match exactly even if their countdowns differ. Only when both snapshots lack an absolute timestamp may relative countdowns differ, and then by at most five seconds to cover concurrent-fetch drift. Known and unknown resets, mixed absolute and relative-only resets, and larger drift disagree. Disagreement excludes the whole logical group. A deterministic physical home is chosen only after agreement. One failed duplicate can fall back to a consistent success only when protected official email identity safely joins them. Missing or conflicting fallback identity contributes no separate capacity.
- Accounts with unavailable or exhausted weekly usage are skipped.
- OAuth `allowed: false` makes that rate-limit bucket unavailable, and `limit_reached: true` makes it exhausted. Omitted eligibility flags keep the existing older-response behavior; they do not add a new fallback.
- Known weekly resets are tried soonest first.
- A requested Spark model requires that account's Spark weekly bucket.
- Effective model routing recognizes Codex `--model`/`-m` flags and exact root `model` values passed through `-c`/`--config`; the dedicated model flag has Codex's higher precedence.
- Without an explicit model or profile selector, all candidate `config.toml` files must declare the same root model or all omit it. Conflicts exit with code 2 and require `--model`.
- `--profile`/`-p` without an explicit model exits with code 2 because the selected Codex config can change the model.
- Managed profile children receive file-backed-auth isolation.
- Routing and the unified usage report use the same default-account source rule. A private regular `auth.json` with `tokens.access_token` uses OAuth directly and starts no app-server process. If that file also has a safe account id, the OAuth request includes `ChatGPT-Account-Id`. The id may come from `tokens.account_id`, top-level `account_id`, or sanitized token claims. That identifier is never printed. When the auth file is not usable, the official app server runs against the default home in unmanaged mode. It receives a sanitized environment, no managed file-store override, and no credential read or fingerprint from the app-server source. The official process may write non-credential logs, caches, database files, and database write-ahead files in the default home. It does not change credentials.
- The unmanaged fallback must return real weekly data. It does not invent a percentage or create an unmeasured routing tier. The default account then follows the same identity reconciliation, weekly scoring, reset, and Spark rules as managed accounts.
- Default-account execution uses the default Codex home without a managed file-auth override or default-credential mutation. After selection, exec makes two bounded `codex login status` attempts in that home, separated by a short pause so a momentarily busy home is not read as a login failure. The official check honors the credential store configured for the Codex CLI and does not treat a missing `auth.json` as proof of logout.
- Only an explicit logged-in result passes the launch check. If neither attempt confirms login and another candidate exists, exec prints a prominent stderr warning that names the default account, states the cause, says `Run: codex login`, and says that default is being skipped. It excludes default and selects exactly once more from the remaining candidates. A redirected job always has this warning.
- The stated cause is one of a fixed set of phrases selected by observed state: not logged in, check could not complete, or state not recognized. Raw `codex login status` output never appears.
- When no other candidate exists, no reroute warning is printed, because no reroute can happen. Exec exits with code 1 and one blocked message carrying the same cause and the same `Run: codex login` fix. Replacement selection that then fails returns that same blocked message. Raw selection or provider failure output never appears in it.
- Exact provider help requests pass through without config or state creation.
- Optional selected-profile metadata is confined to `MULTISUBS_HOME/run`.

### `multisubs codex generate [--account <name>] [-m|--model <model>] [--effort <effort>] [--base-instructions-file <path>] [--developer-instructions-file <path>] [--output-schema <path>] [--json] [prompt]`

Sends one text prompt through Codex App Server using ChatGPT subscription authentication and Codex's built-in OpenAI provider.

- Reads the prompt from standard input when no prompt argument is present and rejects input larger than 4 MiB.
- Uses the same weekly-aware routing, identity reconciliation, equal default/managed priority, and default-login gate as `multisubs codex exec`, unless `--account <name>` selects one managed profile directly.
- Passes an explicit model to the existing model-aware selector. Otherwise, uses the highest-priority visible model in the installed Codex bundled catalog.
- Validates `--effort` against the selected model's bundled supported-effort metadata. Without the flag, uses the selected model's bundled default effort.
- Requires exactly `codex-cli 0.147.0` and fails before generation for any other version.
- Requires App Server `account/read` to report managed ChatGPT authentication. It rejects API-key billing and does not start login or token-refresh flows.
- Runs an ephemeral thread in a private empty temporary directory with read-only sandboxing and approval policy `never`.
- Sends exact base or developer instruction file contents when selected and empty client instructions otherwise. The prompt and each optional input file have independent 4 MiB limits; file inputs must resolve to regular files. File-read errors do not print those paths.
- Accepts `--output-schema` only when the file contains a JSON object and passes that object to App Server for structured-output enforcement.
- Disables client context sources, tools, MCP servers, and notification hooks, ignores unrelated custom model providers, and uses a private `0600` one-model catalog with tool metadata removed.
- Fails closed if Codex config replaces the built-in OpenAI provider, overrides its endpoint, or loads a configuration lockfile.
- Rejects server requests, command, file, web, image, and unexpected item events.
- By default, streams only assistant text to standard output. Resource notices and safe errors use standard error. A failure can occur after partial text was written.
- With `--json`, buffers up to 16 MiB of assistant text and writes one object only after success. The object contains response text, model, effective effort, elapsed milliseconds, and numeric token usage. Usage is `null` when App Server emits no usage event. It excludes account and profile identifiers, paths, reasoning text, and raw events. A generation failure or response-limit failure writes no JSON object.
- Writes selected-profile metadata under `MULTISUBS_HOME/run` when `MULTISUBS_SELECTED_PROFILE_PATH` is set. The selection source is `explicit_account`, `usage_selector`, or `usage_selector_default`.
- Deletes the temporary workspace and model catalog when the command ends.
- Re-checks profile filesystem and file-backed auth isolation before a configured-profile run. It never changes or manages default account authentication.

### `multisubs codex status`

Shows safe, profile-local authentication state for every managed Codex profile and the normal default Codex account. Default stays unmanaged: the probe uses the default home without a managed file-auth override. When a row is not logged in or cannot be checked, a `Next` section prints the exact command using the same closed allow-list as `multisubs status`. It is otherwise read-only and accepts no extra arguments.

### `multisubs codex usage`

Prints the Codex-only view of the shared usage report. It shows `Session (5h)` for a declared 300-minute window. If there is no declared five-hour window, one unambiguous declared non-weekly window is labeled with its actual duration. Missing durations are not guessed by position, and several ambiguous non-weekly durations leave session usage unreported.

The existing declared 10,080-minute weekly selection and narrow older-response fallback stay unchanged. A primary result without weekly data still triggers fallback. The report-only managed source can merge a safe primary session with fallback weekly data while keeping fallback identity and weekly limit fields together. If fallback fails, a retained session is partial and the account still fails strict success. Shared routing and monitor sources do not use this report-only merge. Only fixed product-owned extra-limit labels are reported; the current known label is `Spark weekly`. A Spark-only snapshot is not standard weekly data: status keeps the Spark row and marks the account `weekly usage unavailable`. Managed profiles reuse the validated app-server-to-OAuth source path. The normal default account uses OAuth when its usable protected auth file has an access token, sending `ChatGPT-Account-Id` when that file has a safe account id. The id may come from `tokens.account_id`, top-level `account_id`, or sanitized token claims. It is never printed. Otherwise the official app server runs in unmanaged mode. Routing uses the same typed default source. Duplicate logical subscriptions use one deterministic successful snapshot. The command does not use the TUI or observed-token estimates.

Codex account and model scoring remains weekly-only. Identity reconciliation happens before that existing scoring. No arguments are accepted.

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

`multisubs codex monitor help` accepts no arguments and is a completion leaf.

The monitor uses official weekly data. Validated managed profiles try the Codex app server first and use the existing narrow OAuth fallback. An included default home follows the same OAuth-or-unmanaged-app-server rule as routing and the usage report. A requested monitor-doctor app-server probe may therefore start the official process against the default home, where it can write non-credential logs, caches, database files, and database write-ahead files without changing credentials. Active homes follow their explicit inclusion rules. With `--discover`, filesystem candidates canonically inside `MULTISUBS_HOME` are excluded; registered managed profiles still come from `config.json`. It remains the live Codex view; `multisubs usage` is the separate quick snapshot.

The live monitor reloads its account set on the polling schedule. A failed reload closes and clears the current targets before the next fetch, reports the reload error through the existing fetch-error state, and retries later. The monitor loop stays open so a repaired registry can recover without a restart. A reload that can still verify a safe target set replaces the old set and excludes every rejected target.

`multisubs codex monitor doctor --json` keeps `name`, `ok`, and the human `details` sentence for every check. A successful usage-fetch check also includes `plan_type`, `source`, and the numeric `weekly_used_percent`. Each structured value is omitted when it is unavailable. In particular, unavailable weekly usage omits `weekly_used_percent` while `details` still says `weekly=unavailable`. Failed fetch checks keep the safe error in `details` and omit all three structured usage fields. The new fields add no session windows, provider account identifiers, paths, or raw provider payloads.

Any failed monitor-doctor check makes the human summary `FAIL` and exits with code 1. A successful fetch does not hide another fetch or setup failure.

`MULTISUBS_MONITOR_ACCOUNTS_FILE` may point to an explicit monitor account file.

Timed provider checks that capture output keep at most 1,000,000 bytes and stop waiting on inherited output pipes after a 500 ms drain bound. Truncated output fails strict classification and is never shown or parsed as a complete provider response.

### `multisubs codex doctor [--json] [--timeout 8s]`

Runs only the focused Codex checks. It does not include Claude checks or create state.

A managed `auth.json` with group or world permissions fails its credential check. The dependent login-status probe is skipped for that profile, while unrelated profiles and checks continue. The permission finding does not print the credential path, permission bits, or contents.

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

Verification requires all of: logged-in status, `claude.ai` auth method, first-party API provider, a `max` subscription, and a non-empty organization ID. Any other subscription plan, including Pro, fails verification, so managed Claude profiles currently require Claude Max. The same rule decides which accounts `multisubs claude exec` may route to, and it makes a target's Claude doctor check warn. Claude usage reporting is deliberately exempt: it shows quota for any logged-in account.

Exact `multisubs claude login <name> --help` and `multisubs claude login <name> -h` requests instead run neutral official help as `claude auth login --claudeai <flag>`. The named profile need not exist. These forms do not load provider config, create product state, set `CLAUDE_CONFIG_DIR`, inspect auth or usage, or run post-login verification. Any help flag mixed with extra login arguments exits with code 2 before state access.

### `multisubs claude cli <name|default> [claude args...]`

Runs the official interactive Claude CLI. A managed target receives its derived `CLAUDE_CONFIG_DIR`. The `default` target receives no `CLAUDE_CONFIG_DIR`.

### `multisubs claude exec [claude -p args...]`

Runs official Claude print mode after fresh target-scoped auth and usage checks.

- The default account and usable managed profiles share one candidate list.
- Original Claude arguments are parsed once for `--model`/`-m`, `--fallback-model`, `--settings`, `--setting-sources`, session restoration, `--`, and the invocation directory. The same arguments are forwarded unchanged.
- Effective settings are resolved for each candidate. The default user source is `~/.claude/settings.json`, ignoring inherited `CLAUDE_CONFIG_DIR`; a managed candidate's user source is `<profile.ConfigDir>/settings.json`.
- With no `--setting-sources`, the standard user, project, and local sources apply. An explicit list selects only those named standard sources, and an empty value selects none. Managed settings and explicit inline or path-based `--settings` remain independent of that selection. Relative explicit paths use the invocation directory.
- Selected standard settings merge from user to project to local precedence, followed by explicit `--settings` and managed policy. Local macOS policy reads `/Library/Application Support/ClaudeCode/managed-settings.json` and then sorted `/Library/Application Support/ClaudeCode/managed-settings.d/*.json` fragments.
- Project and local settings use the repository or worktree root. Outside a repository they use the invocation directory. If the root cannot be established safely, only the relevant project and local fields become uncertain.
- Primary precedence is CLI model, effective candidate `ANTHROPIC_MODEL`, effective settings model, then account or organization default. Fallback precedence is CLI fallback, then the highest-precedence persistent `fallbackModel`, then no fallback. A CLI fallback fully replaces persistent fallback; a CLI primary does not.
- Model classification is candidate-specific. Fable IDs and values matching the effective Fable alias are applicable. `best`, unresolved `default`, custom values, malformed or cyclic aliases, and restored sessions without a conclusive override are possible. Sonnet, Opus, and Haiku aliases first use that candidate's alias mappings; recognized full non-Fable IDs are not applicable.
- Fallback chains use comma splitting, case-insensitive deduplication, and at most three entries. Any applicable or possible entry requires Fable.
- Locally unreadable managed or server-side model fields are represented as field-level uncertainty. They matter only when a higher-precedence CLI value does not settle that field. Full recognized non-Fable CLI primary and fallback values can together prove that Fable is not applicable.
- Candidates with applicable or possible Fable use require an available, unexhausted Fable window. Candidates rank by their worst applicable session, weekly all-model, or candidate-specific Fable percentage, then by name.
- Settings files must be regular and at most 2 MiB. Routing streams only `model`, `fallbackModel`, and the supported model environment fields. Malformed, duplicate, wrong-type, unreadable, oversized, or non-regular input makes only the affected fields and candidates uncertain; it does not reject the wrapper invocation.
- Duplicate organizations are removed before execution.
- A busy candidate is skipped while the next candidate is tried.
- If every eligible candidate is busy, the command returns the normal busy error. If none is usable, it returns one no-usable-account error.
- Default-account execution has no `CLAUDE_CONFIG_DIR`.

### `multisubs claude status`

Uses official `claude auth status --json` for the default account and each managed profile. Probe failures print a fixed safe reason instead of raw provider text. When a target is not logged in or cannot be checked, a `Next` section prints the exact command using the same closed allow-list as `multisubs status`. It accepts no extra arguments.

### `multisubs claude usage`

Prints the Claude-only view of the shared usage report. For each managed profile and the normal default account, one bounded context covers official `claude auth status --json`, the free non-persistent `/usage` probe, and a second official auth status. If the first official auth result reports that the account is not logged in, the row is `not logged in` and `/usage` does not run. If the second official auth result reports that the account is not logged in, the row is also `not logged in`, even when the usage probe text cannot be parsed. Usage identity requires logged-in status, a non-empty organization ID, and a strictly normalized email; it does not apply Max, provider, or auth-method routing restrictions. Grouping is allowed only when organization ID and normalized email are unchanged across both status results. Identity change or failure keeps valid quota as an ungrouped `identity unavailable` partial row. Targets with the same stable organization collapse into one logical row; different non-empty organization IDs never merge by email.

The labels are `Session (~5h)`, `Weekly all models`, and `Fable weekly`. Only an explicit bounded parenthesized duration in the session heading, such as `(5h)`, replaces `~5h`; reset countdown text never supplies the duration. Session and weekly all-model data are required provider sections. Missing optional Fable data is `not reported`, not an account failure. Reset text is printed only for supported `Resets in N ...`, weekday, month-and-day, or `Resets at ...` forms, with an optional safe IANA timezone. Parser, authentication, path, timeout, and binary failures affect only the relevant account and use fixed safe reasons. It accepts no extra arguments.

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
generate
reconcile
heartbeat
monitor
dry-run
```

Each exits with code 2 and points to the matching `multisubs codex ...` route.

## Legacy environment rejection

Startup checks the environment before path resolution. If any `MULTICODEX_*` variable is present, the command exits with code 2 and tells the user to clear it.

Runtime never reads the old environment namespace or the old `~/multicodex` state root. All old `MULTICODEX_*` variables remain on provider child-environment denylists to prevent account-routing leakage.

Monitor discovery also prunes `~/.multicodex` and the canonical alias targets of both legacy roots before descent.
