//go:build integration

// Package e2e contains end-to-end integration tests for the Skyscale sandbox API.
//
// These tests require a running Skyscale control plane and real Firecracker VMs.
// Run with:
//
//	SKYSCALE_URL=http://localhost:8080 go test -tags=integration ./tests/e2e/... -v -timeout 300s
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

var baseURL string

func init() {
	baseURL = os.Getenv("SKYSCALE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func createSandbox(t *testing.T, runtime string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"runtime":     runtime,
		"ttl_seconds": 120,
	})
	resp, err := http.Post(baseURL+"/api/sandboxes", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var sb map[string]any
	json.NewDecoder(resp.Body).Decode(&sb)
	return sb["id"].(string)
}

func execCode(t *testing.T, sandboxID, code, language string, timeout int) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"code":            code,
		"language":        language,
		"timeout_seconds": timeout,
	})
	resp, err := http.Post(
		fmt.Sprintf("%s/api/sandboxes/%s/exec", baseURL, sandboxID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exec returned %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func destroySandbox(t *testing.T, sandboxID string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/sandboxes/%s", baseURL, sandboxID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("destroy sandbox warning: %v", err)
		return
	}
	resp.Body.Close()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestFullSandboxLifecycle(t *testing.T) {
	id := createSandbox(t, "python3")
	defer destroySandbox(t, id)

	// 1. Run Python and get stdout
	r1 := execCode(t, id, `print("lifecycle test")`, "python3", 15)
	if exitCode, _ := r1["exit_code"].(float64); exitCode != 0 {
		t.Errorf("expected exit 0, got %v (stderr: %v)", exitCode, r1["stderr"])
	}
	if stdout, _ := r1["stdout"].(string); !strings.Contains(stdout, "lifecycle test") {
		t.Errorf("unexpected stdout: %q", stdout)
	}

	// 2. Write a file in one exec
	r2 := execCode(t, id, `open("lifecycle.txt","w").write("persistent42")`, "python3", 15)
	if exitCode, _ := r2["exit_code"].(float64); exitCode != 0 {
		t.Fatalf("write file exec failed: exit=%v, stderr=%v", r2["exit_code"], r2["stderr"])
	}

	// 3. Read the file back in a subsequent exec (same sandbox = same VM = same workspace)
	r3 := execCode(t, id, `print(open("lifecycle.txt").read())`, "python3", 15)
	if exitCode, _ := r3["exit_code"].(float64); exitCode != 0 {
		t.Fatalf("read file exec failed: exit=%v, stderr=%v", r3["exit_code"], r3["stderr"])
	}
	if stdout, _ := r3["stdout"].(string); !strings.Contains(stdout, "persistent42") {
		t.Errorf("expected 'persistent42' in stdout, got %q", stdout)
	}

	// 4. Bash exec
	r4 := execCode(t, id, `echo "bash works"`, "bash", 10)
	if exitCode, _ := r4["exit_code"].(float64); exitCode != 0 {
		t.Errorf("bash exec failed: exit=%v, stderr=%v", r4["exit_code"], r4["stderr"])
	}

	// 5. Destroy sandbox
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/sandboxes/%s", baseURL, id), nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 on destroy, got %d", resp.StatusCode)
	}

	// 6. GET after destroy should still return 200 (sandbox record exists with status=destroyed)
	getResp, _ := http.Get(fmt.Sprintf("%s/api/sandboxes/%s", baseURL, id))
	getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for get-after-destroy, got %d", getResp.StatusCode)
	}
}

func TestFileUploadDownload(t *testing.T) {
	id := createSandbox(t, "python3")
	defer destroySandbox(t, id)

	content := []byte("binary content: \x00\x01\x02\x03")

	// Upload
	uploadResp, err := http.Post(
		fmt.Sprintf("%s/api/sandboxes/%s/files/test_binary.bin", baseURL, id),
		"application/octet-stream",
		bytes.NewReader(content),
	)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("upload returned %d", uploadResp.StatusCode)
	}

	// Download
	downloadResp, err := http.Get(fmt.Sprintf("%s/api/sandboxes/%s/files/test_binary.bin", baseURL, id))
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("download returned %d", downloadResp.StatusCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(downloadResp.Body)
	if !bytes.Equal(buf.Bytes(), content) {
		t.Errorf("downloaded content mismatch")
	}

	// Verify the file is accessible via exec
	r := execCode(t, id, `import os; print(os.path.exists('test_binary.bin'))`, "python3", 10)
	if stdout, _ := r["stdout"].(string); !strings.Contains(stdout, "True") {
		t.Errorf("expected True for file existence check, got %q", stdout)
	}
}

func TestSandboxIsolation(t *testing.T) {
	// Two sandboxes should not share filesystems
	idA := createSandbox(t, "python3")
	idB := createSandbox(t, "python3")
	defer destroySandbox(t, idA)
	defer destroySandbox(t, idB)

	// Write secret in sandbox A
	execCode(t, idA, `open("isolation_secret.txt","w").write("sandbox_A_secret")`, "python3", 10)

	// Sandbox B should NOT be able to read sandbox A's file
	rB := execCode(t, idB, `import os; print(os.path.exists("isolation_secret.txt"))`, "python3", 10)
	if stdout, _ := rB["stdout"].(string); !strings.Contains(stdout, "False") {
		t.Errorf("sandbox isolation violation: sandbox B can see sandbox A's file, stdout=%q", stdout)
	}
}

func TestSandboxTTLCleanup(t *testing.T) {
	// Create a sandbox with a very short TTL via direct API call
	body, _ := json.Marshal(map[string]any{
		"runtime":     "python3",
		"ttl_seconds": 5, // 5-second TTL
	})
	resp, _ := http.Post(baseURL+"/api/sandboxes", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	var sb map[string]any
	json.NewDecoder(resp.Body).Decode(&sb)
	id := sb["id"].(string)

	// Wait for TTL + GC sweep interval (30s) + buffer
	t.Logf("Waiting 40s for TTL expiry and GC sweep...")
	time.Sleep(40 * time.Second)

	// Status should be "destroyed"
	getResp, _ := http.Get(fmt.Sprintf("%s/api/sandboxes/%s", baseURL, id))
	defer getResp.Body.Close()
	var got map[string]any
	json.NewDecoder(getResp.Body).Decode(&got)
	if status, _ := got["status"].(string); status != "destroyed" {
		t.Errorf("expected status 'destroyed' after TTL expiry, got %q", status)
	}
}

func TestConcurrentSandboxes(t *testing.T) {
	const n = 3
	ids := make([]string, n)
	for i := range ids {
		ids[i] = createSandbox(t, "python3")
	}
	defer func() {
		for _, id := range ids {
			destroySandbox(t, id)
		}
	}()

	var wg sync.WaitGroup
	results := make([]map[string]any, n)
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, sandboxID string) {
			defer wg.Done()
			code := fmt.Sprintf("print('sandbox_%d')", idx)
			results[idx] = execCode(t, sandboxID, code, "python3", 15)
		}(i, id)
	}
	wg.Wait()

	for i, r := range results {
		if exitCode, _ := r["exit_code"].(float64); exitCode != 0 {
			t.Errorf("sandbox %d: non-zero exit %v", i, exitCode)
		}
		expected := fmt.Sprintf("sandbox_%d", i)
		if stdout, _ := r["stdout"].(string); !strings.Contains(stdout, expected) {
			t.Errorf("sandbox %d: expected %q in stdout, got %q", i, expected, stdout)
		}
	}
}
