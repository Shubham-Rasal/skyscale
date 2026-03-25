"""Unit tests for the Skyscale Sandbox SDK client."""

import json
from unittest.mock import MagicMock, patch

import pytest

from skyscale_sandbox import SandboxClient, Sandbox, ExecResult
from skyscale_sandbox.client import SandboxError


def make_client() -> SandboxClient:
    return SandboxClient("http://localhost:8080", api_key="test-key")


def mock_sandbox_data(sandbox_id="sb-1", status="ready", runtime="python3") -> dict:
    return {
        "id": sandbox_id,
        "status": status,
        "runtime": runtime,
        "vm_ip": "192.168.1.2",
        "created_at": "2026-01-01T00:00:00Z",
        "expires_at": "2026-01-01T01:00:00Z",
    }


# ─── SandboxClient.create ─────────────────────────────────────────────────────

def test_create_sandbox():
    client = make_client()
    mock_resp = MagicMock()
    mock_resp.status_code = 201
    mock_resp.json.return_value = mock_sandbox_data()

    with patch("requests.post", return_value=mock_resp) as mock_post:
        sb = client.create(runtime="python3", ttl=300)

    assert sb.id == "sb-1"
    assert sb.status == "ready"
    assert sb.runtime == "python3"
    called_url = mock_post.call_args[0][0]
    assert "/api/sandboxes" in called_url


def test_create_sandbox_failure():
    client = make_client()
    mock_resp = MagicMock()
    mock_resp.status_code = 500
    mock_resp.text = "internal error"

    with patch("requests.post", return_value=mock_resp):
        with pytest.raises(SandboxError, match="500"):
            client.create()


# ─── Sandbox.exec ─────────────────────────────────────────────────────────────

def test_exec_success():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {
        "exec_id": "e1",
        "stdout": "hello\n",
        "stderr": "",
        "exit_code": 0,
        "duration_ms": 42,
    }

    with patch("requests.post", return_value=mock_resp):
        result = sb.exec("print('hello')")

    assert isinstance(result, ExecResult)
    assert result.stdout == "hello\n"
    assert result.exit_code == 0
    assert result.success is True
    assert result.duration_ms == 42


def test_exec_nonzero_exit():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = {
        "exec_id": "e2",
        "stdout": "",
        "stderr": "NameError: name 'x' is not defined",
        "exit_code": 1,
        "duration_ms": 10,
    }

    with patch("requests.post", return_value=mock_resp):
        result = sb.exec("print(x)")

    assert result.exit_code == 1
    assert result.success is False
    assert "NameError" in result.stderr


def test_exec_on_destroyed_sandbox():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 404
    mock_resp.text = "Sandbox has been destroyed"

    with patch("requests.post", return_value=mock_resp):
        with pytest.raises(SandboxError, match="not found or destroyed"):
            sb.exec("print('hi')")


def test_exec_busy_sandbox():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 409
    mock_resp.text = "busy"

    with patch("requests.post", return_value=mock_resp):
        with pytest.raises(SandboxError, match="busy"):
            sb.exec("import time; time.sleep(10)")


# ─── Sandbox.upload_file / download_file ─────────────────────────────────────

def test_upload_file():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 200

    with patch("requests.post", return_value=mock_resp) as mock_post:
        sb.upload_file("data.csv", b"col1,col2\n1,2\n")

    called_url = mock_post.call_args[0][0]
    assert "data.csv" in called_url


def test_download_file():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.content = b"file content"

    with patch("requests.get", return_value=mock_resp):
        content = sb.download_file("output.txt")

    assert content == b"file content"


def test_download_file_not_found():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 404

    with patch("requests.get", return_value=mock_resp):
        with pytest.raises(SandboxError, match="not found"):
            sb.download_file("missing.txt")


# ─── Sandbox.destroy ─────────────────────────────────────────────────────────

def test_destroy():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 204

    with patch("requests.delete", return_value=mock_resp):
        sb.destroy()

    assert sb.status == "destroyed"


def test_destroy_idempotent():
    client = make_client()
    sb = Sandbox(mock_sandbox_data(status="destroyed"), client)

    mock_resp = MagicMock()
    mock_resp.status_code = 404  # already gone on server

    with patch("requests.delete", return_value=mock_resp):
        sb.destroy()  # should not raise

    assert sb.status == "destroyed"


# ─── Context manager ─────────────────────────────────────────────────────────

def test_context_manager():
    client = make_client()

    create_resp = MagicMock()
    create_resp.status_code = 201
    create_resp.json.return_value = mock_sandbox_data()

    destroy_resp = MagicMock()
    destroy_resp.status_code = 204

    with patch("requests.post", return_value=create_resp):
        sb = client.create()

    with patch("requests.delete", return_value=destroy_resp):
        with sb:
            assert sb.status == "ready"

    assert sb.status == "destroyed"


# ─── SandboxClient.list ───────────────────────────────────────────────────────

def test_list_sandboxes():
    client = make_client()

    mock_resp = MagicMock()
    mock_resp.status_code = 200
    mock_resp.json.return_value = [
        mock_sandbox_data("sb-1"),
        mock_sandbox_data("sb-2"),
    ]
    mock_resp.raise_for_status = MagicMock()

    with patch("requests.get", return_value=mock_resp):
        sandboxes = client.list()

    assert len(sandboxes) == 2
    assert sandboxes[0].id == "sb-1"
    assert sandboxes[1].id == "sb-2"
