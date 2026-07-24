#!/usr/bin/env bash
set -euo pipefail

script_directory="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(CDPATH= cd -- "${script_directory}/../.." && pwd)"
exec python3 "${script_directory}/check_identity.py" "${repository_root}"
