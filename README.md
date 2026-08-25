# multisubs

`multisubs` is a local-first Go command-line tool for isolated Codex and Claude subscription profiles. It keeps each managed account in its own provider directory, routes work by current provider usage, and leaves both normal default accounts outside product ownership. Managed Claude profiles require Claude Max; managed Codex profiles have no plan requirement.

This is a deliberate breaking rename for one local user. There is no old executable alias, state fallback, environment fallback, or compatibility command.

## Install

This repository is private. After a `multisubs` binary is on `PATH`, later upgrades are:

```bash
multisubs install
```

That command installs into the directory of the running binary, writes `GOPRIVATE` and `GOBIN` into the login shell profile, and deletes leftover regular `multisubs` copies at the default Go bin path (`$(go env GOPATH)/bin`, often `~/go/bin`). Doctor warns when those copies would still diverge; it never deletes them. `multisubs install` does.

First-time install, before that command exists:

```bash
export GOPRIVATE=github.com/Enrico-DA/multi_subs
export GOBIN="${GOBIN:-$HOME/.local/bin}"
mkdir -p "$GOBIN"
go install github.com/Enrico-DA/multi_subs/cmd/multisubs@latest
```

If this is the first install, add `$GOBIN` to `PATH`.

Install unmerged work by commit hash. Branch names that contain `/` are not valid `go install` versions. Use `multisubs install <commit>` when the running binary already has that command. Otherwise:

```bash
GOBIN="$(dirname "$(command -v multisubs)")" GOPRIVATE=github.com/Enrico-DA/multi_subs go install github.com/Enrico-DA/multi_subs/cmd/multisubs@<commit>
```

For a source checkout:

```bash
go build -o multisubs ./cmd/multisubs
```

`multisubs version` prints the release tag when the binary was built with `-ldflags`. A `go install` from a commit prints that module version. A local source build prints the short Git revision when VCS info is present. The compile-time `0.1.0-dev` default appears only when nothing more specific is available. Status, usage, and the first doctor check all print that same version. Doctor also prints the resolved path of the binary that is running.

After install, confirm the binary on that machine:

```bash
command -v multisubs
go version -m "$(command -v multisubs)"
multisubs version
multisubs doctor
```

`multisubs codex generate` requires Codex CLI 0.147.0 or 0.148.0. `codex --version` must print one of those versions.

## Start

Initialize shared product and Codex profile state:

```bash
multisubs init
```

Add and log in to Codex profiles:

```bash
multisubs codex add personal
multisubs codex add work
multisubs codex login personal
multisubs codex login work
multisubs codex status
```

Run Codex with one named profile or automatic weekly routing:

```bash
multisubs codex cli
multisubs codex cli personal
multisubs codex exec -s read-only "Summarize this repository."
multisubs codex exec --search "What changed this week?"
multisubs codex generate "Name three risks in this change."
multisubs codex generate --search "What changed this week?"
```

Add and use a Claude profile. Managed Claude profiles currently require a Claude Max subscription with first-party `claude.ai` login; other plans, including Pro, fail login verification and are never routed to. Claude usage reporting still works for any logged-in managed profile. It also keeps live quota visible for the default account, but does not trust that account's cached identity.

```bash
multisubs claude add personal
multisubs claude login personal
multisubs claude status
multisubs claude usage
multisubs claude exec "Review this change."
```

Show one quota snapshot across both providers, or filter the same report by provider. When an account is not ready, the snapshot prints a Next section with the exact command to run:

```bash
multisubs status
multisubs usage
multisubs codex usage
multisubs claude usage
```

## Command tree

```text
multisubs init
multisubs install [ref]
multisubs doctor
multisubs status
multisubs usage
multisubs completion <shell>
multisubs version
multisubs help [topic]

multisubs codex init
multisubs codex add <name>
multisubs codex login <name> [...]
multisubs codex login-all
multisubs codex cli [<name>] [...]
multisubs codex exec [--search] [...]
multisubs codex generate [--search] [...]
multisubs codex status
multisubs codex usage
multisubs codex reconcile
multisubs codex monitor doctor [...]
multisubs codex doctor [...]
multisubs codex dry-run [...]

multisubs claude add <name>
multisubs claude login <name> [...]
multisubs claude cli <name|default> [...]
multisubs claude exec [...]
multisubs claude status
multisubs claude usage
multisubs claude doctor
```

`multisubs init` and `multisubs codex init` call the same shared initialization path. `multisubs install [ref]` replaces the running binary, persists `GOBIN` in the login shell profile, and removes leftover Go-bin copies. `multisubs doctor` is the aggregate read-only check. It prints shared/base, Codex, and Claude sections. The first shared/base check is the same version string as `multisubs version`, plus the resolved path of the running binary. The next check warns when `go install` would write a second copy, or when a leftover binary remains at the default Go bin path, and points at `multisubs install`. Doctor never deletes those files. The two provider doctors stay focused on their own provider.

The three usage commands share one report format. This local-only output includes each identified logical subscription's full, validated account email by default. The combined command prints Codex first, then Claude. Duplicate physical targets for one subscription collapse into one row and one availability count. Managed aliases sort by name. If a Codex logical row also contains the normal default account, it ends with `(also default)` and stays last for that provider. The default Claude account never joins a duplicate group. Email-shaped profile names use unique aliases such as `[managed-1]`. Codex rows are `Session`, `Weekly`, and fixed product-known model limits; the only current extra label is `Spark weekly`, and unknown provider limit names are suppressed. Claude rows are `Session (~5h)`, `Weekly all models`, and `Fable weekly`; only an explicit parenthesized duration in the provider's session heading replaces the approximate label, and an absent optional Fable window is `not reported`. One deterministic successful quota snapshot represents a managed duplicate group; percentages are never averaged.

Usage snapshots do not create multisubs product state, change provider credentials, or persist identity in config. They do not include monitor-only account files, inspect active-home overrides, or discover filesystem accounts. When the default Codex account has no usable `auth.json`, its official app-server probe may write non-credential state in the default Codex home, including logs, caches, database files, and database write-ahead files. The probe does not change credentials. Account, user, and organization IDs stay internal and are never printed. If a validated email or unambiguous logical identity is unavailable while quota succeeds, the quota remains visible as a separate `identity unavailable` row, contributes one unavailable count because no safe collapse is proven, makes the report partial, and causes exit code 1. Successful default Claude quota always has this outcome: cached email and organization cannot prove which live credential context supplied `/usage`, so the quota stays visible in a separate row and is never grouped with a managed profile. Its recovery command is `multisubs doctor`. A Codex session or Spark-only extra limit may also be shown as partial when required standard weekly data is unavailable. Source cleanup failure is a fixed safe account failure. Invalid arguments, including `--json`, exit with code 2. JSON output is not available in this release. When the result is not complete, a Next section prints each distinct exact recovery command once; the first failed account that needs a command supplies that row's label and reason. Default Codex login is `codex login`. A logged-out default Claude account uses `claude auth login`. Managed profiles use `multisubs codex login <name>` or `multisubs claude login <name>`. Other failures use `multisubs doctor` or `multisubs init`. Those commands are a closed allow-list; profile names are validated before they are printed.

Use `multisubs status` or `multisubs usage` for a point-in-time view. `multisubs usage` is the Codex usage snapshot; there is no live terminal interface. `multisubs codex monitor doctor` checks the monitor's setup and sources.

The Codex monitor accepts the nested topics `doctor`, `completion`, and `help`. Bare `multisubs codex monitor` prints its usage. The argument-free `multisubs codex monitor help` path is a leaf, so completion does not offer anything after it. Use `multisubs help codex monitor doctor` for details.

Bare Codex routes were removed. Product-wide `multisubs status` prints the quota snapshot and next commands. `multisubs login` and other old top-level Codex routes still exit with code 2 and point to `multisubs codex ...`.

The profile name `default` is reserved for each provider's built-in default account and cannot be used for a managed profile.

## State and environment

The product state root is `~/multisubs`. Set `MULTISUBS_HOME` to use another location. Set `MULTISUBS_DEFAULT_CODEX_HOME` only when the default Codex home is not `~/.codex`.

Codex state:

- Shared registry: `~/multisubs/config.json`
- Managed profile: `~/multisubs/profiles/<name>/codex-home`
- Official provider variable: `CODEX_HOME`
- Selected-profile metadata: `~/multisubs/run`

Each managed profile's `config.toml` has exactly two allowed forms:

- a regular, non-symlink file whose hard-link count can be verified as exactly one; or
- a symlink whose fully resolved path is exactly the fully resolved default Codex `config.toml`, with a regular file as its target.

Managed setup, execution readiness, status, doctor, model inspection, and monitoring all use one filesystem validator before reading TOML. A hard-linked config is rejected and is never repaired automatically. The default Codex account and its config remain unmanaged.

Claude state:

- Provider registry: `~/multisubs/providers/claude/config.json`
- Managed profile: `~/multisubs/providers/claude/profiles/<name>/config`
- Official provider variable: `CLAUDE_CONFIG_DIR`

Active product controls use the `MULTISUBS_*` namespace. This includes selected-profile metadata and provider-routing markers. Provider children strip every inherited variable in that namespace, including unknown future controls. A managed Codex child then receives exactly one product variable: the selected `MULTISUBS_ACTIVE_PROFILE` marker added by multisubs. Default-account Codex, neutral provider help, and Claude children receive no `MULTISUBS_*` variable.

An explicit Codex monitor account file may be selected with `MULTISUBS_MONITOR_ACCOUNTS_FILE`.

Any legacy `MULTICODEX_*` variable causes startup to fail before state access. Clear it before running `multisubs`. Runtime never reads the old product home or old environment namespace. All legacy `MULTICODEX_*` controls are still removed from provider child environments as a denylist.

Filesystem monitor discovery prunes both `~/multicodex` and `~/.multicodex`, including canonical targets reached through aliases.

It excludes candidates canonically inside `MULTISUBS_HOME`; registered managed profiles still come from `config.json`.

This phase does not move any live state or installed binary. Move or replace local state only in a separate, explicit migration step.

For the current user, valid default-config symlinks and valid single-link manual overrides keep working. The old exact multisubs-generated regular config may be replaced with the default-config symlink during normal managed setup. Hard-linked configs, arbitrary config symlinks, broken links, and non-regular entries now require a manual fix before that profile can be used; multisubs does not repair them.

## Provider behavior

Codex:

- Each managed profile receives its own `CODEX_HOME`, including auth, sessions, threads, `/goal`, and related Codex state.
- Exact target-scoped CLI help runs official Codex help with a neutral environment, does not require the named profile to exist, and does not create or reconcile product state.
- Exact target-scoped login help runs `codex login --help|-h` with the same state-free neutral boundary and without post-login checks.
- Managed execution enforces file-backed Codex auth.
- Automatic `exec` routing reconciles duplicate Codex homes into logical subscriptions before quota scoring. All successful snapshots in one group must agree on the requested bucket, availability, exhaustion, used percentage, and reset meaning. Exact reset timestamps must match exactly; relative-only countdowns allow at most five seconds of concurrent-fetch drift. Disagreement fails that logical group closed; only then is one physical home chosen. Missing or conflicting fallback identity cannot add routing capacity.
- The usage snapshot preserves a declared five-hour session window, or one other unambiguous declared non-weekly duration, alongside weekly and model-specific weekly limits. A session-only primary response still triggers the weekly fallback; the snapshot merges that session with safe fallback weekly fields. It never guesses a session window from response position.
- `exec` resolves the effective model from `--model`/`-m`, exact root `model` config overrides, or one common root model across every candidate config. Conflicting candidate models fail with code 2. A Codex `--profile`/`-p` selector requires an explicit model.
- Routing and the usage snapshot use the same source rule for the default account. A private regular `auth.json` with `tokens.access_token` uses OAuth first. If that snapshot has a standard weekly number, no app-server process starts. If OAuth has no standard weekly (Spark-only is not enough), one unmanaged official app-server probe runs against the same default home. If that file also has a safe account id, the OAuth request includes `ChatGPT-Account-Id` and never prints it. The id may come from `tokens.account_id`, top-level `account_id`, or sanitized token claims. If the auth file is not usable, or OAuth has no standard weekly, multisubs starts the official app server against the default home in unmanaged mode. This fallback uses a sanitized environment, adds no managed file-store override, and does not read or fingerprint credentials in the app-server source. It still requires real weekly data; unavailable, exhausted, or model-ineligible usage is skipped as before. Status keeps a Spark-only extra limit visible and marks that row partial when standard weekly is still missing. The official process may write non-credential logs, caches, database files, and database write-ahead files in the default home, but it does not change credentials.
- Explicit OAuth eligibility is authoritative. `allowed: false` makes a weekly bucket unavailable, and `limit_reached: true` makes it exhausted. Omitted fields keep the narrow older-response compatibility behavior.
- If scoring selects the default account, exec asks the official Codex CLI to confirm its login. This check honors the CLI's configured credential store and does not treat a missing `auth.json` as proof of logout. Multisubs makes two bounded attempts, with a short pause between them. If neither confirms login and another account is available, it prints a prominent stderr warning that names the default account and says `Run: codex login`, excludes the default for that command, and selects once more from the remaining accounts. A redirect always has that visible warning. If no other account can run the job, exec exits with code 1 and one blocked message with the same cause and fix. Default-account execution remains unmanaged and receives no managed auth override.
- Resource reconciliation does not overwrite regular user files. It changes only documented product-owned links.

Claude:

- Each managed profile receives a derived `CLAUDE_CONFIG_DIR`.
- Login, status, usage, and managed-profile routing use the official Claude CLI.
- Claude usage collection is shared by the combined and provider-only snapshots. For a managed profile, one per-target deadline covers official `claude auth status --json` before and after the official non-persistent `/usage` probe. A first official logged-out result skips `/usage` and is `not logged in`. Managed organization grouping is allowed only when the logged-in organization and normalized email are unchanged. This managed usage identity check does not require Max routing eligibility.
- The default Claude target is different. Its usage collector runs only the official non-persistent `/usage` probe and never queries cached identity. Cached `auth status` email and organization can diverge from the live credential store used by `/usage` and `-p`, so multisubs never prints or uses those fields. Successful default quota is shown as `identity unavailable`, remains ungrouped, makes usage and product-wide status partial, exits with code 1, and uses `multisubs doctor` as its recovery command.
- `multisubs claude status` can still show whether the default credential is present, but a logged-in default shows `identity unavailable` and no cached auth details. It adds no Next step while logged in. Claude doctor warns that the default identity cannot be verified, does not use it for duplicate checks, and still passes when no check fails.
- Managed login rejects duplicate organizations only among other managed profiles. The default account is outside that comparison because automatic Claude routing never uses it.
- Exact target-scoped login help runs `claude auth login --claudeai --help|-h` without profile state, `CLAUDE_CONFIG_DIR`, probes, or post-login checks.
- `exec` parses the original model, fallback, settings-source, explicit-settings, and session-restoration intent once without changing the arguments passed to Claude. It then resolves effective settings separately for each managed candidate from that profile's local `settings.json`.
- Candidate settings also include selected project and local files, explicit inline or path-based `--settings`, and local macOS managed settings. Managed and server-side values that cannot be proved from local files remain unknown.
- Fable applicability has three outcomes: not applicable, applicable, or possible. Applicable and possible candidates both require an available Fable window and include that window in their score. A settings read or classification failure affects only that candidate.
- Settings inspection reads only model, fallback, and the five model environment fields used by routing. Each file must be regular and no larger than 2 MiB; paths, contents, values, and unrelated settings are not reported.
- An unusable or busy managed candidate is skipped. Automatic `multisubs claude exec` never probes or selects the default account. The default remains explicitly runnable with `multisubs claude cli default` and receives no `CLAUDE_CONFIG_DIR`.
- The product does not read, copy, or write Claude credential contents.

## Doctor and dry run

```bash
multisubs doctor
multisubs codex doctor
multisubs claude doctor
multisubs codex dry-run
multisubs codex dry-run login personal
```

Doctor commands and dry-run startup create no multisubs product state and do not change credentials. A requested monitor-doctor app-server probe and the default Codex usage fallback may start the official process against the default home. That process can write non-credential operational state there. Aggregate doctor output always includes shared/base, Codex, and Claude sections after successful argument parsing, even when the Codex profile registry is invalid. Any failed aggregate, focused, or monitor-doctor check prints a `FAIL` summary and exits with code 1. A managed Codex credential file with group or world permissions fails its check and skips that profile's login-status probe without stopping unrelated checks. Help, version, completion, invalid commands, and dynamic profile completion also avoid state creation.

## Completion

```bash
eval "$(multisubs completion zsh)"
eval "$(multisubs completion bash)"
multisubs completion fish > ~/.config/fish/completions/multisubs.fish
```

Completion covers both provider namespaces, Codex monitor topics, all help topics, and dynamic Codex and Claude profile names.

## Security

- Never copy, sync, transmit, or share provider auth files between machines.
- Use the official provider login commands for every managed profile.
- State directories must be private regular directories. Sensitive files, locks, and routing metadata reject unsafe links.
- Codex and Claude registry locks time out after five seconds rather than proceeding unlocked. Timed provider probes cap captured output at 1,000,000 bytes, bound inherited-pipe draining after cancellation, and reject truncated output as incomplete.
- Provider child environments remove credential overrides, every inherited `MULTISUBS_*` variable, and all legacy `MULTICODEX_*` controls. Multisubs adds only `MULTISUBS_ACTIVE_PROFILE` to managed Codex children for the selected profile; default-account Codex, neutral provider help, and Claude children receive no `MULTISUBS_*` variable.
- Output avoids raw credentials, raw provider failure text, and caller-supplied arguments in wrapper failure messages.
- Current and legacy-sensitive state patterns remain ignored to prevent accidental credential commits.

See [the command contract](docs/command-spec.md), [the security contract](docs/security-and-privacy.md), and [the upstream translation map](docs/upstream-sync.md).

## Upstream

This fork is based on [olliecrow/multicodex](https://github.com/olliecrow/multicodex) and preserves its attribution and license.
