# multicodex

`multicodex` helps you use multiple Codex and Claude subscription accounts on one machine without changing either normal default account.

Bare commands continue to manage Codex. The `multicodex claude` namespace manages Claude profiles and routes headless Claude workers. Each provider keeps its own profile registry and isolated runtime directories. The regular system accounts remain managed by the official CLIs outside multicodex.

By default, each profile reuses your global Codex `config.toml` through a symlink, so model defaults, reasoning settings, permission settings, and other normal Codex config changes apply everywhere. Profile homes also inherit missing top-level skill entries from the global Codex skills directory through symlinks.

Profile login requires file-backed auth. If the effective Codex config does not set `cli_auth_credentials_store = "file"`, profile login and profile-scoped Codex execution fail with a setup error instead of sharing global auth state.

## Status

- Usable for local multi-account Codex CLI, `codex exec`, heartbeat, and usage-monitor workflows.
- Usable for local multi-account Claude CLI, quota-aware `claude -p`, and Fable routing.
- The command surface is intentionally narrow. Multicodex does not implement global account switching.

## Prerequisites

- Go 1.25 or newer for building from source.
- Development and CI checks use the patched Go toolchain listed in `go.mod`.
- Official `codex` CLI installed and available in `PATH`.
- Official `claude` CLI installed and available in `PATH` for Claude commands.
- macOS or Linux.

## Install

Build from source.

```bash
go build -o multicodex ./cmd/multicodex
```

Or install from the public module path.

```bash
go install github.com/Enrico-DA/multicodex/cmd/multicodex@latest
```

Optional local install path.

```bash
mv ./multicodex ~/.local/bin/multicodex
```

## Quick Start

```bash
multicodex init
multicodex add personal
multicodex add work
multicodex login personal
multicodex login work
multicodex status
```

Run interactive Codex with one profile.

```bash
multicodex cli personal
multicodex cli work "check this repo"
```

Run `codex exec` on the best available account.

```bash
multicodex exec -s read-only "Summarize the README in 3 bullets."
```

Add a Claude Max account and run Fable on the best available Claude profile.

```bash
multicodex claude add personal
multicodex claude login personal
multicodex claude status
multicodex claude usage
multicodex claude exec --model claude-fable-5 "Review this change."
```

Claude login uses the official browser flow. Multicodex never reads, copies, or writes Claude credentials.

Open the monitor and run checks.

```bash
multicodex monitor
multicodex doctor
multicodex monitor doctor
multicodex dry-run
```

## Local State

- Default multicodex state home is `~/multicodex`.
- Override the state location with `MULTICODEX_HOME`.
- Profile auth stays under `~/multicodex/profiles/<name>/codex-home/auth.json`.
- Profile sessions, threads, and `/goal` state stay under that profile's `codex-home`.
- Multicodex state directories, profile directories, profile `codex-home`, profile skills directories, `auth.json`, selected-profile metadata under `MULTICODEX_HOME/run`, heartbeat lock files, and config lock files must be profile-local regular filesystem entries with local-user-only directory permissions. Symlinks and hard links are rejected where they could cross account boundaries.
- Profile `config.toml` defaults to a symlink from `~/multicodex/profiles/<name>/codex-home/config.toml` to the default Codex config at `~/.codex/config.toml`.
- Profile skills fill in missing top-level entries from `~/.codex/skills` using symlinks. Manual top-level profile skill overrides are left in place.
- To use a per-profile Codex config, replace the profile `config.toml` symlink with a regular profile-local `config.toml` file that still enables file-backed auth.
- Claude metadata is separate at `~/multicodex/providers/claude/config.json`.
- Managed Claude state lives under `~/multicodex/providers/claude/profiles/<name>/config`.
- The default Claude account is a protected reserve. It is launched with `CLAUDE_CONFIG_DIR` absent; managed accounts receive exactly one profile-local `CLAUDE_CONFIG_DIR`.

## Commands

```text
multicodex init
multicodex add <name>
multicodex login <name> [codex login args]
multicodex login-all
multicodex cli <name> [codex args...]
multicodex exec [codex exec args]
multicodex status
multicodex heartbeat
multicodex monitor [flags]
multicodex monitor tui [flags]
multicodex monitor doctor [flags]
multicodex monitor completion [shell]
multicodex doctor [--json] [--timeout 8s]
multicodex dry-run [operation]
multicodex completion <bash|zsh|fish>
multicodex version
multicodex help [command [subcommand]]
multicodex --version
multicodex claude add <name>
multicodex claude login <name> [claude auth login args]
multicodex claude cli <name|default> [claude args...]
multicodex claude exec [claude -p args...]
multicodex claude status
multicodex claude usage
multicodex claude doctor
```

## Interactive CLI

`multicodex cli <name> [codex args...]` launches the official `codex` CLI with that profile's `CODEX_HOME`.

Codex defaults such as model, reasoning level, approvals, sandbox, and search come from the shared Codex config unless you pass explicit Codex args. Multicodex does not inject its own model or permission defaults.

Two terminals can run `multicodex cli` with different profiles at the same time. Each terminal uses its own account, auth, threads, and `/goal` state because each one has a different `CODEX_HOME`.

## Exec Routing

`multicodex exec [codex exec args]` runs `codex exec` after selecting among configured multicodex profiles, with the default Codex home as a built-in reserve account.

- Help requests such as `multicodex exec --help` delegate directly to `codex exec` and do not require profiles.
- Exec can run with no configured profiles by using the default Codex home as the only available account.
- Configured profiles at 100% weekly usage are not selected.
- Exec uses configured selection priority first, then prefers the profile whose known weekly reset is soonest.
- Profiles with an unknown weekly reset follow profiles with a known reset. Exact ties are randomized.
- The default Codex home is a protected reserve. It is used only when no configured profile has usable weekly usage.
- If the default Codex home is the only remaining destination, exec uses it as the final fallback even when its usage data is unavailable or exhausted.
- For explicit Spark model names, configured profiles need Spark usage windows to win normal routing; the default Codex home still remains the final fallback.

## Claude Routing

`multicodex claude exec [claude -p args...]` asks each managed Claude profile for fresh plan usage through the official, free `/usage` command.

- Session and all-model weekly usage must both be below 100%.
- Explicit Fable requests also require an available Fable weekly window.
- When the effective model is omitted or unknown, routing conservatively requires Fable capacity. A Fable or unknown fallback also requires Fable capacity.
- Eligible managed profiles are ordered by their highest applicable usage percentage, then name.
- Only first-party Claude Max logins with a stable organization ID are routable. Profiles that resolve to the same organization are deduplicated, including duplicates of the default reserve.
- A non-blocking organization lock reserves the chosen account until the child exits. The child inherits the lock descriptor, so the reservation survives wrapper death.
- If eligible managed profiles are only busy, the command returns a busy error instead of spending the default reserve.
- The default Claude account is used only when no managed profile has usable quota.
- Arguments are passed to official `claude -p` unchanged. Multicodex does not inject a model.
- A managed auth or usage failure excludes that profile. Unsafe local state is a fatal error.
- Usage probes disable session persistence, user/project settings, and MCP servers and run from a neutral directory.

## Heartbeat

`multicodex heartbeat` sends a minimal ephemeral, read-only keepalive hello to every logged-in profile. Heartbeat requests do not persist Codex session files.

```bash
multicodex heartbeat
```

Heartbeat:

- skips logged-out profiles
- uses a non-blocking lock under `MULTICODEX_HOME`
- retries failed profile heartbeats once by default
- runs profile-scoped `codex exec --skip-git-repo-check --ephemeral --sandbox read-only --color never hello`
- redacts raw failure output

Optional environment overrides:

- `MULTICODEX_HEARTBEAT_TIMEOUT_SECONDS`
- `MULTICODEX_HEARTBEAT_RETRIES`
- `MULTICODEX_HEARTBEAT_BACKOFF_SECONDS`
- `MULTICODEX_HEARTBEAT_LOCK_PATH`

`MULTICODEX_HEARTBEAT_LOCK_PATH` must resolve under `MULTICODEX_HOME`.

## Monitor

`multicodex monitor` shows live subscription usage across configured profile homes.

```bash
multicodex monitor
multicodex monitor tui
multicodex monitor --interval 30s
multicodex monitor doctor
multicodex monitor completion
```

By default, monitor account candidates come only from:

- explicit account file under `~/multicodex/monitor/accounts.json`
- configured multicodex profiles from `~/multicodex/config.json`

Additional sources are opt-in:

- `--include-default` includes the default Codex home
- `--include-active` includes the active `CODEX_HOME`
- `--discover` scans compatible Codex homes from the local filesystem
- `multicodex monitor doctor --app-server` also checks the raw Codex app-server source separately

For validated multicodex profile homes, the monitor asks the Codex app-server for usage first and falls back to direct OAuth from the profile home. This matches Codex CLI auth handling for logged-in profiles whose access token can still be refreshed. Other monitor account homes use direct OAuth unless they dedupe with a validated profile home.

The TUI:

- orders account rows by weekly reset time
- shows configured account labels before raw identity fields
- keeps timestamps in UTC internally and renders local time in the UI
- shows one full-width weekly card per account, with default and Spark usage on separate lines when available
- shows a compact progress bar where space permits, plus the reset countdown and exact local reset time
- shows one local seven-day observed-token estimate from session logs
- uses each official window's declared duration, with a narrow positional fallback for older provider responses
- keeps last good official window cards visible and marked stale during a full refresh outage
- prefers short re-login warnings when a profile token has expired

Example manual monitor account file:

```json
{
  "version": 1,
  "accounts": [
    {"label": "personal", "codex_home": "/path/to/personal/codex-home"},
    {"label": "work", "codex_home": "/path/to/work/codex-home"}
  ]
}
```

## Checks And Completion

Run non-mutating checks and previews.

```bash
multicodex doctor
multicodex dry-run
multicodex dry-run login personal
```

Enable tab completion.

```bash
eval "$(multicodex completion zsh)"
eval "$(multicodex completion bash)"
multicodex completion fish > ~/.config/fish/completions/multicodex.fish
```

Get detailed help.

```bash
multicodex help
multicodex help cli
multicodex help exec
multicodex help heartbeat
multicodex help monitor
multicodex help monitor doctor
```

## Development Checks

```bash
go test ./...
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.4.0 ./...
go build -o multicodex ./cmd/multicodex
```

## Safety Model

- Uses official `codex login` flows.
- Uses official `claude auth login --claudeai`, `claude auth status --json`, and `claude -p "/usage"` flows.
- Keeps profile auth and Codex state local to each profile `CODEX_HOME`.
- Keeps Claude state isolated through `CLAUDE_CONFIG_DIR` and never reads Claude credential contents.
- Does not store raw secrets in multicodex config.
- Does not change, restore, back up, symlink, or otherwise manage either shared default auth account.
- Scrubs inherited account-routing and account-token environment variables before launching profile-scoped Codex commands.
- Scrubs inherited Anthropic/Claude account overrides before launching Claude commands.
- `monitor` is read-only and does not mutate Codex account data.
- `doctor` and `dry-run` are non-mutating helpers.
- `doctor` includes repo leak guards for tracked sensitive files and ignore-pattern coverage.
- After successful login, regular auth file permissions are normalized to `0600`.

## Notes

- If your default Codex setup uses keychain auth only, configure file-backed auth for the profiles you want to use with multicodex.
- Do not copy, sync, transmit, transfer, or share Codex auth files between machines. Sign in locally with the official Codex login flow.

## License

Apache License 2.0. See `LICENSE`.

<!-- third-party-policy:start -->
## Third-Party Code Policy
This repository allows external-code snapshots for static analysis only. External clones must stay in ephemeral `plan/` locations, be sanitized immediately (`rm -rf .git`, or remove all remotes first if `.git` is temporarily retained), and must never be executed.

See `docs/untrusted-third-party-repos.md`.
<!-- third-party-policy:end -->
