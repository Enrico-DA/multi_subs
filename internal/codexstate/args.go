package codexstate

// ManagedAuthConfig is the Codex CLI override for managed file-backed auth.
const ManagedAuthConfig = `cli_auth_credentials_store="file"`

// WithManagedAuthOverride preserves args and appends the managed file-backed
// auth override before the first standalone argument delimiter.
func WithManagedAuthOverride(args []string) []string {
	insertAt := len(args)
	for i, arg := range args {
		if arg == "--" {
			insertAt = i
			break
		}
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args[:insertAt]...)
	out = append(out, "-c", ManagedAuthConfig)
	out = append(out, args[insertAt:]...)
	return out
}
