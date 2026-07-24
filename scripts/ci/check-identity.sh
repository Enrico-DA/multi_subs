#!/usr/bin/env bash
set -euo pipefail

expected_module="github.com/Enrico-DA/multi_subs"
expected_command_dir="cmd/multisubs"
release_workflow=".github/workflows/release.yml"

if [[ "$(sed -n '1p' go.mod)" != "module ${expected_module}" ]]; then
  echo "go.mod module identity does not match ${expected_module}" >&2
  exit 1
fi

command_dirs="$(find cmd -mindepth 1 -maxdepth 1 -type d -print | sort)"
if [[ "${command_dirs}" != "${expected_command_dir}" ]]; then
  echo "cmd must contain only ${expected_command_dir}; found: ${command_dirs}" >&2
  exit 1
fi

grep -Fq "\"${expected_module}/internal/multisubs\"" "${expected_command_dir}/main.go"
grep -Fq "github.repository == 'Enrico-DA/multi_subs'" "${release_workflow}"
grep -Fq "${expected_module}/internal/buildinfo.Version" "${release_workflow}"
grep -Fq './cmd/multisubs' "${release_workflow}"
grep -Fq 'name="multisubs_${version}_${os}_${arch}"' "${release_workflow}"
grep -Fq -- '-o "dist/${name}/multisubs"' "${release_workflow}"
grep -Fq 'multisubs LICENSE README.md' "${release_workflow}"

echo "multisubs product identity is internally consistent"
