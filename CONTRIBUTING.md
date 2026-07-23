# Contributing

Thanks for helping improve multicodex.

## Development

Use the Go version declared in `go.mod`, then run:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Format Go changes with `gofmt`. If `pre-commit` is installed, enable the repository hooks with `pre-commit install --install-hooks`.

Keep changes narrow and preserve the contracts in `docs/command-spec.md` and `docs/security-and-privacy.md`. Update `README.md` and the command spec together when commands, flags, output, or routing behavior changes.

Tests and examples must use synthetic account data and portable paths. Never commit credentials, authentication state, private account details, or machine-specific paths. Report security issues through the process in `SECURITY.md`.

## Pull requests

Explain the user-visible outcome and the checks you ran. CI tests Linux and macOS, enables Go's race detector, runs `go vet`, builds the CLI, scans dependencies, and checks commits and patches for sensitive text.

## Releases

Do not create or publish version tags from `Enrico-DA/multi_subs` before the planned product and module rename. Install-from-module and release-download instructions are not valid for this fork during the transition.

The imported tag release workflow remains available for later adaptation, but its current repository guard allows publication only from `olliecrow/multicodex`. Do not widen that guard in an upstream-sync change.
