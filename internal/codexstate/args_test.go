package codexstate

import (
	"reflect"
	"testing"
)

func TestWithManagedAuthOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no delimiter",
			args: []string{"--model", "gpt-5", "prompt"},
			want: []string{"--model", "gpt-5", "prompt", "-c", `cli_auth_credentials_store="file"`},
		},
		{
			name: "before delimiter",
			args: []string{"--sandbox", "read-only", "--", "-c", "prompt text"},
			want: []string{"--sandbox", "read-only", "-c", ManagedAuthConfig, "--", "-c", "prompt text"},
		},
		{
			name: "short config",
			args: []string{"-c", `cli_auth_credentials_store=\"keyring\"`, "prompt"},
			want: []string{"-c", `cli_auth_credentials_store=\"keyring\"`, "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "long config",
			args: []string{"--config", `cli_auth_credentials_store=\"keyring\"`, "prompt"},
			want: []string{"--config", `cli_auth_credentials_store=\"keyring\"`, "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "long config equals",
			args: []string{`--config=cli_auth_credentials_store=\"keyring\"`, "prompt"},
			want: []string{`--config=cli_auth_credentials_store=\"keyring\"`, "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "attached short config",
			args: []string{`-ccli_auth_credentials_store=\"keyring\"`, "prompt"},
			want: []string{`-ccli_auth_credentials_store=\"keyring\"`, "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "repeated config overrides",
			args: []string{"-c", "one=1", "--config=two=2", "-cthree=3", "prompt"},
			want: []string{"-c", "one=1", "--config=two=2", "-cthree=3", "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "short profile",
			args: []string{"-p", "unsafe", "prompt"},
			want: []string{"-p", "unsafe", "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "long profile",
			args: []string{"--profile", "unsafe", "prompt"},
			want: []string{"--profile", "unsafe", "prompt", "-c", ManagedAuthConfig},
		},
		{
			name: "harmless arguments keep order",
			args: []string{"--model=gpt-5", "--sandbox", "workspace-write", "review this repo"},
			want: []string{"--model=gpt-5", "--sandbox", "workspace-write", "review this repo", "-c", ManagedAuthConfig},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			original := append([]string(nil), test.args...)
			got := WithManagedAuthOverride(test.args)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("managed args: got %#v want %#v", got, test.want)
			}
			if !reflect.DeepEqual(test.args, original) {
				t.Fatalf("input args changed: got %#v want %#v", test.args, original)
			}
		})
	}
}
