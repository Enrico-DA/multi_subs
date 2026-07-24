#!/usr/bin/env python3
"""Check the active multisubs product identity without third-party packages."""

from __future__ import annotations

import sys
from pathlib import Path


EXPECTED_MODULE = "github.com/Enrico-DA/multi_subs"
EXPECTED_COMMAND_DIR = "cmd/multisubs"

# These files contain reviewed legacy names for attribution, hard-cut rejection,
# child-environment denylists, sensitive-path protection, or tests of those rules.
LEGACY_TEXT_ALLOWLIST = frozenset(
    {
        ".gitignore",
        "AGENTS.md",
        "CONTRIBUTING.md",
        "README.md",
        "docs/README.md",
        "docs/command-spec.md",
        "docs/decisions.md",
        "docs/security-and-privacy.md",
        "docs/upstream-sync.md",
        "internal/codexstate/env.go",
        "internal/codexstate/env_test.go",
        "internal/multisubs/app.go",
        "internal/multisubs/claude_process.go",
        "internal/multisubs/claude_process_test.go",
        "internal/multisubs/doctor.go",
        "internal/multisubs/doctor_test.go",
        "internal/multisubs/help_completion_test.go",
        "internal/multisubs/paths_test.go",
        "internal/multisubs/process_test.go",
        "internal/multisubs/run_cli_test.go",
        "scripts/ci/check_identity.py",
    }
)

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
    for path in sorted(root.rglob("*")):
        if path.is_symlink() or not path.is_file():
            continue
        relative = path.relative_to(root)
        if any(part in IGNORED_SCAN_DIRECTORIES for part in relative.parts):
            continue
        relative_path = relative.as_posix()
        if relative_path in LEGACY_TEXT_ALLOWLIST:
            continue
        try:
            data = path.read_bytes()
        except OSError as error:
            errors.append(f"{relative_path}: cannot scan for legacy identity: {error}")
            continue
        if b"multicodex" in data.lower():
            errors.append(
                f"{relative_path}: legacy product text is outside the reviewed allowlist"
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
