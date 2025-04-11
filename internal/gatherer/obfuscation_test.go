//go:build test || unit

package gatherer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObfuscateName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple name",
			input:    "my-cluster-name",
			expected: "880d9279006f73a32536d549cc3cfd617c42a023e16eaecabf2971aaf7b01676",
		},
		{
			name:     "Another name",
			input:    "production-us-east-1",
			expected: "583d6af8436c7e65185372ff1b0e57161bacc891e9d242f820577b18e8052639",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Name with special chars",
			input:    "cluster_1/test@",
			expected: "bab41a9bdf7aeb4cb4cc6ad55f75838fb92e19b229f00553cb787ccd759b28ad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ObfuscateName(tt.input)

			// Basic assertion: Ensure it's empty for empty input
			if tt.input == "" {
				assert.Equal(t, "", actual)
				return
			}

			assert.NotEqual(t, tt.input, actual)

			// The expected is the full hex encoded hash, so we take the first 32 characters (16 bytes from the hash, which is hex-encoded, so 32 characters)
			expectedSplice := tt.expected[:32]

			assert.Equal(t, expectedSplice, actual)
		})
	}
}
