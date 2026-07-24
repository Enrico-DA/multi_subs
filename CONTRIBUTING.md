# Contributing

Thanks for helping improve multisubs.

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

Release publication is guarded to `Enrico-DA/multi_subs`. Release archives, checksums, linker targets, command paths, and version output must use the `multisubs` identity.

Pull-request CI runs a non-publishing identity check. When syncing from `olliecrow/multicodex`, apply the translation map in `docs/upstream-sync.md` and preserve upstream attribution and license text.
