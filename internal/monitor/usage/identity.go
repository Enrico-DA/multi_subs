package usage

import (
	"sort"
	"strings"
)

// CodexIdentityRecord contains only the official account fields that can prove
// that two physical Codex homes use one logical subscription.
type CodexIdentityRecord struct {
	AccountID    string
	AccountEmail string
}

// CodexIdentityGroup is one reconciled logical subscription. MemberIndexes
// refer to the input records. Resolved is false when the identity is missing or
// an email-only record could belong to more than one strong account ID.
type CodexIdentityGroup struct {
	MemberIndexes []int
	AccountEmail  string
	Resolved      bool
	Conflicted    bool
}

type normalizedCodexIdentity struct {
	accountID string
	email     string
}

type codexIdentityGroupBuilder struct {
	members  []int
	resolved bool
	conflict bool
}

// ReconcileCodexIdentities groups physical targets by official subscription
// identity. Account ID is strongest. A validated email is used only as a
// fallback, and an email-only target is not attached when that email belongs to
// more than one strong account ID.
func ReconcileCodexIdentities(records []CodexIdentityRecord) []CodexIdentityGroup {
	if len(records) == 0 {
		return nil
	}

	normalized := make([]normalizedCodexIdentity, len(records))
	for index, record := range records {
		normalized[index] = normalizedCodexIdentity{
			accountID: strings.TrimSpace(record.AccountID),
			email:     NormalizeAccountEmail(record.AccountEmail),
		}
	}

	var groups []*codexIdentityGroupBuilder
	strongGroupByID := make(map[string]int)
	for index, identity := range normalized {
		if identity.accountID == "" {
			continue
		}
		groupIndex, exists := strongGroupByID[identity.accountID]
		if !exists {
			groupIndex = len(groups)
			strongGroupByID[identity.accountID] = groupIndex
			groups = append(groups, &codexIdentityGroupBuilder{
				resolved: true,
			})
		}
		groups[groupIndex].members = append(groups[groupIndex].members, index)
	}

	strongGroupsByEmail := make(map[string]map[int]struct{})
	for groupIndex, group := range groups {
		for _, memberIndex := range group.members {
			email := normalized[memberIndex].email
			if email == "" {
				continue
			}
			if strongGroupsByEmail[email] == nil {
				strongGroupsByEmail[email] = make(map[int]struct{})
			}
			strongGroupsByEmail[email][groupIndex] = struct{}{}
		}
	}

	emailOnlyGroup := make(map[string]int)
	for index, identity := range normalized {
		if identity.accountID != "" {
			continue
		}
		if identity.email == "" {
			groups = append(groups, &codexIdentityGroupBuilder{members: []int{index}})
			continue
		}

		strongMatches := strongGroupsByEmail[identity.email]
		switch len(strongMatches) {
		case 0:
			groupIndex, exists := emailOnlyGroup[identity.email]
			if !exists {
				groupIndex = len(groups)
				emailOnlyGroup[identity.email] = groupIndex
				groups = append(groups, &codexIdentityGroupBuilder{resolved: true})
			}
			groups[groupIndex].members = append(groups[groupIndex].members, index)
		case 1:
			for groupIndex := range strongMatches {
				groups[groupIndex].members = append(groups[groupIndex].members, index)
			}
		default:
			groups = append(groups, &codexIdentityGroupBuilder{
				members:  []int{index},
				conflict: true,
			})
		}
	}

	out := make([]CodexIdentityGroup, 0, len(groups))
	for _, builder := range groups {
		sort.Ints(builder.members)
		emails := make(map[string]struct{})
		for _, memberIndex := range builder.members {
			if email := normalized[memberIndex].email; email != "" {
				emails[email] = struct{}{}
			}
		}

		group := CodexIdentityGroup{
			MemberIndexes: append([]int(nil), builder.members...),
			Resolved:      builder.resolved,
			Conflicted:    builder.conflict || len(emails) > 1,
		}
		if len(emails) == 1 && !group.Conflicted {
			for email := range emails {
				group.AccountEmail = email
			}
		}
		out = append(out, group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].MemberIndexes[0] < out[j].MemberIndexes[0]
	})
	return out
}

// NormalizeAccountEmail returns a conservative, normalized mailbox or an empty
// string. Usage output and email fallback grouping use the same validation.
func NormalizeAccountEmail(value string) string {
	email := strings.ToLower(strings.TrimSpace(value))
	if len(email) < 3 || len(email) > 254 || !isPrintableASCII(email) ||
		strings.Count(email, "@") != 1 {
		return ""
	}
	at := strings.IndexByte(email, '@')
	local := email[:at]
	domain := email[at+1:]
	if len(local) == 0 || len(local) > 64 || len(domain) == 0 || len(domain) > 253 ||
		local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") ||
		!validEmailLocalPart(local) || !validEmailDomain(domain) {
		return ""
	}
	return email
}

func isPrintableASCII(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func validEmailLocalPart(local string) bool {
	for index := 0; index < len(local); index++ {
		character := local[index]
		if character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '/', '=', '?',
			'^', '_', '`', '{', '|', '}', '~', '.':
			continue
		default:
			return false
		}
	}
	return true
}

func validEmailDomain(domain string) bool {
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") ||
		strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return false
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 ||
			!isASCIIAlphaNumeric(label[0]) ||
			!isASCIIAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			character := label[index]
			if !isASCIIAlphaNumeric(character) && character != '-' {
				return false
			}
		}
	}
	return len(labels[len(labels)-1]) >= 2
}

func isASCIIAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= '0' && character <= '9'
}
