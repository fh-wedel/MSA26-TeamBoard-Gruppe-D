from uuid import UUID

import httpx

from .config import settings


def lookup_user(user_id: UUID) -> dict | None:
    try:
        response = httpx.get(f"{settings.AUTH_SERVICE_URL}/internal/users/{user_id}", timeout=5.0)
        if response.status_code == 404:
            return None
        response.raise_for_status()
        return response.json()
    except httpx.HTTPError:
        return None
