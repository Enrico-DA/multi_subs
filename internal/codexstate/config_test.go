package codexstate

import "testing"

func TestCredentialStoreFromTOML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantStore string
		wantFound bool
		wantError bool
	}{
		{name: "root basic string", content: "cli_auth_credentials_store = \"file\"\n", wantStore: "file", wantFound: true},
		{name: "root literal string", content: "cli_auth_credentials_store = 'file'\n", wantStore: "file", wantFound: true},
		{name: "escaped basic string", content: `cli_auth_credentials_store = "f\u0069le"`, wantStore: "file", wantFound: true},
		{name: "quoted equals before key", content: "\"not=the_key\" = \"x\"\ncli_auth_credentials_store = \"file\"\n", wantStore: "file", wantFound: true},
		{name: "commented key", content: "# cli_auth_credentials_store = \"file\"\n", wantFound: false},
		{name: "nested key", content: "[auth]\ncli_auth_credentials_store = \"file\"\n", wantFound: false},
		{name: "lookalike key", content: "cli_auth_credentials_store_backup = \"file\"\n", wantFound: false},
		{name: "invalid value", content: "cli_auth_credentials_store = file\n", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store, found, err := CredentialStoreFromTOML(test.content)
			if (err != nil) != test.wantError {
				t.Fatalf("CredentialStoreFromTOML() error = %v, wantError %v", err, test.wantError)
			}
			if store != test.wantStore || found != test.wantFound {
				t.Fatalf("CredentialStoreFromTOML() = (%q, %v), want (%q, %v)", store, found, test.wantStore, test.wantFound)
			}
		})
	}
}

func TestModelFromTOML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantModel string
		wantFound bool
		wantError bool
	}{
		{name: "root basic string", content: "model = \"gpt-5-codex-spark\"\n", wantModel: "gpt-5-codex-spark", wantFound: true},
		{name: "root literal raw string", content: "model = 'gpt-5-codex-spark'\n", wantModel: "gpt-5-codex-spark", wantFound: true},
		{name: "escaped basic string", content: `model = "gpt-5-codex-\u0073park"`, wantModel: "gpt-5-codex-spark", wantFound: true},
		{name: "quoted root key", content: `"model" = "gpt-5"`, wantModel: "gpt-5", wantFound: true},
		{name: "commented key", content: "# model = \"gpt-5\"\n", wantFound: false},
		{name: "nested key", content: "[profiles.fast]\nmodel = \"gpt-5-codex-spark\"\n", wantFound: false},
		{name: "lookalike key", content: "review_model = \"gpt-5-codex-spark\"\n", wantFound: false},
		{name: "non-string value", content: "model = 5\n", wantError: true},
		{name: "duplicate root key", content: "model = \"gpt-5\"\nmodel = \"gpt-5-codex-spark\"\n", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			model, found, err := ModelFromTOML(test.content)
			if (err != nil) != test.wantError {
				t.Fatalf("ModelFromTOML() error = %v, wantError %v", err, test.wantError)
			}
			if model != test.wantModel || found != test.wantFound {
				t.Fatalf("ModelFromTOML() = (%q, %v), want (%q, %v)", model, found, test.wantModel, test.wantFound)
			}
		})
	}
}

func TestModelFromConfigOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		override  string
		wantModel string
		wantFound bool
		wantError bool
	}{
		{name: "basic string", override: `model="gpt-5-codex-spark"`, wantModel: "gpt-5-codex-spark", wantFound: true},
		{name: "literal raw string", override: `model='gpt-5-codex-spark'`, wantModel: "gpt-5-codex-spark", wantFound: true},
		{name: "unquoted fallback", override: `model=gpt-5-codex-spark`, wantModel: "gpt-5-codex-spark", wantFound: true},
		{name: "space around key", override: ` model = gpt-5 `, wantModel: "gpt-5", wantFound: true},
		{name: "nested model", override: `profiles.fast.model=gpt-5-codex-spark`, wantFound: false},
		{name: "lookalike model", override: `review_model=gpt-5-codex-spark`, wantFound: false},
		{name: "missing equals for model", override: "model", wantFound: true, wantError: true},
		{name: "empty model", override: "model=", wantFound: true, wantError: true},
		{name: "broken quote", override: `model="gpt-5`, wantFound: true, wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			model, found, err := ModelFromConfigOverride(test.override)
			if (err != nil) != test.wantError {
				t.Fatalf("ModelFromConfigOverride() error = %v, wantError %v", err, test.wantError)
			}
			if model != test.wantModel || found != test.wantFound {
				t.Fatalf("ModelFromConfigOverride() = (%q, %v), want (%q, %v)", model, found, test.wantModel, test.wantFound)
			}
		})
	}
}
