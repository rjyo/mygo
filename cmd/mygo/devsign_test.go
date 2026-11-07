package main

import "testing"

func TestDevelopmentIdentity(t *testing.T) {
	out := `  1) 0000000000000000000000000000000000000001 "Developer ID Application: Jane Doe (AAAA)"
  2) 0000000000000000000000000000000000000002 "Mac Developer: Jane Doe (OLD)"
  3) 0000000000000000000000000000000000000003 "Apple Development: jane@example.com (BBBB)"
  4) 0000000000000000000000000000000000000004 "Apple Development: Jane Doe (AAAA)"
     4 valid identities found
`
	for _, tt := range []struct {
		out, team, hash, name string
	}{
		{out, "", "0000000000000000000000000000000000000003", "Apple Development: jane@example.com (BBBB)"},
		{out, "AAAA", "0000000000000000000000000000000000000004", "Apple Development: Jane Doe (AAAA)"},
		{out, "OLD", "0000000000000000000000000000000000000002", "Mac Developer: Jane Doe (OLD)"},
		{out, "CCCC", "0000000000000000000000000000000000000003", "Apple Development: jane@example.com (BBBB)"},
		{`  1) 0000000000000000000000000000000000000002 "Mac Developer: Jane Doe (OLD)"`, "", "0000000000000000000000000000000000000002", "Mac Developer: Jane Doe (OLD)"},
		{`  1) 0000000000000000000000000000000000000001 "Developer ID Application: Jane Doe (AAAA)"`, "AAAA", "", ""},
		{"     0 valid identities found\n", "", "", ""},
	} {
		if hash, name := developmentIdentity(tt.out, tt.team); hash != tt.hash || name != tt.name {
			t.Errorf("developmentIdentity(%q, %q) = %q, %q; want %q, %q", tt.out, tt.team, hash, name, tt.hash, tt.name)
		}
	}
}

func TestSigningTeam(t *testing.T) {
	for in, want := range map[string]string{
		"Developer ID Application: Jane Doe (AAAA)": "AAAA",
		"Apple Development: Jane (Work) (BBBB)":     "BBBB",
		"-":                                         "",
		"":                                          "",
		"0000000000000000000000000000000000000001": "",
	} {
		if got := signingTeam(in); got != want {
			t.Errorf("signingTeam(%q) = %q, want %q", in, got, want)
		}
	}
}
