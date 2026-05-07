package cli

import (
	"strings"
	"testing"
)

func TestGuardShellEnv(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		envVar    string
		wantError bool
	}{
		{
			name:      "bash rejected",
			args:      []string{"bash", "-c", "echo $TOKEN"},
			envVar:    "TOKEN",
			wantError: true,
		},
		{
			name:      "sh rejected",
			args:      []string{"/bin/sh"},
			envVar:    "TOKEN",
			wantError: true,
		},
		{
			name:      "pwsh rejected",
			args:      []string{"pwsh"},
			envVar:    "TOKEN",
			wantError: true,
		},
		{
			name:   "non-shell binary allowed",
			args:   []string{"./scripts/deploy"},
			envVar: "TOKEN",
		},
		{
			name:      "docker run without -e rejected",
			args:      []string{"docker", "run", "alpine"},
			envVar:    "TOKEN",
			wantError: true,
		},
		{
			name:   "docker run -e TOKEN scoped allowed",
			args:   []string{"docker", "run", "-e", "TOKEN", "alpine"},
			envVar: "TOKEN",
		},
		{
			name:      "docker run -e OTHER rejected",
			args:      []string{"docker", "run", "-e", "OTHER", "alpine"},
			envVar:    "TOKEN",
			wantError: true,
		},
		{
			name:   "docker run --env=TOKEN scoped allowed",
			args:   []string{"docker", "run", "--env=TOKEN", "alpine"},
			envVar: "TOKEN",
		},
		{
			name:   "docker run --env-file=- allowed",
			args:   []string{"docker", "run", "--env-file=-", "alpine"},
			envVar: "TOKEN",
		},
		{
			name:      "docker run --env-file=other rejected",
			args:      []string{"docker", "run", "--env-file", "all.env", "alpine"},
			envVar:    "TOKEN",
			wantError: true,
		},
		{
			name:   "podman run -e TOKEN scoped allowed",
			args:   []string{"podman", "run", "-e", "TOKEN", "alpine"},
			envVar: "TOKEN",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := guardShellEnv(tc.args, tc.envVar)
			if tc.wantError {
				if err == nil {
					t.Fatalf("guardShellEnv accepted args=%v env=%q", tc.args, tc.envVar)
				}
				if !strings.Contains(err.Error(), "use stdin or --allow-shell-env") {
					t.Fatalf("error = %v, expected guard message", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("guardShellEnv rejected safe args %v: %v", tc.args, err)
			}
		})
	}
}
