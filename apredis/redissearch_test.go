package apredis

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEscapeRedisSearchString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "url",
			input: "https://www.example.com/some-path?query=123&other=456",
			want:  "https\\://www\\.example\\.com/some\\-path?query\\=123\\&other\\=456",
		},
		{
			name:  "underscores not escaped",
			input: "hello_world",
			want:  "hello_world",
		},
		{
			name:  "wildcard",
			input: "hello*",
			want:  "hello\\*",
		},
		{
			name:  "bobby tables",
			input: "malicious) | (@title:*)",
			want:  "malicious\\) | \\(\\@title\\:\\*\\)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, EscapeRedisSearchString(tt.input))
		})
	}
}

func TestEscapeRedisSearchStringAllowWildcards(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "url",
			input: "https://www.example.com/some-path?query=123&other=456",
			want:  "https\\://www\\.example\\.com/some\\-path?query\\=123\\&other\\=456",
		},
		{
			name:  "underscores not escaped",
			input: "hello_world",
			want:  "hello_world",
		},
		{
			name:  "wildcard",
			input: "hello*",
			want:  "hello*",
		},
		{
			name:  "bobby tables",
			input: "malicious) | (@title:*)",
			want:  "malicious\\) | \\(\\@title\\:*\\)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, EscapeRedisSearchStringAllowWildcards(tt.input))
		})
	}
}
