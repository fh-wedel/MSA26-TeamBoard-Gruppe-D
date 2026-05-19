"""End-to-end smoke test for TeamBoard.

Run after `docker compose up`:
    python tests/smoke_test.py [base_url]

Default base_url: http://localhost:8080
Requires: httpx (`pip install httpx`)
"""
import io
import os
import sys
import time
import uuid

import httpx


def main(base_url: str) -> None:
    suffix = uuid.uuid4().hex[:8]
    email = f"alice+{suffix}@example.com"
    password = "secret123"

    print(f"[1] Register {email}")
    r = httpx.post(
        f"{base_url}/api/auth/register",
        json={"email": email, "password": password, "display_name": "Alice"},
        timeout=10.0,
    )
    r.raise_for_status()
    user = r.json()
    print(f"    -> user id {user['id']}")

    print("[2] Login")
    r = httpx.post(
        f"{base_url}/api/auth/login",
        json={"email": email, "password": password},
        timeout=10.0,
    )
    r.raise_for_status()
    token = r.json()["access_token"]
    headers = {"Authorization": f"Bearer {token}"}

    print("[3] Create project")
    r = httpx.post(
        f"{base_url}/api/projects/projects",
        json={"name": "Demo", "description": "smoke test"},
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    project = r.json()
    project_id = project["id"]
    print(f"    -> project id {project_id}")

    print("[4] Create board (default Kanban columns)")
    r = httpx.post(
        f"{base_url}/api/projects/projects/{project_id}/boards",
        json={"name": "Sprint 1", "type": "kanban"},
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    board = r.json()
    board_id = board["id"]

    r = httpx.get(
        f"{base_url}/api/projects/boards/{board_id}/columns",
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    columns = r.json()
    assert len(columns) >= 3, columns
    print(f"    -> {len(columns)} columns")

    print("[5] Create task in first column")
    r = httpx.post(
        f"{base_url}/api/tasks/boards/{board_id}/tasks",
        json={"title": "Write docs", "column_id": columns[0]["id"]},
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    task = r.json()
    task_id = task["id"]

    print("[6] Move task to second column")
    r = httpx.patch(
        f"{base_url}/api/tasks/tasks/{task_id}",
        json={"column_id": columns[1]["id"], "status": "in_progress"},
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()

    print("[7] Comment on task")
    r = httpx.post(
        f"{base_url}/api/tasks/tasks/{task_id}/comments",
        json={"body": "Looking good!"},
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()

    print("[8] Upload document")
    files = {"file": ("hello.txt", io.BytesIO(b"hello world"), "text/plain")}
    r = httpx.post(
        f"{base_url}/api/documents/projects/{project_id}/documents",
        files=files,
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    document_id = r.json()["id"]

    print("[9] Download document")
    r = httpx.get(
        f"{base_url}/api/documents/documents/{document_id}/content",
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    assert r.content == b"hello world", r.content

    print("[10] List tasks")
    r = httpx.get(
        f"{base_url}/api/tasks/boards/{board_id}/tasks",
        headers=headers,
        timeout=10.0,
    )
    r.raise_for_status()
    assert len(r.json()) == 1

    if os.environ.get("TEAMBOARD_SMOKE_AI") == "1":
        print("[11] Generate AI project summary")
        r = httpx.post(
            f"{base_url}/api/ai/projects/{project_id}/summary",
            json={
                "include_comments": True,
                "include_documents": True,
                "focus": "Summarize smoke-test project state.",
            },
            headers=headers,
            timeout=60.0,
        )
        r.raise_for_status()
        ai_summary = r.json()
        assert ai_summary["summary"]

    print("\nAll smoke tests passed.")


if __name__ == "__main__":
    base_url = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("TEAMBOARD_URL", "http://localhost:8080")
    # tiny retry loop for cold start
    for attempt in range(10):
        try:
            main(base_url)
            break
        except (httpx.HTTPError, AssertionError) as exc:
            if attempt == 9:
                print(f"FAILED: {exc}")
                sys.exit(1)
            print(f"  retrying ({attempt + 1}/10) after error: {exc}")
            time.sleep(2)
