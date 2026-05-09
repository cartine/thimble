package store_test

import (
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/store"
)

func TestValidateRecipient(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
		wantSnip  string
	}{
		{
			name:  "valid age1 62 chars",
			input: "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
		},
		{
			name:  "valid ssh-ed25519",
			input: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICmGOzSrFnp+Bu6CQrK8tZTw0wnH3oJM",
		},
		{
			name:  "valid ssh-ed25519 with comment",
			input: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICmGOzSrFnp+Bu6CQrK8tZTw0wnH3oJM alice@example",
		},
		{
			name:  "valid ssh-rsa",
			input: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC8s2YlpqXpC9LnG1IWVMfH operator",
		},
		{
			name:  "trailing newline trimmed",
			input: "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq\n",
		},
		{
			name:      "malformed age0 prefix rejected",
			input:     "age0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
			wantError: true,
			wantSnip:  "expected `age1...`",
		},
		{
			name:      "random string rejected",
			input:     "totally-not-a-recipient",
			wantError: true,
			wantSnip:  "expected `age1...`",
		},
		{
			name:      "leading dash rejected",
			input:     "-age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
			wantError: true,
			wantSnip:  "expected `age1...`",
		},
		{
			name:      "age1 too short",
			input:     "age1abc",
			wantError: true,
		},
		{
			name:      "age1 invalid bech32 char b",
			input:     "age1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			wantError: true,
		},
		{
			name:      "empty rejected",
			input:     "",
			wantError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.ValidateRecipient(tc.input)
			if tc.wantError {
				if err == nil {
					t.Fatalf("ValidateRecipient(%q) = nil, want error", tc.input)
				}
				if tc.wantSnip != "" && !strings.Contains(err.Error(), tc.wantSnip) {
					t.Fatalf("error %q missing %q", err.Error(), tc.wantSnip)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateRecipient(%q) = %v, want nil", tc.input, err)
			}
		})
	}
}
