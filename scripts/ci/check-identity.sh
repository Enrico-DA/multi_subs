#!/usr/bin/env bash
set -euo pipefail

script_directory="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(CDPATH= cd -- "${script_directory}/../.." && pwd)"
cd "${repository_root}"
exec go test ./internal/productidentity -run '^TestRepositoryIdentity$' -count=1
