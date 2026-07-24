package productidentity

import (
	"fmt"
	"strings"
)

type activeYAMLLine struct {
	number int
	indent int
	raw    string
	text   string
}

type releaseScriptLine struct {
	text  string
	label string
}

var expectedReleaseScript = []releaseScriptLine{
	{text: "set -euo pipefail", label: "shell safety setup"},
	{text: `version="${RELEASE_TAG#v}"`, label: "release version extraction"},
	{text: "mkdir -p dist", label: "release output directory"},
	{text: "for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do", label: "release target list"},
	{text: `  os="${target%/*}"`, label: "release operating system extraction"},
	{text: `  arch="${target#*/}"`, label: "release architecture extraction"},
	{text: `  name="multisubs_${version}_${os}_${arch}"`, label: "archive name"},
	{text: `  mkdir -p "dist/${name}"`, label: "archive staging directory"},
	{text: `  CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \`, label: "release build command"},
	{text: `    -trimpath \`, label: "release trimpath option"},
	{text: `    -ldflags="-s -w -X github.com/Enrico-DA/multi_subs/internal/buildinfo.Version=${RELEASE_TAG}" \`, label: "linker target"},
	{text: `    -o "dist/${name}/multisubs" \`, label: "output binary"},
	{text: "    ./cmd/multisubs", label: "build target"},
	{text: `  cp LICENSE README.md "dist/${name}/"`, label: "copied release files"},
	{text: `  tar -C "dist/${name}" -czf "dist/${name}.tar.gz" multisubs LICENSE README.md`, label: "archive members"},
	{text: `  if [[ "${target}" == "linux/amd64" ]]; then`, label: "version assertion target"},
	{text: `    test "$("dist/${name}/multisubs" version)" = "multisubs ${RELEASE_TAG}"`, label: "version assertion"},
	{text: "  fi", label: "version assertion terminator"},
	{text: "done", label: "release target loop terminator"},
	{text: "cd dist", label: "checksum directory"},
	{text: "sha256sum ./*.tar.gz > SHA256SUMS", label: "checksum command"},
}

func (repo *repository) checkReleaseWorkflow() {
	const relativePath = ".github/workflows/release.yml"
	source, ok := repo.readFile(relativePath)
	if !ok {
		return
	}
	lines := activeYAMLLines(string(source))

	jobs := directYAMLLines(lines, 0, len(lines), 0, "jobs:")
	if len(jobs) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active jobs mapping must occur exactly once; found %d", relativePath, len(jobs)))
		return
	}
	jobsEnd := yamlBlockEnd(lines, jobs[0], 0)

	releases := directYAMLLines(lines, jobs[0]+1, jobsEnd, 2, "release:")
	if len(releases) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active jobs.release mapping must occur exactly once; found %d", relativePath, len(releases)))
		return
	}
	releaseEnd := yamlBlockEndWithin(lines, releases[0], 2, jobsEnd)

	var guards []int
	for index := releases[0] + 1; index < releaseEnd; index++ {
		if lines[index].indent == 4 && strings.HasPrefix(lines[index].text, "if:") {
			guards = append(guards, index)
		}
	}
	const expectedGuard = "if: ${{ github.repository == 'Enrico-DA/multi_subs' }}"
	if len(guards) != 1 || lines[guards[0]].text != expectedGuard {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active jobs.release repository guard must be exactly %q", relativePath, expectedGuard))
	}

	steps := directYAMLLines(lines, releases[0]+1, releaseEnd, 4, "steps:")
	if len(steps) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active jobs.release.steps mapping must occur exactly once; found %d", relativePath, len(steps)))
		return
	}
	stepsEnd := yamlBlockEndWithin(lines, steps[0], 4, releaseEnd)

	archives := directYAMLLines(lines, steps[0]+1, stepsEnd, 6, "- name: Build release archives")
	if len(archives) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active release archive step must occur exactly once; found %d", relativePath, len(archives)))
		return
	}
	archiveEnd := stepEnd(lines, archives[0], stepsEnd)

	envBlocks := directYAMLLines(lines, archives[0]+1, archiveEnd, 8, "env:")
	if len(envBlocks) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active archive step must contain exactly one env mapping; found %d", relativePath, len(envBlocks)))
		return
	}
	envEnd := yamlBlockEndWithin(lines, envBlocks[0], 8, archiveEnd)
	releaseTags := directYAMLLines(lines, envBlocks[0]+1, envEnd, 10, "RELEASE_TAG: ${{ github.ref_name }}")
	if len(releaseTags) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: archive step must bind RELEASE_TAG to github.ref_name", relativePath))
	}
	runBlocks := directYAMLLines(lines, archives[0]+1, archiveEnd, 8, "run: |")
	if len(runBlocks) != 1 {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active archive step must contain exactly one literal run block; found %d", relativePath, len(runBlocks)))
		return
	}
	if envBlocks[0] >= runBlocks[0] {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: archive env mapping must precede its active run block", relativePath))
	}

	var script []string
	for index := runBlocks[0] + 1; index < archiveEnd; index++ {
		line := lines[index]
		if line.indent <= 8 {
			break
		}
		if line.indent < 10 {
			repo.errors = append(repo.errors, fmt.Sprintf("%s:%d: archive shell line must be indented beneath run block", relativePath, line.number))
			continue
		}
		script = append(script, line.raw[10:])
	}
	repo.compareReleaseScript(relativePath, script)
}

func (repo *repository) compareReleaseScript(relativePath string, actual []string) {
	if len(actual) != len(expectedReleaseScript) {
		repo.errors = append(repo.errors, fmt.Sprintf("%s: active archive run block must contain exactly %d non-comment commands in the reviewed sequence; found %d", relativePath, len(expectedReleaseScript), len(actual)))
	}
	limit := len(actual)
	if len(expectedReleaseScript) < limit {
		limit = len(expectedReleaseScript)
	}
	for index := 0; index < limit; index++ {
		expected := expectedReleaseScript[index]
		if actual[index] != expected.text {
			repo.errors = append(repo.errors, fmt.Sprintf("%s: active %s is incorrect at archive command %d; expected %q, found %q", relativePath, expected.label, index+1, expected.text, actual[index]))
		}
	}
}

func activeYAMLLines(source string) []activeYAMLLine {
	rawLines := strings.Split(source, "\n")
	lines := make([]activeYAMLLine, 0, len(rawLines))
	for index, raw := range rawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, activeYAMLLine{
			number: index + 1,
			indent: leadingSpaces(raw),
			raw:    raw,
			text:   strings.TrimLeft(raw, " "),
		})
	}
	return lines
}

func leadingSpaces(value string) int {
	for index := 0; index < len(value); index++ {
		if value[index] != ' ' {
			return index
		}
	}
	return len(value)
}

func directYAMLLines(lines []activeYAMLLine, start, end, indent int, text string) []int {
	var matches []int
	for index := start; index < end; index++ {
		if lines[index].indent == indent && lines[index].text == text {
			matches = append(matches, index)
		}
	}
	return matches
}

func yamlBlockEnd(lines []activeYAMLLine, start, indent int) int {
	return yamlBlockEndWithin(lines, start, indent, len(lines))
}

func yamlBlockEndWithin(lines []activeYAMLLine, start, indent, limit int) int {
	for index := start + 1; index < limit; index++ {
		if lines[index].indent <= indent {
			return index
		}
	}
	return limit
}

func stepEnd(lines []activeYAMLLine, start, limit int) int {
	for index := start + 1; index < limit; index++ {
		if lines[index].indent <= 4 || (lines[index].indent == 6 && strings.HasPrefix(lines[index].text, "- ")) {
			return index
		}
	}
	return limit
}
