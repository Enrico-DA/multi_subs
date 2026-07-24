# Upstream sync translation map

This fork keeps `olliecrow/multicodex` as its read-only upstream source and preserves Ollie's attribution and license. Fork work is pushed only to `Enrico-DA/multi_subs`.

When bringing in upstream changes, translate product-owned identity as follows:

| Upstream identity | This fork |
| --- | --- |
| `olliecrow/multicodex` | `Enrico-DA/multi_subs` |
| `github.com/olliecrow/multicodex` | `github.com/Enrico-DA/multi_subs` |
| `multicodex` executable and product text | `multisubs` |
| `cmd/multicodex` | `cmd/multisubs` |
| `internal/multicodex` package and directory | `internal/multisubs` |
| `package multicodex` | `package multisubs` |
| `~/multicodex` active state | `~/multisubs` |
| `MULTICODEX_*` active controls | `MULTISUBS_*` |
| bare Codex commands | `multisubs codex ...` |
| Claude provider commands | `multisubs claude ...` |

Do not translate:

- upstream attribution or license references;
- official provider names such as `CODEX_HOME` and `CLAUDE_CONFIG_DIR`;
- generic packages such as `internal/monitor`, `internal/codexstate`, and `internal/buildinfo`;
- old sensitive patterns used only by rejection, denylist, ignore, or leak-protection checks.

Review every sync for command-tree drift. Upstream bare Codex routes must not reappear. Upstream release guards, module paths, linker paths, archive names, help, completion, tests, and docs must be translated before the fork publishes a release.

The fork has no executable alias, dual home, environment fallback, or compatibility command. A sync must preserve that hard boundary.
