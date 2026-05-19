from uuid import UUID

import httpx
from fastapi import HTTPException, status

from .config import settings


async def _get_json(client: httpx.AsyncClient, url: str, token: str) -> dict | list:
    try:
        response = await client.get(url, headers={"Authorization": f"Bearer {token}"})
    except httpx.HTTPError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"Internal service unreachable: {exc}",
        )
    if response.status_code >= 400:
        raise HTTPException(status_code=response.status_code, detail=response.text)
    return response.json()


async def collect_project_context(
    project_id: UUID,
    token: str,
    include_comments: bool,
    include_documents: bool,
) -> dict:
    async with httpx.AsyncClient(timeout=15.0) as client:
        project = await _get_json(
            client,
            f"{settings.PROJECTS_SERVICE_URL}/projects/{project_id}",
            token,
        )
        boards = await _get_json(
            client,
            f"{settings.PROJECTS_SERVICE_URL}/projects/{project_id}/boards",
            token,
        )

        board_contexts = []
        for board in boards:
            board_id = board["id"]
            columns = await _get_json(
                client,
                f"{settings.PROJECTS_SERVICE_URL}/boards/{board_id}/columns",
                token,
            )
            tasks = await _get_json(
                client,
                f"{settings.TASKS_SERVICE_URL}/boards/{board_id}/tasks",
                token,
            )
            if include_comments:
                for task in tasks:
                    task["comments"] = await _get_json(
                        client,
                        f"{settings.TASKS_SERVICE_URL}/tasks/{task['id']}/comments",
                        token,
                    )
            board_contexts.append(
                {
                    "board": board,
                    "columns": columns,
                    "tasks": tasks,
                }
            )

        documents = []
        if include_documents:
            documents = await _get_json(
                client,
                f"{settings.DOCUMENTS_SERVICE_URL}/projects/{project_id}/documents",
                token,
            )

    return {
        "project": project,
        "boards": board_contexts,
        "documents": documents,
    }
