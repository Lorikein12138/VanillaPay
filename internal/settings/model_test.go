package settings

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestUnit_SecurityEmailDomainRules(t *testing.T) {
	for _, tc := range []struct {
		name          string
		domains, want []string
	}{
		{"wildcard rules and duplicates", []string{"qq.com", "*.EDU.CN", "*.edu.cn", " mail.*.com ", "*.com", "QQ.COM"}, []string{"qq.com", "*.edu.cn", "mail.*.com", "*.com"}},
		{"unrestricted wildcard", []string{"*"}, nil},
		{"repeated unrestricted wildcard", []string{"**"}, nil},
		{"slash", []string{"foo/bar.com"}, nil},
		{"space", []string{"foo bar.com"}, nil},
		{"colon", []string{"foo:bar.com"}, nil},
		{"at", []string{"foo@bar.com"}, nil},
		{"consecutive dots", []string{"*..com"}, nil},
		{"leading dot", []string{".*.com"}, nil},
		{"trailing dot", []string{"*.com."}, nil},
		{"rule too long", []string{strings.Repeat("a", 254)}, nil},
		{"too many rules", slices.Repeat([]string{"example.com"}, 101), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := marshal(SecurityGroupRequest{RestrictEmailDomains: true, EmailDomains: &tc.domains})
			saved, _, err := ValidateGroup("security", value)
			if tc.want == nil {
				if err == nil {
					t.Fatal("invalid domain rules were accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result SecurityGroupRequest
			if err := json.Unmarshal(saved, &result); err != nil || result.EmailDomains == nil || !slices.Equal(*result.EmailDomains, tc.want) {
				t.Fatalf("normalized security value = %s, err=%v, want domains=%v", saved, err, tc.want)
			}
		})
	}
}

func TestUnit_HomeHTMLModeRequiresContent(t *testing.T) {
	t.Run("empty_html_is_rejected", func(t *testing.T) {
		if _, _, err := ValidateGroup("home", json.RawMessage(`{"mode":"html","html":"","url":null}`)); err == nil {
			t.Fatal("empty HTML content was accepted")
		}
	})

	t.Run("missing_html_is_rejected", func(t *testing.T) {
		if _, _, err := ValidateGroup("home", json.RawMessage(`{"mode":"html","html":null,"url":null}`)); err == nil {
			t.Fatal("missing HTML content was accepted")
		}
	})

	t.Run("nonempty_html_is_accepted", func(t *testing.T) {
		if _, _, err := ValidateGroup("home", json.RawMessage(`{"mode":"html","html":"<main>home</main>","url":null}`)); err != nil {
			t.Fatalf("nonempty HTML content was rejected: %v", err)
		}
	})
}
