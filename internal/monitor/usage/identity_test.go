package usage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReconcileCodexIdentitiesUsesAccountIDBeforeEmail(t *testing.T) {
	groups := ReconcileCodexIdentities([]CodexIdentityRecord{
		{AccountID: "account-one", AccountEmail: "Shared@Example.com"},
		{AccountID: "account-two", AccountEmail: "shared@example.com"},
	})
	if len(groups) != 2 {
		t.Fatalf("group count: got %d want 2", len(groups))
	}
	for index, group := range groups {
		if !reflect.DeepEqual(group.MemberIndexes, []int{index}) ||
			group.AccountEmail != "shared@example.com" ||
			!group.Resolved || group.Conflicted {
			t.Fatalf("group %d: %+v", index, group)
		}
	}
}

func TestReconcileCodexIdentitiesAttachesEmailOnlyRecordToOneStrongID(t *testing.T) {
	groups := ReconcileCodexIdentities([]CodexIdentityRecord{
		{AccountID: "account-one", AccountEmail: "person@example.com"},
		{AccountEmail: " PERSON@example.com "},
	})
	if len(groups) != 1 {
		t.Fatalf("group count: got %d want 1", len(groups))
	}
	if !reflect.DeepEqual(groups[0].MemberIndexes, []int{0, 1}) ||
		groups[0].AccountEmail != "person@example.com" ||
		!groups[0].Resolved || groups[0].Conflicted {
		t.Fatalf("group: %+v", groups[0])
	}
}

func TestReconcileCodexIdentitiesKeepsConflictingStrongIDsSeparate(t *testing.T) {
	groups := ReconcileCodexIdentities([]CodexIdentityRecord{
		{AccountID: "account-one", AccountEmail: "person@example.com"},
		{AccountID: "account-two", AccountEmail: "person@example.com"},
		{AccountEmail: "person@example.com"},
	})
	if len(groups) != 3 {
		t.Fatalf("group count: got %d want 3", len(groups))
	}
	if !groups[0].Resolved || !groups[1].Resolved {
		t.Fatalf("strong groups were not resolved: %+v", groups)
	}
	if groups[2].Resolved || !groups[2].Conflicted || groups[2].AccountEmail != "" {
		t.Fatalf("ambiguous email-only group: %+v", groups[2])
	}
}

func TestReconcileCodexIdentitiesKeepsEachAmbiguousFallbackSeparate(t *testing.T) {
	groups := ReconcileCodexIdentities([]CodexIdentityRecord{
		{AccountID: "account-one", AccountEmail: "person@example.com"},
		{AccountID: "account-two", AccountEmail: "person@example.com"},
		{AccountEmail: "person@example.com"},
		{AccountEmail: "person@example.com"},
	})
	if len(groups) != 4 {
		t.Fatalf("group count: got %d want 4", len(groups))
	}
	for index := 2; index < 4; index++ {
		if !reflect.DeepEqual(groups[index].MemberIndexes, []int{index}) ||
			groups[index].Resolved || !groups[index].Conflicted ||
			groups[index].AccountEmail != "" {
			t.Fatalf("ambiguous fallback group %d: %+v", index, groups[index])
		}
	}
}

func TestReconcileCodexIdentitiesNeverUsesUserIDOrMalformedEmail(t *testing.T) {
	groups := ReconcileCodexIdentities([]CodexIdentityRecord{
		{AccountEmail: "not-an-email"},
		{AccountEmail: "person@example.com"},
		{},
	})
	if len(groups) != 3 {
		t.Fatalf("group count: got %d want 3", len(groups))
	}
	if groups[0].Resolved || groups[0].AccountEmail != "" ||
		!groups[1].Resolved || groups[1].AccountEmail != "person@example.com" ||
		groups[2].Resolved {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestNormalizeAccountEmailIsStrict(t *testing.T) {
	for input, want := range map[string]string{
		" Enrico.Varano+work@Example.COM ": "enrico.varano+work@example.com",
		"not-an-email":                     "",
		"name@example":                     "",
		"name@example..com":                "",
		"name @example.com":                "",
		"name@example.com/path":            "",
		"name\u0085@example.com":           "",
	} {
		if got := NormalizeAccountEmail(input); got != want {
			t.Fatalf("NormalizeAccountEmail(%q): got %q want %q", input, got, want)
		}
	}
}

func TestAccountEmailFromAuthFileForHomeReturnsOnlyNormalizedEmail(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(codexHome, "auth.json"),
		[]byte(`{"email":" Person@Example.COM "}`),
		0o600,
	); err != nil {
		t.Fatalf("write synthetic auth identity: %v", err)
	}
	email, err := AccountEmailFromAuthFileForHome(codexHome)
	if err != nil {
		t.Fatalf("recover auth identity: %v", err)
	}
	if email != "person@example.com" {
		t.Fatalf("normalized auth identity: got %q", email)
	}

	if err := os.WriteFile(
		filepath.Join(codexHome, "auth.json"),
		[]byte(`{"email":"not-an-email"}`),
		0o600,
	); err != nil {
		t.Fatalf("replace synthetic auth identity: %v", err)
	}
	email, err = AccountEmailFromAuthFileForHome(codexHome)
	if err != nil {
		t.Fatalf("recover malformed auth identity: %v", err)
	}
	if email != "" {
		t.Fatalf("malformed auth identity escaped normalization: %q", email)
	}
}
