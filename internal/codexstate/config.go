package codexstate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const credentialStoreKey = "cli_auth_credentials_store"
const modelKey = "model"

// CredentialStoreFromTOML returns the root-level Codex credential store.
func CredentialStoreFromTOML(content string) (string, bool, error) {
	return rootStringFromTOML(content, credentialStoreKey, false)
}

// ModelFromTOML returns the effective root-level Codex model. Duplicate root
// model declarations are rejected because Codex rejects duplicate TOML keys.
func ModelFromTOML(content string) (string, bool, error) {
	return rootStringFromTOML(content, modelKey, true)
}

// ModelFromConfigOverride returns a model only for the exact root `model`
// override accepted by Codex's -c/--config option. Quoted TOML strings and
// Codex's unquoted string fallback are both supported.
func ModelFromConfigOverride(override string) (string, bool, error) {
	key, rawValue, ok := strings.Cut(override, "=")
	if !ok {
		if strings.TrimSpace(override) == modelKey {
			return "", true, errors.New("model override is missing '='")
		}
		return "", false, nil
	}
	if strings.TrimSpace(key) != modelKey {
		return "", false, nil
	}

	value := strings.TrimSpace(rawValue)
	if value == "" {
		return "", true, errors.New("model override has an empty value")
	}
	if value[0] == '"' || value[0] == '\'' {
		model, err := parseTOMLStringValue(modelKey, value)
		if err != nil {
			return "", true, err
		}
		if strings.TrimSpace(model) == "" {
			return "", true, errors.New("model override has an empty value")
		}
		return model, true, nil
	}
	return value, true, nil
}

func rootStringFromTOML(content, wantedKey string, rejectDuplicate bool) (string, bool, error) {
	inRootTable := true
	multilineDelimiter := ""
	foundValue := ""
	found := false
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if multilineDelimiter != "" {
			if strings.Contains(line, multilineDelimiter) {
				multilineDelimiter = ""
			}
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			inRootTable = false
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inRootTable = false
			continue
		}
		if !inRootTable {
			continue
		}

		assignIndex := indexTOMLUnquotedByte(line, '=')
		if assignIndex == -1 {
			continue
		}

		key, err := parseTOMLKey(line[:assignIndex])
		if err != nil {
			return "", false, err
		}
		if key != wantedKey {
			value := strings.TrimSpace(line[assignIndex+1:])
			switch {
			case strings.HasPrefix(value, `"""`) && strings.Count(value, `"""`)%2 == 1:
				multilineDelimiter = `"""`
			case strings.HasPrefix(value, `'''`) && strings.Count(value, `'''`)%2 == 1:
				multilineDelimiter = `'''`
			}
			continue
		}

		if found && rejectDuplicate {
			return "", false, fmt.Errorf("duplicate root %s key", wantedKey)
		}
		value, err := parseTOMLStringValue(wantedKey, line[assignIndex+1:])
		if err != nil {
			return "", false, err
		}
		foundValue = value
		found = true
		if !rejectDuplicate {
			return foundValue, true, nil
		}
	}
	return foundValue, found, nil
}

func parseTOMLKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", errors.New("empty config key")
	}
	if len(key) >= 2 && key[0] == '"' && key[len(key)-1] == '"' {
		unquoted, err := strconv.Unquote(key)
		if err != nil {
			return "", fmt.Errorf("invalid quoted config key %q: %w", key, err)
		}
		return unquoted, nil
	}
	if len(key) >= 2 && key[0] == '\'' && key[len(key)-1] == '\'' {
		return key[1 : len(key)-1], nil
	}
	return key, nil
}

func parseTOMLStringValue(key, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s has empty value", key)
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid quoted %s value: %w", key, err)
		}
		return unquoted, nil
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		if strings.Contains(value[1:len(value)-1], "'") {
			return "", fmt.Errorf("invalid literal %s value", key)
		}
		return value[1 : len(value)-1], nil
	}
	return "", fmt.Errorf("invalid %s value", key)
}

func stripTOMLComment(line string) string {
	inDouble := false
	inSingle := false
	escaped := false
	for i := 0; i < len(line); i++ {
		character := line[i]
		switch {
		case escaped:
			escaped = false
		case inDouble:
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inDouble = false
			}
		case inSingle:
			if character == '\'' {
				inSingle = false
			}
		default:
			switch character {
			case '"':
				inDouble = true
			case '\'':
				inSingle = true
			case '#':
				return line[:i]
			}
		}
	}
	return line
}

func indexTOMLUnquotedByte(value string, needle byte) int {
	inDouble := false
	inSingle := false
	escaped := false
	for i := 0; i < len(value); i++ {
		character := value[i]
		switch {
		case escaped:
			escaped = false
		case inDouble:
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inDouble = false
			}
		case inSingle:
			if character == '\'' {
				inSingle = false
			}
		default:
			switch character {
			case '"':
				inDouble = true
			case '\'':
				inSingle = true
			case needle:
				return i
			}
		}
	}
	return -1
}
