package multisubs

import (
	"fmt"

	"github.com/Enrico-DA/multi_subs/internal/codexstate"
)

const codexDefaultAccountName = "default"

func ValidateProfileName(name string) error {
	return codexstate.ValidateProfileName(name)
}

func ValidateCodexProfileName(name string) error {
	if err := codexstate.ValidateProfileName(name); err != nil {
		return err
	}
	if name == codexDefaultAccountName {
		return fmt.Errorf("profile name %q is reserved for the built-in default Codex account", name)
	}
	return nil
}
