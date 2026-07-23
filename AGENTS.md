# multicodex repository guidance

## Repository ownership

- This fork belongs under the personal GitHub account `Enrico-DA`.
- The upstream project is `olliecrow/multicodex`; preserve its attribution and license.
- Push fork work only to `Enrico-DA/multi_subs`. Never push to the upstream repository without Ollie's explicit approval.
- New repositories remain private unless Enrico explicitly approves publication.

## Purpose and layout

- `multicodex` is a local-first Go CLI for isolated Codex and Claude subscription profiles, provider-specific execution routing, heartbeat checks, and usage monitoring.
- `cmd/multicodex/` contains the entry point. Product code and tests live under `internal/`.
- `README.md` is the user guide. `docs/command-spec.md` is the command contract, `docs/security-and-privacy.md` is the security contract, and `docs/decisions.md` records durable cross-cutting rationale.

## Product invariants

- Keep each profile's auth, sessions, threads, `/goal`, and other Codex state inside its profile-local `CODEX_HOME`.
- Keep each managed Claude account inside its derived profile-local `CLAUDE_CONFIG_DIR`.
- Never change, copy, restore, back up, symlink, or otherwise manage either shared default auth account. Each default account is only a protected routing reserve; the default Codex home is also a read-only monitor source.
- Never print raw credentials or raw subprocess failure output that could contain credentials. Tests and examples must use synthetic state and dummy paths.
- Preserve resource reconciliation's no-clobber behavior: regular profile guidance, config, and skill entries are user overrides; only documented multicodex-owned symlinks may be changed. Runtime-managed `.system` skills remain profile-local.
- Keep Codex usage and routing weekly-only. Prefer declared 10,080-minute windows and retain only the existing narrow compatibility fallback for older provider responses.
- Keep Claude routing based on fresh official session, weekly all-model, and Fable usage without reading credential contents.
- Keep the CLI surface and error behavior aligned with `docs/command-spec.md`.

## Development

- Format Go changes with `gofmt`.
- Run focused tests while iterating, then `go test ./...`, `go test -race ./...`, and `go vet ./...` for material changes.
- Update `README.md` and `docs/command-spec.md` together when user-visible commands, flags, output, or routing behavior changes.
- Keep temporary plans and artifacts in ignored `plan/`; do not commit them.

## Change discipline

- Make small, evidence-backed changes and fail closed when routing, auth, or local state cannot be verified safely.
- Keep one clear current path. Do not add speculative fallbacks that hide failures or spend protected account quota unexpectedly.
- Treat user docs, command contracts, and security contracts as behavior.
- Verify close to the change first, then run the broader repository checks.

## Dictation-Aware Input Handling

- The user often dictates prompts, so minor transcription errors and homophone substitutions are expected.
- Infer intent from local context and repository state; ask a concise clarification only when ambiguity changes execution risk.
- Keep explicit typo dictionaries at workspace level (do not duplicate repo-local typo maps).

## Third-Party Dependency Trust Policy

- Prefer official packages, libraries, SDKs, frameworks, and services from authoritative sources.
- Prefer options that are reputable, well-maintained, popular, and well-supported.
- Before adopting or upgrading third-party dependencies, verify ownership/publisher authenticity, maintenance activity, security history, license fit, and ecosystem adoption.
- Avoid low-trust, obscure, or weakly maintained dependencies when a stronger alternative exists.
- Pin versions and keep lockfiles current for reproducibility and supply-chain safety.
- If trust signals are unclear, do not adopt the dependency until explicitly approved.

<!-- third-party-policy:start -->
## Third-Party Repository Handling

- External repositories may be cloned for static analysis only.
- Clone them only into ephemeral `plan/` locations such as `plan/scratch/upstream/` or `plan/artifacts/external/`.
- Immediately sanitize clone metadata: prefer `rm -rf .git`; if `.git` is temporarily needed, remove all remotes first and then remove `.git`.
- Never execute third-party code (no scripts, tests, builds, package installs, binaries, or containers).
- Persistent write remotes in this fork must reference only `github.com/Enrico-DA/*`.
- A read-only upstream remote may reference `github.com/olliecrow/multicodex`.
<!-- third-party-policy:end -->

## Plain English Default

- Use plain English in chat, session replies, docs, notes, comments, reports, commit messages, issue text, and review text.
- Prefer short words, short sentences, and direct statements.
- If a technical term is needed for correctness, explain it in simple words the first time.
- In code, prefer clear descriptive names for files, folders, flags, config keys, functions, classes, types, variables, tests, and examples.
- Avoid vague names, short cryptic names, and cute internal code names unless an old established name is already clearer than changing it.
- When touching old code, rename confusing names if the change is low risk and clearly improves readability.
- Keep the durable why for this rule in `docs/decisions.md`.
