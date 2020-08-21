package stalecontroller

import "testing"

func TestRemoveKeywordFromWhiteboard(t *testing.T) {
	tests := []struct {
		name string
		wb   string
		want string
	}{
		{"empty", "", ""},
		{"space", " ", ""},
		{"spaces", "   ", ""},
		{"kw", "kw", ""},
		{"trim", "   kw      ", ""},
		{"foo bar", "foo   kw   bar   ", "foo bar"},
		{"kw foo bar", "kw  foo   bar   ", "foo bar"},
		{"foo bar kw", "foo   bar   kw", "foo bar"},
		{"foo kw bla kw bar", "foo  kw bla kw bar", "foo bla bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WithoutKeyword(tt.wb, "kw"); got != tt.want {
				t.Errorf("WithoutKeyword(%q, \"kw\") = %v, want %v", tt.wb, got, tt.want)
			}
		})
	}
}
