import time
import uuid
from datetime import datetime, timezone

import boto3
from botocore.exceptions import BotoCoreError, ClientError, NoCredentialsError
from fastapi import Request
from jose import JWTError, jwt

from .config import settings

WRITE_METHODS = {"POST", "PUT", "PATCH", "DELETE"}


class AuditLogger:
    def __init__(self) -> None:
        self.backend = settings.AUDIT_LOG_BACKEND.lower()
        self.table = None
        if self.backend == "dynamodb" and settings.AUDIT_DYNAMODB_TABLE:
            session_kwargs = {}
            if settings.AWS_PROFILE_NAME:
                session_kwargs["profile_name"] = settings.AWS_PROFILE_NAME
            session = boto3.Session(**session_kwargs)
            self.table = session.resource("dynamodb", region_name=settings.AWS_REGION).Table(
                settings.AUDIT_DYNAMODB_TABLE
            )

    def enabled(self) -> bool:
        return self.backend == "dynamodb" and bool(settings.AUDIT_DYNAMODB_TABLE) and self.table is not None

    def log_api_call(
        self,
        request: Request,
        service: str,
        path: str,
        status_code: int,
        duration_ms: int,
    ) -> None:
        if request.method not in WRITE_METHODS or not self.enabled():
            return
        user_id = _extract_user_id(request)
        item = {
            "pk": f"SERVICE#{service}",
            "sk": f"{datetime.now(timezone.utc).isoformat()}#{uuid.uuid4()}",
            "event_id": str(uuid.uuid4()),
            "event_type": "api_write",
            "service": service,
            "path": f"/{path}",
            "method": request.method,
            "status_code": status_code,
            "duration_ms": duration_ms,
            "user_id": user_id or "anonymous",
            "created_at": int(time.time()),
        }
        try:
            self.table.put_item(Item=item)
        except (BotoCoreError, ClientError, NoCredentialsError):
            return


def _extract_user_id(request: Request) -> str | None:
    header = request.headers.get("authorization", "")
    if not header.lower().startswith("bearer "):
        return None
    token = header.split(" ", 1)[1]
    try:
        payload = jwt.decode(token, settings.JWT_SECRET, algorithms=[settings.JWT_ALGORITHM])
    except JWTError:
        return None
    sub = payload.get("sub")
    return str(sub) if sub else None
