#!/usr/bin/env python3
"""Mutation tests for the active product identity checker."""

from __future__ import annotations

import shutil
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT_DIRECTORY = Path(__file__).resolve().parent
REPOSITORY_ROOT = SCRIPT_DIRECTORY.parent.parent
sys.path.insert(0, str(SCRIPT_DIRECTORY))

from check_identity import check_repository  # noqa: E402


FIXTURE_FILES = (
    "go.mod",
    "cmd/multisubs/main.go",
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
