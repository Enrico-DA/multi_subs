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

## Command tree

```text
multisubs init
multisubs doctor
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

The Codex monitor also accepts the nested topics `tui`, `doctor`, `completion`, and `help`. Use `multisubs help codex monitor doctor` for details.

Bare Codex routes were removed. For example, `multisubs status` exits with code 2 and points to `multisubs codex status`.

The profile name `default` is reserved for each provider's built-in default account and cannot be used for a managed profile.

## State and environment

The product state root is `~/multisubs`. Set `MULTISUBS_HOME` to use another location. Set `MULTISUBS_DEFAULT_CODEX_HOME` only when the default Codex home is not `~/.codex`.

Codex state:

- Shared registry: `~/multisubs/config.json`
- Managed profile: `~/multisubs/profiles/<name>/codex-home`
- Official provider variable: `CODEX_HOME`
- Selected-profile metadata: `~/multisubs/run`

Claude state:

- Provider registry: `~/multisubs/providers/claude/config.json`
- Managed profile: `~/multisubs/providers/claude/profiles/<name>/config`
- Official provider variable: `CLAUDE_CONFIG_DIR`

Active product controls use the `MULTISUBS_*` namespace. This includes heartbeat settings, selected-profile metadata, and provider-routing markers.

An explicit Codex monitor account file may be selected with `MULTISUBS_MONITOR_ACCOUNTS_FILE`.

Any legacy `MULTICODEX_*` variable causes startup to fail before state access. Clear it before running `multisubs`. Runtime never reads the old product home or old environment namespace. Old variables are still removed from provider child environments as a denylist.

This phase does not move any live state or installed binary. Move or replace local state only in a separate, explicit migration step.

## Provider behavior

Codex:

- Each managed profile receives its own `CODEX_HOME`, including auth, sessions, threads, `/goal`, and related Codex state.
- Managed execution enforces file-backed Codex auth.
- Automatic `exec` routing applies the same weekly, model, and reset policy to the default account and managed profiles.
- The default account is skipped when its usage is unavailable, exhausted, or missing a required model bucket. Its execution remains unmanaged and receives no managed auth override.
- `heartbeat` uses an ephemeral, read-only Codex request and a private lock under `MULTISUBS_HOME`.
- Resource reconciliation does not overwrite regular user files. It changes only documented product-owned links.

Claude:

- Each managed profile receives a derived `CLAUDE_CONFIG_DIR`.
- Login, status, usage, and routing use the official Claude CLI.
- Routing scores the default account and managed profiles together using fresh session, weekly all-model, and Fable usage.
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

All doctor commands and dry-run startup are non-mutating. Help, version, completion, invalid commands, and dynamic profile completion also avoid state creation.

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
- Provider child environments remove credential overrides, active product controls, and legacy product controls.
- Output avoids raw credentials and raw provider failure text.
- Current and legacy-sensitive state patterns remain ignored to prevent accidental credential commits.

See [the command contract](docs/command-spec.md), [the security contract](docs/security-and-privacy.md), and [the upstream translation map](docs/upstream-sync.md).

## Upstream

This fork is based on [olliecrow/multicodex](https://github.com/olliecrow/multicodex) and preserves its attribution and license.
