"""
Skyscale Sandbox SDK

Provides a simple interface for AI agents to create isolated execution
environments (sandboxes) backed by Firecracker microVMs.

Usage:
    from skyscale_sandbox import SandboxClient

    client = SandboxClient("http://skyscale:8080")

    # Context-manager style (auto-destroys on exit)
    with client.create(runtime="python3") as sb:
        result = sb.exec("print('hello')")
        print(result.stdout)

        sb.upload_file("data.csv", open("data.csv", "rb").read())
        result = sb.exec("import pandas as pd; print(pd.read_csv('data.csv').shape)")
        print(result.stdout)

    # Manual lifecycle
    sb = client.create(runtime="bash", ttl=600)
    sb.exec("echo hello")
    sb.destroy()
"""

from __future__ import annotations

import io
from dataclasses import dataclass
from typing import Optional, Union

import requests


@dataclass
class ExecResult:
    """Result of a code execution inside a sandbox."""

    exec_id: str
    stdout: str
    stderr: str
    exit_code: int
    duration_ms: int

    @property
    def success(self) -> bool:
        return self.exit_code == 0

    def __repr__(self) -> str:
        return (
            f"ExecResult(exit_code={self.exit_code}, "
            f"stdout={self.stdout!r}, "
            f"stderr={self.stderr!r})"
        )


class SandboxError(Exception):
    """Raised when a sandbox operation fails."""


class Sandbox:
    """
    A long-lived isolated execution environment backed by a Firecracker microVM.

    Use as a context manager for automatic cleanup:
        with client.create() as sb:
            sb.exec("print('hi')")
    """

    def __init__(self, data: dict, client: "SandboxClient") -> None:
        self.id: str = data["id"]
        self.status: str = data["status"]
        self.runtime: str = data["runtime"]
        self.vm_ip: str = data.get("vm_ip", "")
        self.created_at: str = data.get("created_at", "")
        self.expires_at: str = data.get("expires_at", "")
        self._client = client

    def exec(
        self,
        code: str,
        language: Optional[str] = None,
        timeout: int = 30,
    ) -> ExecResult:
        """
        Execute code inside the sandbox and return the result.

        Args:
            code:     Source code to run.
            language: Override the sandbox's default runtime ("python3" or "bash").
            timeout:  Execution timeout in seconds (default: 30).

        Returns:
            ExecResult with stdout, stderr, exit_code, and duration_ms.

        Raises:
            SandboxError: If the sandbox is destroyed or the request fails.
        """
        payload: dict = {"code": code, "timeout_seconds": timeout}
        if language:
            payload["language"] = language

        resp = self._client._post(f"/api/sandboxes/{self.id}/exec", payload)
        if resp.status_code == 409:
            raise SandboxError("Sandbox is busy (concurrent exec rejected)")
        if resp.status_code == 404:
            raise SandboxError(f"Sandbox {self.id} not found or destroyed")
        if resp.status_code != 200:
            raise SandboxError(f"exec failed: HTTP {resp.status_code}: {resp.text}")

        data = resp.json()
        return ExecResult(
            exec_id=data.get("exec_id", ""),
            stdout=data.get("stdout", ""),
            stderr=data.get("stderr", ""),
            exit_code=int(data.get("exit_code", -1)),
            duration_ms=int(data.get("duration_ms", 0)),
        )

    def upload_file(self, path: str, content: Union[bytes, io.IOBase]) -> None:
        """
        Upload a file to the sandbox workspace.

        Args:
            path:    Relative path inside /sandbox/workspace (e.g. "data/input.csv").
            content: File bytes or a file-like object.

        Raises:
            SandboxError: If the upload fails.
        """
        if hasattr(content, "read"):
            content = content.read()

        url = f"{self._client.base_url}/api/sandboxes/{self.id}/files/{path}"
        resp = requests.post(url, data=content, headers=self._client._headers())
        if resp.status_code != 200:
            raise SandboxError(f"upload_file failed: HTTP {resp.status_code}: {resp.text}")

    def download_file(self, path: str) -> bytes:
        """
        Download a file from the sandbox workspace.

        Args:
            path: Relative path inside /sandbox/workspace.

        Returns:
            File contents as bytes.

        Raises:
            SandboxError: If the file is not found or download fails.
        """
        url = f"{self._client.base_url}/api/sandboxes/{self.id}/files/{path}"
        resp = requests.get(url, headers=self._client._headers())
        if resp.status_code == 404:
            raise SandboxError(f"File not found: {path}")
        if resp.status_code != 200:
            raise SandboxError(f"download_file failed: HTTP {resp.status_code}: {resp.text}")
        return resp.content

    def refresh(self) -> "Sandbox":
        """Refresh sandbox metadata from the control plane."""
        resp = requests.get(
            f"{self._client.base_url}/api/sandboxes/{self.id}",
            headers=self._client._headers(),
        )
        if resp.status_code == 404:
            raise SandboxError(f"Sandbox {self.id} not found")
        resp.raise_for_status()
        data = resp.json()
        self.status = data.get("status", self.status)
        return self

    def destroy(self) -> None:
        """Terminate the sandbox and its underlying VM."""
        resp = requests.delete(
            f"{self._client.base_url}/api/sandboxes/{self.id}",
            headers=self._client._headers(),
        )
        # 204 = success, 404 = already gone — both are acceptable
        if resp.status_code not in (204, 404):
            raise SandboxError(f"destroy failed: HTTP {resp.status_code}: {resp.text}")
        self.status = "destroyed"

    # Context-manager support
    def __enter__(self) -> "Sandbox":
        return self

    def __exit__(self, *_) -> None:
        self.destroy()

    def __repr__(self) -> str:
        return f"Sandbox(id={self.id!r}, status={self.status!r}, runtime={self.runtime!r})"


class SandboxClient:
    """
    Client for the Skyscale Sandbox API.

    Args:
        base_url: Base URL of the Skyscale control plane (e.g. "http://localhost:8080").
        api_key:  Optional API key sent as the X-API-Key header.
        timeout:  Default HTTP request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        timeout: int = 30,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout

    def _headers(self) -> dict:
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["X-API-Key"] = self.api_key
        return headers

    def _post(self, path: str, payload: dict) -> requests.Response:
        return requests.post(
            self.base_url + path,
            json=payload,
            headers=self._headers(),
            timeout=self.timeout,
        )

    def create(
        self,
        runtime: str = "python3",
        ttl: int = 3600,
        metadata: Optional[dict] = None,
    ) -> Sandbox:
        """
        Create a new sandbox.

        Args:
            runtime:  Execution runtime ("python3" or "bash"). Default: "python3".
            ttl:      Sandbox lifetime in seconds. Default: 3600 (1 hour).
            metadata: Optional key/value tags stored with the sandbox.

        Returns:
            A Sandbox instance ready for use.

        Raises:
            SandboxError: If sandbox creation fails.
        """
        payload: dict = {"runtime": runtime, "ttl_seconds": ttl}
        if metadata:
            payload["metadata"] = metadata

        resp = self._post("/api/sandboxes", payload)
        if resp.status_code != 201:
            raise SandboxError(f"create sandbox failed: HTTP {resp.status_code}: {resp.text}")
        return Sandbox(resp.json(), self)

    def get(self, sandbox_id: str) -> Sandbox:
        """Retrieve an existing sandbox by ID."""
        resp = requests.get(
            f"{self.base_url}/api/sandboxes/{sandbox_id}",
            headers=self._headers(),
            timeout=self.timeout,
        )
        if resp.status_code == 404:
            raise SandboxError(f"Sandbox {sandbox_id} not found")
        resp.raise_for_status()
        return Sandbox(resp.json(), self)

    def list(self) -> list[Sandbox]:
        """List all sandboxes."""
        resp = requests.get(
            f"{self.base_url}/api/sandboxes",
            headers=self._headers(),
            timeout=self.timeout,
        )
        resp.raise_for_status()
        return [Sandbox(item, self) for item in resp.json()]
