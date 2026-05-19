from uuid import UUID

import httpx
from fastapi import HTTPException, status

from .config import settings


def get_board(board_id: UUID) -> dict:
    url = f"{settings.PROJECTS_SERVICE_URL}/internal/boards/{board_id}"
    try:
        resp = httpx.get(url, timeout=5.0)
    except httpx.HTTPError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"Projects service unreachable: {exc}",
        )
    if resp.status_code == 404:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Board not found")
    if resp.status_code >= 400:
        raise HTTPException(status_code=resp.status_code, detail=resp.text)
    return resp.json()


def get_column(column_id: UUID) -> dict:
    url = f"{settings.PROJECTS_SERVICE_URL}/internal/columns/{column_id}"
    try:
        resp = httpx.get(url, timeout=5.0)
    except httpx.HTTPError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"Projects service unreachable: {exc}",
        )
    if resp.status_code == 404:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Column not found")
    if resp.status_code >= 400:
        raise HTTPException(status_code=resp.status_code, detail=resp.text)
    return resp.json()


def assert_member(project_id: UUID, user_id: UUID) -> None:
    url = f"{settings.PROJECTS_SERVICE_URL}/internal/projects/{project_id}/members/{user_id}"
    try:
        resp = httpx.get(url, timeout=5.0)
    except httpx.HTTPError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"Projects service unreachable: {exc}",
        )
    if resp.status_code == 404:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Not a project member")
    if resp.status_code >= 400:
        raise HTTPException(status_code=resp.status_code, detail=resp.text)
