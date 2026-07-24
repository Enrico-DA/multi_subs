# multisubs

`multisubs` is a local-first Go command-line tool for isolated Codex and Claude subscription profiles. It keeps each managed account in its own provider directory, routes work by current provider usage, and leaves both normal default accounts outside product ownership.

This is a deliberate breaking rename for one local user. There is no old executable alias, state fallback, environment fallback, or compatibility command.

## Install

```bash
go install github.com/Enrico-DA/multi_subs/cmd/multisubs@latest
```

For a source checkout:

```bash
go build -o multisubs ./cmd/multisubs
```

Development builds report `multisubs 0.1.0-dev`. Release builds report the release tag through the same `multisubs version` command.

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
multisubs codex cli personal
multisubs codex exec -s read-only "Summarize this repository."
```

Add and use a Claude profile:

```bash
multisubs claude add personal
multisubs claude login personal
multisubs claude status
multisubs claude usage
multisubs claude exec "Review this change."
```

Show one quota snapshot across both providers, or filter the same report by provider:

```bash
multisubs usage
multisubs codex usage
multisubs claude usage
```

## Command tree

```text
multisubs init
multisubs doctor
multisubs usage
multisubs completion <shell>
multisubs version
multisubs help [topic]

multisubs codex init
multisubs codex add <name>
multisubs codex login <name> [...]
multisubs codex login-all
multisubs codex cli <name> [...]
multisubs codex exec [...]
multisubs codex status
multisubs codex usage
multisubs codex reconcile
multisubs codex heartbeat
multisubs codex monitor [...]
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

`multisubs init` and `multisubs codex init` call the same shared initialization path. `multisubs doctor` is the aggregate read-only check. It prints shared/base, Codex, and Claude sections. The two provider doctors stay focused on their own provider.

The three usage commands share one report format. The combined command prints Codex first, then Claude. Within each provider it prints managed profiles in name order and the normal default account last. Codex rows are `Session`, `Weekly`, and stable model-specific weekly limits such as `Spark weekly`. Claude rows are `Session (~5h)`, `Weekly all models`, and `Fable weekly`; a concrete provider session duration replaces the approximate label, and an absent optional Fable window is `not reported`. Percentages always mean used quota, and accounts are never averaged together.

Usage snapshots are read-only. They do not create product or provider state, include monitor-only account files, inspect active-home overrides, discover filesystem accounts, or estimate observed tokens. A partial account failure still prints every successful account and exits with code 1; invalid arguments, including `--json`, exit with code 2. JSON output is not available in this release.

Use `multisubs usage` for a quick point-in-time view. Use `multisubs codex monitor` for the live Codex terminal interface. The snapshot adds Codex session display but does not change the monitor or weekly-only Codex routing.

The Codex monitor also accepts the nested topics `tui`, `doctor`, `completion`, and `help`. The argument-free `multisubs codex monitor help` path is a leaf, so completion does not offer anything after it. Use `multisubs help codex monitor doctor` for details.

Bare Codex routes were removed. For example, `multisubs status` exits with code 2 and points to `multisubs codex status`.

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

Active product controls use the `MULTISUBS_*` namespace. This includes heartbeat settings, selected-profile metadata, and provider-routing markers. Provider children strip every inherited variable in that namespace, including unknown future controls. A managed Codex child then receives exactly one product variable: the selected `MULTISUBS_ACTIVE_PROFILE` marker added by multisubs. Default-account Codex, neutral provider help, and Claude children receive no `MULTISUBS_*` variable.

An explicit Codex monitor account file may be selected with `MULTISUBS_MONITOR_ACCOUNTS_FILE`.

Any legacy `MULTICODEX_*` variable causes startup to fail before state access. Clear it before running `multisubs`. Runtime never reads the old product home or old environment namespace. All legacy `MULTICODEX_*` controls are still removed from provider child environments as a denylist.

Filesystem monitor discovery prunes both `~/multicodex` and `~/.multicodex`, including canonical targets reached through aliases.

This phase does not move any live state or installed binary. Move or replace local state only in a separate, explicit migration step.

For the current user, valid default-config symlinks and valid single-link manual overrides keep working. The old exact multisubs-generated regular config may be replaced with the default-config symlink during normal managed setup. Hard-linked configs, arbitrary config symlinks, broken links, and non-regular entries now require a manual fix before that profile can be used; multisubs does not repair them.

## Provider behavior

Codex:

- Each managed profile receives its own `CODEX_HOME`, including auth, sessions, threads, `/goal`, and related Codex state.
- Exact target-scoped CLI help runs official Codex help with a neutral environment, does not require the named profile to exist, and does not create or reconcile product state.
- Exact target-scoped login help runs `codex login --help|-h` with the same state-free neutral boundary and without post-login checks.
- Managed execution enforces file-backed Codex auth.
- Automatic `exec` routing applies the same weekly, model, and reset policy to the default account and managed profiles.
- The usage snapshot preserves a declared five-hour session window, or one other unambiguous declared non-weekly duration, alongside weekly and model-specific weekly limits. It never guesses a session window from response position.
- `exec` resolves the effective model from `--model`/`-m`, exact root `model` config overrides, or one common root model across every candidate config. Conflicting candidate models fail with code 2. A Codex `--profile`/`-p` selector requires an explicit model.
- The default account is skipped when its usage is unavailable, exhausted, or missing a required model bucket. Its execution remains unmanaged and receives no managed auth override.
- `heartbeat` uses an ephemeral, read-only Codex request and a private lock under `MULTISUBS_HOME`.
- Resource reconciliation does not overwrite regular user files. It changes only documented product-owned links.

Claude:

- Each managed profile receives a derived `CLAUDE_CONFIG_DIR`.
- Login, status, usage, and routing use the official Claude CLI.
- Claude usage collection is shared by the combined and provider-only snapshots and uses the official non-persistent `/usage` probe.
- Exact target-scoped login help runs `claude auth login --claudeai --help|-h` without profile state, `CLAUDE_CONFIG_DIR`, probes, or post-login checks.
- `exec` parses the original model, fallback, settings-source, explicit-settings, and session-restoration intent once without changing the arguments passed to Claude. It then resolves effective settings separately for each candidate. The default account uses `~/.claude/settings.json`; a managed account uses its profile-local `settings.json`.
- Candidate settings also include selected project and local files, explicit inline or path-based `--settings`, and local macOS managed settings. Managed and server-side values that cannot be proved from local files remain unknown.
- Fable applicability has three outcomes: not applicable, applicable, or possible. Applicable and possible candidates both require an available Fable window and include that window in their score. A settings read or classification failure affects only that candidate.
- Settings inspection reads only model, fallback, and the five model environment fields used by routing. Each file must be regular and no larger than 2 MiB; paths, contents, values, and unrelated settings are not reported.
- An unusable or busy candidate is skipped. The default account runs without `CLAUDE_CONFIG_DIR`.
- The product does not read, copy, or write Claude credential contents.

## Doctor and dry run

```bash
multisubs doctor
multisubs codex doctor
multisubs claude doctor
multisubs codex dry-run
multisubs codex dry-run login personal
```

All doctor commands, usage reports, and dry-run startup are non-mutating. Aggregate doctor output always includes shared/base, Codex, and Claude sections after successful argument parsing, even when the Codex profile registry is invalid. Help, version, completion, invalid commands, and dynamic profile completion also avoid state creation.

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
- Provider child environments remove credential overrides, every inherited `MULTISUBS_*` variable, and all legacy `MULTICODEX_*` controls. Multisubs adds only `MULTISUBS_ACTIVE_PROFILE` to managed Codex children for the selected profile; default-account Codex, neutral provider help, and Claude children receive no `MULTISUBS_*` variable.
- Output avoids raw credentials and raw provider failure text.
- Current and legacy-sensitive state patterns remain ignored to prevent accidental credential commits.

See [the command contract](docs/command-spec.md), [the security contract](docs/security-and-privacy.md), and [the upstream translation map](docs/upstream-sync.md).

## Upstream

This fork is based on [olliecrow/multicodex](https://github.com/olliecrow/multicodex) and preserves its attribution and license.
