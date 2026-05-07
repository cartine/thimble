package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsSecretArgumentAndAcceptsPipe(t *testing.T) {
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	t.Setenv(
		"PATH",
		filepath.Dir(fakeAge)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	storeFlag := []string{"--store", filepath.Join(root, "secrets")}
	var stdout, stderr strings.Builder
	initArgs := append([]string{}, storeFlag...)
	initArgs = append(
		initArgs,
		"init", "web-api", "dev", "--recipient", testRecipientOperator,
	)
	if err := Run(initArgs, &stdout, &stderr); err != nil {
		t.Fatalf("init: %v stderr=%s", err, stderr.String())
	}

	argvSet := append([]string{}, storeFlag...)
	argvSet = append(argvSet, "set", "web-api", "dev", "API_KEY", "unsafe-argv-value")
	err := Run(argvSet, &stdout, &stderr)
	if err == nil {
		t.Fatalf("set with argv value succeeded")
	}
	if !strings.Contains(err.Error(), "do not pass secret values") {
		t.Fatalf("unexpected argv value error: %v", err)
	}

	pipedSet := append([]string{}, storeFlag...)
	pipedSet = append(pipedSet, "set", "web-api", "dev", "API_KEY")
	withStdin(t, "safe piped value\n", func() {
		if err := Run(pipedSet, &stdout, &stderr); err != nil {
			t.Fatalf("piped set: %v stderr=%s", err, stderr.String())
		}
	})

	st := newRawStore(filepath.Join(root, "secrets"))
	rendered, err := st.Render("web-api", "dev")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, "API_KEY=\"safe piped value\"") {
		t.Fatalf("piped value was not stored: %q", rendered)
	}
}

func TestProvisionAndSetAndGetFlow(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("worker", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	root := t.TempDir()
	producer := filepath.Join(root, "producer.sh")
	const producerScript = "#!/bin/sh\nprintf '%s\\n' generated-secret\n"
	if err := os.WriteFile(producer, []byte(producerScript), 0o700); err != nil {
		t.Fatalf("write producer: %v", err)
	}
	var stdout, stderr strings.Builder
	andSet := []string{"worker", "prod", "SERVICE_TOKEN", "--", producer}
	if err := runAndSet(st, andSet, &stdout, &stderr); err != nil {
		t.Fatalf("and-set: %v stderr=%s", err, stderr.String())
	}
	rendered, err := st.Render("worker", "prod")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, "SERVICE_TOKEN=generated-secret") {
		t.Fatalf("and-set value missing: %q", rendered)
	}

	capture := filepath.Join(root, "capture.sh")
	outFile := filepath.Join(root, "captured.txt")
	if err := os.WriteFile(capture, []byte("#!/bin/sh\ncat > \"$1\"\n"), 0o700); err != nil {
		t.Fatalf("write capture: %v", err)
	}
	andGet := []string{"worker", "prod", "SERVICE_TOKEN", "--", capture, outFile}
	if err := runAndGet(st, andGet, &stdout, &stderr); err != nil {
		t.Fatalf("and-get: %v stderr=%s", err, stderr.String())
	}
	captured, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read captured: %v", err)
	}
	if string(captured) != "generated-secret" {
		t.Fatalf("and-get passed %q", captured)
	}
}

func TestProvisionRequiresStrongTokenAndWritesToPipe(t *testing.T) {
	var stderr strings.Builder
	if err := runProvision([]string{"--bytes", "8"}, ioDiscardFile{}, &stderr); err == nil {
		t.Fatalf("weak provision succeeded")
	}

	var stdout strings.Builder
	if err := runProvision([]string{"--bytes", "16"}, &stdout, &stderr); err != nil {
		t.Fatalf("provision to non-terminal writer: %v", err)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatalf("provision wrote no token")
	}
}

type ioDiscardFile struct{}

func (ioDiscardFile) Write(p []byte) (int, error) { return len(p), nil }
