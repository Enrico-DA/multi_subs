# Security and Privacy

## Trust model
- `multicodex` is local-only.
- No external auth relay.
- No third-party secret storage.

## Secret handling rules
- Never print auth tokens, refresh tokens, or raw credential blobs.
- Never commit auth files or secret-bearing local state.
- Never copy, sync, transmit, transfer, or share Codex auth files or auth details between machines. Each machine must sign in through the official Codex login flow.
- Keep auth directories permissioned to the local user only.
- Keep profile `auth.json` files readable only by the local user.
- Use atomic writes to prevent partial secret files.
- Zero secret data from logs and diagnostics by default.
- User-visible diagnostics must never echo raw provider response bodies, app-server error messages, or Codex subprocess failure output. Preserve only safe status codes and allowlisted recovery guidance.
- Profile-scoped Codex subprocesses must scrub inherited Codex/OpenAI account override environment variables before setting the selected profile `CODEX_HOME`.
- Multicodex never reads, copies, writes, exports, or refreshes Claude credentials. Official Claude commands own login and token lifecycle.
- Managed Claude subprocesses scrub inherited Claude/Anthropic account overrides before setting the selected `CLAUDE_CONFIG_DIR`.
- The default Claude subprocess must receive no `CLAUDE_CONFIG_DIR`; an empty value is not equivalent.
- Claude usage inspection uses the official local `/usage` command and never calls OAuth endpoints directly.
- Claude usage probes disable session persistence, user/project settings, and MCP servers and run from a neutral directory.
- Claude auth, usage, and version probe failures discard captured standard error and arbitrary subprocess error text. User-visible failures use only local deterministic categories.
- Profile resource settings may name local directories outside the default Codex home. The user owns the trust decision for those sources; multicodex only creates symlinks and does not execute or copy source contents.
- Explicit skill sources must not overlap multicodex-owned state or the default Codex home, except for the canonical default skills directory. Inherited entries must resolve safely to directories. Reconciliation removes or retargets only symlinks at documented managed positions, preserves regular profile guidance and skill entries, and never owns `.system`.

## Repository safeguards
- `.gitignore` must ignore local auth and profile state.
- Recommended ignore coverage includes targeted current state paths: `**/multicodex/config.json` and `**/multicodex/profiles/`.
- Claude provider sidecars and profile state under `**/multicodex/providers/claude/` are sensitive local state and must remain untracked.
- Legacy `.multicodex/` state paths remain sensitive and should stay ignored.
- Tests must use synthetic fixtures only.
- Example files must never include real credentials.
- CI should run secret scanning before merge.
- `multicodex doctor` should be used before release to verify leak-guard checks.
- Committed tests, examples, logs, and review artifacts must use temporary or dummy resource paths and must not include private resource contents or machine-specific paths.

## Global auth boundary
- Multicodex must not change, restore, back up, symlink, lock, or otherwise manage the shared default Codex auth account.
- The system default Codex account is managed by normal Codex tooling outside multicodex.
- `multicodex exec` may run `codex exec` with the existing default Codex home as the final protected reserve account only when no configured profile has usable weekly usage. It must not mutate default auth state or expose default auth details.
- Monitor defaults include the global Codex home through direct read-only usage requests. Normal monitor usage may start profile-scoped read-only Codex app-server sessions only for validated multicodex profile homes. Active `CODEX_HOME`, filesystem discovery, and extra raw app-server checks require explicit monitor flags; `--include-default=false` omits the global home.
- Multicodex must not change, restore, back up, symlink, or otherwise manage the shared default Claude auth account.
- Managed Claude accounts live in private derived config directories. Profile paths, sidecars, and reservation files reject symlinks, hard links, unsafe permissions, and paths outside the provider tree.
- Claude routing accepts only first-party Max auth with a stable organization ID and deduplicates profiles that spend the same organization quota.
- Claude routing locks are keyed by a one-way hash of the organization ID and contain no credentials. The official child inherits the descriptor, so the lock remains held until that child exits even if the wrapper dies.
- On file-backed platforms, managed `.credentials.json` metadata must be a private regular single-link file. Multicodex validates metadata only and never reads its contents.
