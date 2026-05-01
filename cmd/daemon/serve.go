package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ServeRequest is the payload for POST /serve (persistent FastAPI + uvicorn).
type ServeRequest struct {
	Code         string `json:"code"`
	Requirements string `json:"requirements"`
	SetupScript  string `json:"setup_script"`
	EntryType    string `json:"entry_type"`
	EntryClass   string `json:"entry_class"`
	EntryMethod  string `json:"entry_method"`
	EnterMethod  string `json:"enter_method"`
	Port         int    `json:"port"`
	WebMethod    string `json:"web_method"`
	WebPath      string `json:"web_path"`
}

func handleServeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ServeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid body: %v", err), http.StatusBadRequest)
		return
	}
	if req.Port <= 0 {
		req.Port = 9000
	}
	if req.WebPath == "" {
		req.WebPath = "/"
	}
	if req.WebMethod == "" {
		req.WebMethod = "POST"
	}

	serveDir := filepath.Join(codeDir, "skyscale_serve")
	if err := os.MkdirAll(serveDir, 0755); err != nil {
		http.Error(w, fmt.Sprintf("mkdir: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filepath.Join(serveDir, "handler.py"), []byte(req.Code), 0644); err != nil {
		http.Error(w, fmt.Sprintf("write handler: %v", err), http.StatusInternalServerError)
		return
	}

	reqLines := strings.TrimSpace(req.Requirements)
	if reqLines != "" {
		reqLines += "\n"
	}
	reqLines += "fastapi\nuvicorn[standard]\n"
	if err := os.WriteFile(filepath.Join(serveDir, "requirements.txt"), []byte(reqLines), 0644); err != nil {
		http.Error(w, fmt.Sprintf("write requirements: %v", err), http.StatusInternalServerError)
		return
	}

	if strings.TrimSpace(req.SetupScript) != "" {
		cmd := exec.Command("bash", "-c", req.SetupScript)
		cmd.Dir = serveDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if out, err := cmd.CombinedOutput(); err != nil {
			http.Error(w, fmt.Sprintf("setup_script failed: %v\n%s", err, out), http.StatusInternalServerError)
			return
		}
	}

	_, venvUvicorn, err := ensureServeVenv(serveDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("venv/pip: %v", err), http.StatusInternalServerError)
		return
	}

	serverPy := generateServerWrapper(req)
	if err := os.WriteFile(filepath.Join(serveDir, "server.py"), []byte(serverPy), 0644); err != nil {
		http.Error(w, fmt.Sprintf("write server: %v", err), http.StatusInternalServerError)
		return
	}

	srv := exec.Command(venvUvicorn, "server:app",
		"--host", "0.0.0.0",
		"--port", strconv.Itoa(req.Port),
		"--workers", "1")
	srv.Dir = serveDir
	srv.Env = append(os.Environ(), "PATH="+filepath.Join(serveDir, "venv", "bin")+":"+os.Getenv("PATH"))
	srv.Stdout = os.Stdout
	srv.Stderr = os.Stderr
	if err := srv.Start(); err != nil {
		http.Error(w, fmt.Sprintf("start uvicorn: %v", err), http.StatusInternalServerError)
		return
	}

	if !waitForPort(fmt.Sprintf("127.0.0.1:%d", req.Port), 120*time.Second) {
		_ = srv.Process.Kill()
		http.Error(w, "server did not become ready", http.StatusInternalServerError)
		return
	}

	log.Printf("uvicorn ready on port %d (pid %d)", req.Port, srv.Process.Pid)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"port":   req.Port,
		"status": "ready",
	})
}

func ensureServeVenv(serveDir string) (pythonExe, uvicornExe string, err error) {
	venv := filepath.Join(serveDir, "venv")
	pythonExe = filepath.Join(venv, "bin", "python")
	uvicornExe = filepath.Join(venv, "bin", "uvicorn")
	if _, statErr := os.Stat(pythonExe); os.IsNotExist(statErr) {
		cmd := exec.Command("python3", "-m", "venv", venv)
		cmd.Dir = serveDir
		if out, e := cmd.CombinedOutput(); e != nil {
			return "", "", fmt.Errorf("venv: %w: %s", e, out)
		}
	}
	pip := filepath.Join(venv, "bin", "pip")
	install := exec.Command(pip, "install", "-r", filepath.Join(serveDir, "requirements.txt"))
	install.Dir = serveDir
	if out, e := install.CombinedOutput(); e != nil {
		return "", "", fmt.Errorf("pip: %w: %s", e, out)
	}
	return pythonExe, uvicornExe, nil
}

func waitForPort(addr string, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func fastAPIMethodDecorator(method, path string) string {
	m := strings.ToLower(method)
	switch m {
	case "get":
		return fmt.Sprintf("@app.get(%q)", path)
	case "put":
		return fmt.Sprintf("@app.put(%q)", path)
	case "delete":
		return fmt.Sprintf("@app.delete(%q)", path)
	case "patch":
		return fmt.Sprintf("@app.patch(%q)", path)
	default:
		return fmt.Sprintf("@app.post(%q)", path)
	}
}

func generateServerWrapper(req ServeRequest) string {
	path := req.WebPath
	if path == "" {
		path = "/"
	}
	deco := fastAPIMethodDecorator(req.WebMethod, path)
	method := strings.ToLower(req.WebMethod)
	if method == "" {
		method = "post"
	}

	if method == "get" {
		if req.EntryType == "cls" {
			enter := ""
			if strings.TrimSpace(req.EnterMethod) != "" {
				enter = fmt.Sprintf("getattr(_instance, %q)()\n", req.EnterMethod)
			}
			return fmt.Sprintf(`
from fastapi import FastAPI
from handler import %s

app = FastAPI()

_instance = %s()
%s%s
def endpoint():
    return _instance.%s()
`, req.EntryClass, req.EntryClass, enter, deco+"\n", req.EntryMethod)
		}
		return fmt.Sprintf(`
from fastapi import FastAPI
from handler import %s as _user_fn

app = FastAPI()

%s
def endpoint():
    return _user_fn()
`, req.EntryMethod, deco+"\n")
	}

	if req.EntryType == "cls" {
		enter := ""
		if strings.TrimSpace(req.EnterMethod) != "" {
			enter = fmt.Sprintf("getattr(_instance, %q)()\n", req.EnterMethod)
		}
		return fmt.Sprintf(`
from fastapi import FastAPI, Body
from handler import %s

app = FastAPI()

_instance = %s()
%s%s
def endpoint(payload: dict = Body(...)):
    return _instance.%s(**payload)
`, req.EntryClass, req.EntryClass, enter, deco+"\n", req.EntryMethod)
	}

	return fmt.Sprintf(`
from fastapi import FastAPI, Body
from handler import %s as _user_fn

app = FastAPI()

%s
def endpoint(payload: dict = Body(...)):
    return _user_fn(**payload)
`, req.EntryMethod, deco+"\n")
}
