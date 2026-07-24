#!/usr/bin/env python3
"""Check the active multisubs product identity without third-party packages."""

from __future__ import annotations

import sys
from pathlib import Path


EXPECTED_MODULE = "github.com/Enrico-DA/multi_subs"
EXPECTED_COMMAND_DIR = "cmd/multisubs"
LEGACY_PRODUCT = "multi" + "codex"
LEGACY_ENV_PREFIX = "MULTI" + "CODEX"


def legacy_lines(*templates: str) -> frozenset[str]:
    return frozenset(
        template.replace("{legacy}", LEGACY_PRODUCT).replace(
            "{LEGACY}", LEGACY_ENV_PREFIX
        )
        for template in templates
    )


# Each reviewed legacy occurrence must remain on one exact line for its approved
# attribution, rejection, denylist, leak-protection, or negative-test purpose.
# A whole file is never exempt from the scan.
APPROVED_LEGACY_LINES = {
    ".gitignore": legacy_lines(
        ".{legacy}/",
        "**/.{legacy}/config.json",
        "**/.{legacy}/profiles/",
        "**/{legacy}/config.json",
        "**/{legacy}/profiles/",
        "**/{legacy}/providers/claude/",
        "/{legacy}",
    ),
    "AGENTS.md": legacy_lines(
        "- The upstream project is `olliecrow/{legacy}`; preserve its attribution and license.",
        "- A read-only upstream remote may reference `github.com/olliecrow/{legacy}`.",
    ),
    "CONTRIBUTING.md": legacy_lines(
        "Pull-request CI runs a non-publishing identity check. When syncing from `olliecrow/{legacy}`, apply the translation map in `docs/upstream-sync.md` and preserve upstream attribution and license text.",
    ),
    "README.md": legacy_lines(
        "Any legacy `{LEGACY}_*` variable causes startup to fail before state access. Clear it before running `multisubs`. Runtime never reads the old product home or old environment namespace. Old variables are still removed from provider child environments as a denylist.",
        "This fork is based on [olliecrow/{legacy}](https://github.com/olliecrow/{legacy}) and preserves its attribution and license.",
    ),
    "docs/README.md": legacy_lines(
        "- [`upstream-sync.md`](upstream-sync.md) records the durable identity and command translation from `olliecrow/{legacy}`.",
    ),
    "docs/command-spec.md": legacy_lines(
        "Startup checks the environment before path resolution. If any `{LEGACY}_*` variable is present, the command exits with code 2 and tells the user to clear it.",
        "Runtime never reads the old environment namespace or the old `~/{legacy}` state root. Known old variables remain on provider child-environment denylists to prevent account-routing leakage.",
    ),
    "docs/decisions.md": legacy_lines(
        "Decision: Preserve the Apache 2.0 license terms and attribution to `olliecrow/{legacy}`.",
        "Decision: Old `{LEGACY}_*` variables cause startup to fail before state access.",
        "Enforcement: Top-level startup rejects any old product-prefixed variable. Provider child environments still strip old controls. The old `~/{legacy}` and `.{legacy}` patterns remain only as legacy-sensitive ignore and leak protection.",
    ),
    "docs/security-and-privacy.md": legacy_lines(
        "- known legacy `{LEGACY}_*` controls.",
        "- Any `{LEGACY}_*` variable rejects top-level startup before state access.",
        "- Runtime never reads `{LEGACY}_HOME`.",
        "- Runtime never defaults to `~/{legacy}`.",
        "- `.{legacy}`, `{legacy}` state paths, and old environment names remain only in ignore, leak, denylist, and rejection tests so old credentials cannot be committed or inherited.",
        "Tests and examples use synthetic values and dummy paths. Upstream attribution to `olliecrow/{legacy}` is not a runtime compatibility reference.",
    ),
    "docs/upstream-sync.md": legacy_lines(
        "This fork keeps `olliecrow/{legacy}` as its read-only upstream source and preserves Ollie's attribution and license. Fork work is pushed only to `Enrico-DA/multi_subs`.",
        "| `olliecrow/{legacy}` | `Enrico-DA/multi_subs` |",
        "| `github.com/olliecrow/{legacy}` | `github.com/Enrico-DA/multi_subs` |",
        "| `{legacy}` executable and product text | `multisubs` |",
        "| `cmd/{legacy}` | `cmd/multisubs` |",
        "| `internal/{legacy}` package and directory | `internal/multisubs` |",
        "| `package {legacy}` | `package multisubs` |",
        "| `~/{legacy}` active state | `~/multisubs` |",
        "| `{LEGACY}_*` active controls | `MULTISUBS_*` |",
    ),
    "internal/codexstate/env.go": legacy_lines(
        '\tif strings.HasPrefix(key, "{LEGACY}_") {',
    ),
    "internal/codexstate/env_test.go": legacy_lines(
        '\t\t"{LEGACY}_HOME=/legacy-product-state",',
        '\t\t"{LEGACY}_ACTIVE_PROFILE=legacy",',
        '\tfor _, forbidden := range []string{"CODEX_HOME=/stale", "MULTISUBS_HOME=", "MULTISUBS_ACTIVE_PROFILE=", "{LEGACY}_HOME=", "{LEGACY}_ACTIVE_PROFILE=", "CODEX_USAGE_MONITOR_ACCOUNTS_FILE=", "OPENAI_API_KEY=", "CODEX_AUTH_TOKEN=", "INVALID_ENTRY"} {',
    ),
    "internal/multisubs/app.go": legacy_lines(
        '\t\tif ok && strings.HasPrefix(name, "{LEGACY}_") {',
        '\t\tMessage: fmt.Sprintf("legacy {LEGACY}_* environment variable(s) are set: %s; clear them before running multisubs", strings.Join(names, ", ")),',
    ),
    "internal/multisubs/claude_process.go": legacy_lines(
        '\tif strings.HasPrefix(key, "{LEGACY}_") {',
    ),
    "internal/multisubs/claude_process_test.go": legacy_lines(
        '\t\t"{LEGACY}_HOME=/tmp/legacy",',
        '\t\t"{LEGACY}_CLAUDE_PROFILE=legacy",',
        '\t\t"{LEGACY}_HOME",',
        '\t\t"{LEGACY}_CLAUDE_PROFILE",',
    ),
    "internal/multisubs/doctor.go": legacy_lines(
        '\t\t".{legacy}/",',
        '\t\t"**/{legacy}/config.json",',
        '\t\t"**/{legacy}/profiles/",',
        '\t\t"**/{legacy}/providers/claude/",',
        '\t\t"**/.{legacy}/config.json",',
        '\t\t"**/.{legacy}/profiles/",',
        '\t\t"{legacy}/config.json",',
        '\t\t".{legacy}/config.json",',
        '\t\t"github.com/olliecrow/{legacy}/config.json",',
        '\t\t"github.com/olliecrow/.{legacy}/config.json",',
        '\t\t"github.com/enrico-da/{legacy}/config.json",',
        '\t\t"github.com/enrico-da/.{legacy}/config.json",',
        '\t\tstrings.Contains(clean, "/{legacy}/config.json") ||',
        '\t\tstrings.Contains(clean, "/.{legacy}/config.json") {',
        '\t\t"{legacy}/profiles/",',
        '\t\t".{legacy}/profiles/",',
        '\t\t"github.com/olliecrow/{legacy}/profiles/",',
        '\t\t"github.com/olliecrow/.{legacy}/profiles/",',
        '\t\t"github.com/enrico-da/{legacy}/profiles/",',
        '\t\t"github.com/enrico-da/.{legacy}/profiles/",',
        '\t\tstrings.Contains(clean, "/{legacy}/profiles/") ||',
        '\t\tstrings.Contains(clean, "/.{legacy}/profiles/") {',
        '\t\tstrings.Contains(clean, "/{legacy}/providers/claude/") ||',
        '\t\tstrings.HasPrefix(clean, "{legacy}/providers/claude/") {',
    ),
    "internal/multisubs/doctor_test.go": legacy_lines(
        '\t\t"**/{legacy}/config.json",',
        '\t\t"**/{legacy}/profiles/",',
        '\t\t"**/{legacy}/providers/claude/",',
        '\t\t"**/.{legacy}/config.json",',
        '\t\t"**/.{legacy}/profiles/",',
        '\t\t".{legacy}/",',
        '\tfor _, want := range []string{".multisubs/", ".{legacy}/", "**/multisubs/config.json", "**/{legacy}/config.json", "**/auth.json"} {',
        '\tfor _, want := range []string{".{legacy}/", "**/{legacy}/config.json", "**/{legacy}/profiles/"} {',
        '\t\t{path: "github.com/olliecrow/{legacy}/config.json", sensitive: true},',
        '\t\t{path: "github.com/olliecrow/{legacy}/profiles/work/codex-home/config.toml", sensitive: true},',
        '\t\t{path: ".{legacy}/config.json", sensitive: true},',
        '\t\t{path: ".{legacy}/profiles/work/codex-home/config.toml", sensitive: true},',
        '\t\t{path: "github.com/olliecrow/.{legacy}/config.json", sensitive: true},',
        '\t\t{path: "github.com/olliecrow/{legacy}/docs/readme.md", sensitive: false},',
    ),
    "internal/multisubs/help_completion_test.go": legacy_lines(
        '\tif strings.Contains(out, "{legacy}") {',
        '\tif !strings.HasPrefix(out, "multisubs ") || strings.Contains(out, "{legacy}") {',
        '\t\t\tif strings.Contains(test.out, "{legacy}") || strings.Contains(test.out, "__complete-profiles") {',
    ),
    "internal/multisubs/paths_test.go": legacy_lines(
        '\tlegacyHome := filepath.Join(t.TempDir(), "{legacy}")',
        '\tt.Setenv("{LEGACY}_HOME", legacyHome)',
        '\tt.Setenv("{LEGACY}_DEFAULT_CODEX_HOME", filepath.Join(t.TempDir(), "legacy-codex"))',
    ),
    "internal/multisubs/process_test.go": legacy_lines(
        '\t\t"{LEGACY}_HOME=/tmp/legacy",',
        '\t\t"{LEGACY}_ACTIVE_PROFILE=legacy",',
        '\t\t"{LEGACY}_HOME=",',
        '\t\t"{LEGACY}_ACTIVE_PROFILE=",',
    ),
    "internal/multisubs/run_cli_test.go": legacy_lines(
        '\tlegacyHome := filepath.Join(home, "{legacy}")',
        '\tt.Setenv("{LEGACY}_HOME", legacyHome)',
    ),
}

IGNORED_SCAN_DIRECTORIES = frozenset({".git", "__pycache__", "dist", "plan"})


def read_text(root: Path, relative_path: str, errors: list[str]) -> str:
    path = root / relative_path
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        errors.append(f"{relative_path}: cannot read identity source: {error}")
        return ""


def require_exact_line(
    errors: list[str],
    relative_path: str,
    text: str,
    expected_line: str,
    label: str,
    expected_count: int = 1,
) -> None:
    count = sum(line == expected_line for line in text.splitlines())
    if count != expected_count:
        errors.append(
            f"{relative_path}: {label} must be one exact active line; found {count}"
        )


def require_exact_sequence(
    errors: list[str],
    relative_path: str,
    text: str,
    expected_lines: tuple[str, ...],
    label: str,
) -> None:
    lines = text.splitlines()
    width = len(expected_lines)
    count = sum(
        tuple(lines[index : index + width]) == expected_lines
        for index in range(len(lines) - width + 1)
    )
    if count != 1:
        errors.append(
            f"{relative_path}: {label} must be one exact active relationship; found {count}"
        )


def check_command_directory(root: Path, errors: list[str]) -> None:
    command_root = root / "cmd"
    try:
        entries = sorted(command_root.iterdir(), key=lambda path: path.name)
    except OSError as error:
        errors.append(f"cmd: cannot inspect command directory: {error}")
        return

    if (
        len(entries) != 1
        or entries[0].name != "multisubs"
        or not entries[0].is_dir()
        or entries[0].is_symlink()
    ):
        found = ", ".join(entry.relative_to(root).as_posix() for entry in entries)
        errors.append(f"cmd: must contain only {EXPECTED_COMMAND_DIR}; found: {found}")


def check_legacy_text(root: Path, errors: list[str]) -> None:
    legacy_bytes = LEGACY_PRODUCT.encode("ascii")
    approved_line_counts: dict[tuple[str, str], int] = {}

    for path in sorted(root.rglob("*")):
        relative = path.relative_to(root)
        if any(part in IGNORED_SCAN_DIRECTORIES for part in relative.parts):
            continue
        relative_path = relative.as_posix()
        if LEGACY_PRODUCT in relative_path.casefold():
            errors.append(
                f"{relative_path}: legacy product text is prohibited in repository paths"
            )
        if path.is_symlink() or not path.is_file():
            continue
        try:
            data = path.read_bytes()
        except OSError as error:
            errors.append(f"{relative_path}: cannot scan for legacy identity: {error}")
            continue
        if legacy_bytes not in data.lower():
            continue
        try:
            text = data.decode("utf-8")
        except UnicodeError as error:
            errors.append(
                f"{relative_path}: cannot decode legacy identity occurrence: {error}"
            )
            continue

        approved_lines = APPROVED_LEGACY_LINES.get(relative_path, frozenset())
        for line_number, line in enumerate(text.splitlines(), start=1):
            if LEGACY_PRODUCT not in line.casefold():
                continue
            key = (relative_path, line)
            approved_line_counts[key] = approved_line_counts.get(key, 0) + 1
            if line not in approved_lines or approved_line_counts[key] > 1:
                errors.append(
                    f"{relative_path}:{line_number}: legacy product text is outside "
                    "the reviewed occurrence rules"
                )


def check_repository(root: Path) -> list[str]:
    root = root.resolve()
    errors: list[str] = []

    go_mod = read_text(root, "go.mod", errors)
    require_exact_line(
        errors,
        "go.mod",
        go_mod,
        f"module {EXPECTED_MODULE}",
        "module declaration",
    )
    if go_mod.splitlines()[:1] != [f"module {EXPECTED_MODULE}"]:
        errors.append("go.mod: module declaration must be the first line")

    check_command_directory(root, errors)

    main_source = read_text(root, "cmd/multisubs/main.go", errors)
    require_exact_line(
        errors,
        "cmd/multisubs/main.go",
        main_source,
        f'\t"{EXPECTED_MODULE}/internal/multisubs"',
        "command import path",
    )
    require_exact_line(
        errors,
        "cmd/multisubs/main.go",
        main_source,
        "\tif err := multisubs.RunCLI(os.Args[1:]); err != nil {",
        "command import relationship",
    )

    version_source = read_text(root, "internal/multisubs/version.go", errors)
    require_exact_line(
        errors,
        "internal/multisubs/version.go",
        version_source,
        'const appName = "multisubs"',
        "application name",
    )

    release_path = ".github/workflows/release.yml"
    release = read_text(root, release_path, errors)
    release_lines = {
        "release job": "  release:",
        "release repository guard": "    if: ${{ github.repository == 'Enrico-DA/multi_subs' }}",
        "release archive step": "      - name: Build release archives",
        "archive name": '            name="multisubs_${version}_${os}_${arch}"',
        "linker target": '              -ldflags="-s -w -X github.com/Enrico-DA/multi_subs/internal/buildinfo.Version=${RELEASE_TAG}" \\',
        "output binary": '              -o "dist/${name}/multisubs" \\',
        "build target": "              ./cmd/multisubs",
        "archive member list": '            tar -C "dist/${name}" -czf "dist/${name}.tar.gz" multisubs LICENSE README.md',
        "version assertion": '              test "$("dist/${name}/multisubs" version)" = "multisubs ${RELEASE_TAG}"',
    }
    for label, expected_line in release_lines.items():
        require_exact_line(errors, release_path, release, expected_line, label)

    require_exact_sequence(
        errors,
        release_path,
        release,
        (
            "  # The fork enables releases after its product and module rename.",
            "  release:",
            "    if: ${{ github.repository == 'Enrico-DA/multi_subs' }}",
            "    runs-on: ubuntu-latest",
            "    timeout-minutes: 30",
        ),
        "release job and repository guard",
    )
    require_exact_sequence(
        errors,
        release_path,
        release,
        (
            "      - name: Build release archives",
            "        env:",
            "          RELEASE_TAG: ${{ github.ref_name }}",
            "        run: |",
            "          set -euo pipefail",
            '          version="${RELEASE_TAG#v}"',
            "          mkdir -p dist",
            "          for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do",
            '            os="${target%/*}"',
            '            arch="${target#*/}"',
            '            name="multisubs_${version}_${os}_${arch}"',
            '            mkdir -p "dist/${name}"',
            '            CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \\',
            "              -trimpath \\",
            '              -ldflags="-s -w -X github.com/Enrico-DA/multi_subs/internal/buildinfo.Version=${RELEASE_TAG}" \\',
            '              -o "dist/${name}/multisubs" \\',
            "              ./cmd/multisubs",
            '            cp LICENSE README.md "dist/${name}/"',
            '            tar -C "dist/${name}" -czf "dist/${name}.tar.gz" multisubs LICENSE README.md',
            '            if [[ "${target}" == "linux/amd64" ]]; then',
            '              test "$("dist/${name}/multisubs" version)" = "multisubs ${RELEASE_TAG}"',
            "            fi",
            "          done",
            "          cd dist",
            "          sha256sum ./*.tar.gz > SHA256SUMS",
        ),
        "active release archive script",
    )
    require_exact_sequence(
        errors,
        release_path,
        release,
        (
            '            CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \\',
            "              -trimpath \\",
            '              -ldflags="-s -w -X github.com/Enrico-DA/multi_subs/internal/buildinfo.Version=${RELEASE_TAG}" \\',
            '              -o "dist/${name}/multisubs" \\',
            "              ./cmd/multisubs",
        ),
        "release build target, linker, and output",
    )
    require_exact_sequence(
        errors,
        release_path,
        release,
        (
            '            cp LICENSE README.md "dist/${name}/"',
            '            tar -C "dist/${name}" -czf "dist/${name}.tar.gz" multisubs LICENSE README.md',
            '            if [[ "${target}" == "linux/amd64" ]]; then',
            '              test "$("dist/${name}/multisubs" version)" = "multisubs ${RELEASE_TAG}"',
            "            fi",
        ),
        "archive contents and version assertion",
    )

    app_server_path = "internal/monitor/usage/appserver.go"
    app_server = read_text(root, app_server_path, errors)
    require_exact_line(
        errors,
        app_server_path,
        app_server,
        'const clientName = "multisubs-monitor"',
        "monitor client identity",
    )
    require_exact_sequence(
        errors,
        app_server_path,
        app_server,
        (
            "\t\tClientInfo: clientInfo{",
            "\t\t\tName:    clientName,",
            "\t\t\tVersion: buildinfo.Version,",
            "\t\t},",
        ),
        "monitor client name and version",
    )
    require_exact_line(
        errors,
        app_server_path,
        app_server,
        "\t\t\tName:    clientName,",
        "monitor initialization client name",
    )
    require_exact_line(
        errors,
        app_server_path,
        app_server,
        "\t\t\tVersion: buildinfo.Version,",
        "monitor initialization client version",
    )

    oauth_path = "internal/monitor/usage/oauth.go"
    oauth = read_text(root, oauth_path, errors)
    require_exact_line(
        errors,
        oauth_path,
        oauth,
        '\treq.Header.Set("User-Agent", clientName+"/"+buildinfo.Version)',
        "monitor user-agent relationship",
    )

    tui_path = "internal/monitor/tui/model.go"
    tui = read_text(root, tui_path, errors)
    require_exact_line(
        errors,
        tui_path,
        tui,
        '\ttitle := m.styles.title.Render(" multisubs codex monitor ")',
        "monitor TUI title",
    )
    require_exact_line(
        errors,
        tui_path,
        tui,
        '\t\tleft := m.styles.accent.Render("multisubs") + " " + stateStyle.Render(stateText)',
        "compact monitor TUI product name",
    )

    check_legacy_text(root, errors)
    return errors


def main(argv: list[str]) -> int:
    if len(argv) > 2:
        print("usage: check_identity.py [repository-root]", file=sys.stderr)
        return 2
    root = Path(argv[1]) if len(argv) == 2 else Path.cwd()
    errors = check_repository(root)
    if errors:
        for error in errors:
            print(f"identity error: {error}", file=sys.stderr)
        return 1
    print("multisubs product identity is internally consistent")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
