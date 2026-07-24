#!/usr/bin/env python3
"""Mutation tests for the active product identity checker."""

from __future__ import annotations

import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.dont_write_bytecode = True

SCRIPT_DIRECTORY = Path(__file__).resolve().parent
REPOSITORY_ROOT = SCRIPT_DIRECTORY.parent.parent
sys.path.insert(0, str(SCRIPT_DIRECTORY))

from check_identity import check_repository  # noqa: E402


FIXTURE_FILES = (
    "go.mod",
    "cmd/multisubs/main.go",
    "internal/multisubs/app.go",
    "internal/multisubs/version.go",
    ".github/workflows/release.yml",
    "internal/monitor/usage/appserver.go",
    "internal/monitor/usage/oauth.go",
    "internal/monitor/tui/model.go",
)


class ProductIdentityMutationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary_directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary_directory.cleanup)
        self.fixture_root = Path(self.temporary_directory.name)
        for relative_path in FIXTURE_FILES:
            source = REPOSITORY_ROOT / relative_path
            destination = self.fixture_root / relative_path
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, destination)
        self.assertEqual([], check_repository(self.fixture_root))

    def mutate_active_line_and_keep_comment(
        self, relative_path: str, active_line: str, replacement_line: str
    ) -> list[str]:
        path = self.fixture_root / relative_path
        source = path.read_text(encoding="utf-8")
        self.assertEqual(1, source.splitlines().count(active_line))
        changed = source.replace(active_line, replacement_line, 1)
        changed += f"\n# retained expected text: {active_line.lstrip()}\n"
        path.write_text(changed, encoding="utf-8")
        return check_repository(self.fixture_root)

    def test_changed_repository_guard_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            "    if: ${{ github.repository == 'Enrico-DA/multi_subs' }}",
            "    if: ${{ github.repository == 'someone/other-repository' }}",
        )
        self.assertTrue(any("release repository guard" in error for error in errors))

    def test_changed_module_declaration_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "go.mod",
            "module github.com/Enrico-DA/multi_subs",
            "module example.invalid/other/module",
        )
        self.assertTrue(any("module declaration" in error for error in errors))

    def test_changed_command_import_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "cmd/multisubs/main.go",
            '\t"github.com/Enrico-DA/multi_subs/internal/multisubs"',
            '\tother "example.invalid/other/module"',
        )
        self.assertTrue(any("command import path" in error for error in errors))

    def test_changed_command_call_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "cmd/multisubs/main.go",
            "\tif err := multisubs.RunCLI(os.Args[1:]); err != nil {",
            "\tif err := other.RunCLI(os.Args[1:]); err != nil {",
        )
        self.assertTrue(
            any("command import relationship" in error for error in errors)
        )

    def test_changed_application_name_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/multisubs/version.go",
            'const appName = "multisubs"',
            'const appName = "other-command"',
        )
        self.assertTrue(any("application name" in error for error in errors))

    def test_changed_monitor_client_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/monitor/usage/appserver.go",
            'const clientName = "multisubs-monitor"',
            'const clientName = "other-monitor"',
        )
        self.assertTrue(any("monitor client identity" in error for error in errors))

    def test_changed_monitor_client_relationship_fails_with_expected_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/monitor/usage/appserver.go",
            "\t\t\tName:    clientName,",
            '\t\t\tName:    "other-client",',
        )
        self.assertTrue(
            any("monitor client name and version" in error for error in errors)
        )

    def test_changed_monitor_version_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/monitor/usage/appserver.go",
            "\t\t\tVersion: buildinfo.Version,",
            '\t\t\tVersion: "development",',
        )
        self.assertTrue(
            any("monitor client name and version" in error for error in errors)
        )

    def test_changed_user_agent_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/monitor/usage/oauth.go",
            '\treq.Header.Set("User-Agent", clientName+"/"+buildinfo.Version)',
            '\treq.Header.Set("User-Agent", "other-client")',
        )
        self.assertTrue(
            any("monitor user-agent relationship" in error for error in errors)
        )

    def test_changed_full_tui_name_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/monitor/tui/model.go",
            '\ttitle := m.styles.title.Render(" multisubs codex monitor ")',
            '\ttitle := m.styles.title.Render(" other monitor ")',
        )
        self.assertTrue(any("monitor TUI title" in error for error in errors))

    def test_changed_compact_tui_name_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            "internal/monitor/tui/model.go",
            '\t\tleft := m.styles.accent.Render("multisubs") + " " + stateStyle.Render(stateText)',
            '\t\tleft := m.styles.accent.Render("other") + " " + stateStyle.Render(stateText)',
        )
        self.assertTrue(
            any("compact monitor TUI product name" in error for error in errors)
        )

    def assert_old_product_path_fails(self, old_product: str) -> None:
        path = self.fixture_root / "internal" / old_product / "placeholder.go"
        path.parent.mkdir(parents=True)
        path.write_text("package placeholder\n", encoding="utf-8")

        errors = check_repository(self.fixture_root)
        self.assertTrue(
            any(
                "legacy product text is prohibited in repository paths" in error
                for error in errors
            )
        )

    def test_old_product_path_fails(self) -> None:
        self.assert_old_product_path_fails("multi" + "codex")

    def test_old_product_path_fails_case_insensitively(self) -> None:
        self.assert_old_product_path_fails("Multi" + "Codex")

    def test_new_legacy_read_in_approved_runtime_file_fails(self) -> None:
        path = self.fixture_root / "internal/multisubs/app.go"
        source = path.read_text(encoding="utf-8")
        old_home = "MULTI" + "CODEX" + "_HOME"
        changed = source + (
            "\nfunc prohibitedLegacyReadForMutationTest() string {\n"
            f'\treturn os.Getenv("{old_home}")\n'
            "}\n"
        )
        path.write_text(changed, encoding="utf-8")

        errors = check_repository(self.fixture_root)
        self.assertTrue(
            any(
                "legacy product text is outside the reviewed occurrence rules"
                in error
                for error in errors
            )
        )

    def test_changed_build_target_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            "              ./cmd/multisubs",
            "              ./cmd/other-command",
        )
        self.assertTrue(any("build target" in error for error in errors))

    def test_changed_linker_target_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            '              -ldflags="-s -w -X github.com/Enrico-DA/multi_subs/internal/buildinfo.Version=${RELEASE_TAG}" \\',
            '              -ldflags="-s -w -X example.invalid/other/internal/buildinfo.Version=${RELEASE_TAG}" \\',
        )
        self.assertTrue(any("linker target" in error for error in errors))

    def test_changed_output_binary_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            '              -o "dist/${name}/multisubs" \\',
            '              -o "dist/${name}/other-command" \\',
        )
        self.assertTrue(any("output binary" in error for error in errors))

    def test_changed_archive_name_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            '            name="multisubs_${version}_${os}_${arch}"',
            '            name="other-command_${version}_${os}_${arch}"',
        )
        self.assertTrue(any("archive name" in error for error in errors))

    def test_changed_archive_line_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            '            tar -C "dist/${name}" -czf "dist/${name}.tar.gz" multisubs LICENSE README.md',
            '            tar -C "dist/${name}" -czf "dist/${name}.tar.gz" other-command LICENSE README.md',
        )
        self.assertTrue(any("archive member list" in error for error in errors))

    def test_changed_version_line_fails_when_expected_text_remains_in_comment(
        self,
    ) -> None:
        errors = self.mutate_active_line_and_keep_comment(
            ".github/workflows/release.yml",
            '              test "$("dist/${name}/multisubs" version)" = "multisubs ${RELEASE_TAG}"',
            '              test "$("dist/${name}/other-command" version)" = "other-command ${RELEASE_TAG}"',
        )
        self.assertTrue(any("version assertion" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
