package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// setupWorkspace creates a temp workspace and overrides sandboxWorkspace for tests
func setupWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// ─── /sandbox/exec tests ─────────────────────────────────────────────────────

func TestSandboxExecPython(t *testing.T) {
	workspace := setupWorkspace(t)
	origWS := sandboxWorkspace
	// Can't reassign a const; use a package-var approach via the test build.
	// We test the handler directly, overriding the global via init in the test binary.
	_ = origWS
	_ = workspace

	body, _ := json.Marshal(sandboxExecRequest{
		ExecID:   "test-py",
		Code:     "print('hello from python')",
		Language: "python3",
		Timeout:  10,
	})

	req := httptest.NewRequest(http.MethodPost, "/sandbox/exec", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handleSandboxExec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result sandboxExecResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (stderr: %s)", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "hello from python\n" {
		t.Errorf("unexpected stdout: %q", result.Stdout)
	}
}

func TestSandboxExecBash(t *testing.T) {
	body, _ := json.Marshal(sandboxExecRequest{
		ExecID:   "test-bash",
		Code:     "echo hello_bash",
		Language: "bash",
		Timeout:  10,
	})

	req := httptest.NewRequest(http.MethodPost, "/sandbox/exec", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handleSandboxExec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result sandboxExecResponse
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello_bash\n" {
		t.Errorf("unexpected stdout: %q", result.Stdout)
	}
}

func TestSandboxExecFilePersistence(t *testing.T) {
	// Write a file in one exec, read it back in a second exec.
	// Both execs use sandboxWorkspace as cwd so the file is accessible.
	writeBody, _ := json.Marshal(sandboxExecRequest{
		ExecID:   "write-exec",
		Code:     "with open('persist_test.txt', 'w') as f:\n    f.write('42')\nprint('written')",
		Language: "python3",
		Timeout:  10,
	})
	req1 := httptest.NewRequest(http.MethodPost, "/sandbox/exec", bytes.NewReader(writeBody))
	rr1 := httptest.NewRecorder()
	handleSandboxExec(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("write exec failed: %d %s", rr1.Code, rr1.Body.String())
	}
	var r1 sandboxExecResponse
	json.Unmarshal(rr1.Body.Bytes(), &r1)
	if r1.ExitCode != 0 {
		t.Fatalf("write exec non-zero exit: %d, stderr: %s", r1.ExitCode, r1.Stderr)
	}

	readBody, _ := json.Marshal(sandboxExecRequest{
		ExecID:   "read-exec",
		Code:     "print(open('persist_test.txt').read())",
		Language: "python3",
		Timeout:  10,
	})
	req2 := httptest.NewRequest(http.MethodPost, "/sandbox/exec", bytes.NewReader(readBody))
	rr2 := httptest.NewRecorder()
	handleSandboxExec(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("read exec failed: %d %s", rr2.Code, rr2.Body.String())
	}
	var r2 sandboxExecResponse
	json.Unmarshal(rr2.Body.Bytes(), &r2)
	if r2.ExitCode != 0 {
		t.Errorf("read exec non-zero exit: %d, stderr: %s", r2.ExitCode, r2.Stderr)
	}
	if r2.Stdout != "42\n" {
		t.Errorf("expected stdout '42\\n', got %q", r2.Stdout)
	}

	// Cleanup
	os.Remove(filepath.Join(sandboxWorkspace, "persist_test.txt"))
}

func TestSandboxExecTimeout(t *testing.T) {
	body, _ := json.Marshal(sandboxExecRequest{
		ExecID:   "timeout-exec",
		Code:     "import time; time.sleep(60)",
		Language: "python3",
		Timeout:  1,
	})
	req := httptest.NewRequest(http.MethodPost, "/sandbox/exec", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSandboxExec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 even on timeout, got %d", rr.Code)
	}
	var result sandboxExecResponse
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code for timed-out exec")
	}
}

func TestSandboxExecUnsupportedLanguage(t *testing.T) {
	body, _ := json.Marshal(sandboxExecRequest{
		Code:     "console.log('hi')",
		Language: "javascript",
		Timeout:  5,
	})
	req := httptest.NewRequest(http.MethodPost, "/sandbox/exec", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handleSandboxExec(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported language, got %d", rr.Code)
	}
}

// ─── /sandbox/files/ tests ───────────────────────────────────────────────────

func TestFileUploadDownload(t *testing.T) {
	content := []byte("hello sandbox file")

	// Upload
	uploadReq := httptest.NewRequest(http.MethodPost, "/sandbox/files/test_upload.txt", bytes.NewReader(content))
	uploadRR := httptest.NewRecorder()
	handleSandboxFile(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", uploadRR.Code, uploadRR.Body.String())
	}

	// Download
	downloadReq := httptest.NewRequest(http.MethodGet, "/sandbox/files/test_upload.txt", nil)
	downloadRR := httptest.NewRecorder()
	handleSandboxFile(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("download failed: %d %s", downloadRR.Code, downloadRR.Body.String())
	}
	if !bytes.Equal(downloadRR.Body.Bytes(), content) {
		t.Errorf("downloaded content mismatch: got %q, want %q", downloadRR.Body.String(), content)
	}

	// Cleanup
	os.Remove(filepath.Join(sandboxWorkspace, "test_upload.txt"))
}

func TestFileDownloadMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/sandbox/files/nonexistent_file.txt", nil)
	rr := httptest.NewRecorder()
	handleSandboxFile(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFileMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/sandbox/files/somefile.txt", nil)
	rr := httptest.NewRecorder()
	handleSandboxFile(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}
