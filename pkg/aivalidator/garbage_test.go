package aivalidator

import (
	"strings"
	"testing"
)

func TestIsGarbage(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "legit english essay ~50 chars",
			text: "I really enjoyed working on this team project a lot.",
			want: false,
		},
		{
			name: "legit indonesian essay ~50 chars",
			text: "Saya senang bekerja dalam tim untuk proyek ini kemarin.",
			want: false,
		},
		{
			name: "legit short-but-real answer just above the floor",
			text: "Jujur saya agak gugup tapi tetap semangat menjalani.",
			want: false,
		},
		{
			name: "legit essay with a couple of emoji",
			text: "This assessment was fun and insightful 😊 I learned a lot about myself 👍",
			want: false,
		},
		{
			name: "empty string",
			text: "",
			want: true,
		},
		{
			name: "whitespace only",
			text: "     ",
			want: true,
		},
		{
			name: "too short legit-looking text",
			text: "Not much to say.",
			want: true,
		},
		{
			name: "repeated single character spam",
			text: strings.Repeat("a", 500),
			want: true,
		},
		{
			name: "repeated syllable spam",
			text: strings.Repeat("haha", 50),
			want: true,
		},
		{
			name: "symbol mash",
			text: strings.Repeat("!@#$%^&*()_+-=1234567890", 5),
			want: true,
		},
		{
			name: "single unbroken keyboard mash word",
			text: "asdfghjklqwertyuiopzxcvbnmasdfghjklqwertyuiop",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGarbage(tt.text); got != tt.want {
				t.Errorf("IsGarbage(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
