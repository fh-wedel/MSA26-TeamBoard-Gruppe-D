import os
from pathlib import Path
from typing import BinaryIO
from uuid import UUID

import boto3
from botocore.exceptions import BotoCoreError, ClientError, NoCredentialsError
from fastapi import HTTPException, status

from .config import settings


class StoredObject:
    def __init__(self, storage_path: str, size_bytes: int) -> None:
        self.storage_path = storage_path
        self.size_bytes = size_bytes


class StorageObject:
    def __init__(self, body: BinaryIO, media_type: str | None, filename: str) -> None:
        self.body = body
        self.media_type = media_type
        self.filename = filename


class DocumentStorage:
    def prepare(self) -> None:
        raise NotImplementedError

    async def save(self, project_id: UUID, document_id: UUID, file, max_bytes: int) -> StoredObject:
        raise NotImplementedError

    def open(self, storage_path: str, content_type: str | None, filename: str) -> StorageObject:
        raise NotImplementedError

    def delete(self, storage_path: str) -> None:
        raise NotImplementedError


class LocalDocumentStorage(DocumentStorage):
    def prepare(self) -> None:
        Path(settings.DOCUMENT_STORAGE_DIR).mkdir(parents=True, exist_ok=True)

    async def save(self, project_id: UUID, document_id: UUID, file, max_bytes: int) -> StoredObject:
        project_dir = Path(settings.DOCUMENT_STORAGE_DIR) / str(project_id)
        project_dir.mkdir(parents=True, exist_ok=True)
        storage_path = project_dir / str(document_id)

        size = 0
        with storage_path.open("wb") as out:
            while True:
                chunk = await file.read(1024 * 1024)
                if not chunk:
                    break
                size += len(chunk)
                if size > max_bytes:
                    out.close()
                    storage_path.unlink(missing_ok=True)
                    raise HTTPException(
                        status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE,
                        detail="File too large",
                    )
                out.write(chunk)
        return StoredObject(storage_path=str(storage_path), size_bytes=size)

    def open(self, storage_path: str, content_type: str | None, filename: str) -> StorageObject:
        if not os.path.exists(storage_path):
            raise HTTPException(status_code=status.HTTP_410_GONE, detail="File content missing")
        return StorageObject(
            body=open(storage_path, "rb"),
            media_type=content_type or "application/octet-stream",
            filename=filename,
        )

    def delete(self, storage_path: str) -> None:
        try:
            Path(storage_path).unlink(missing_ok=True)
        except OSError:
            pass


class S3DocumentStorage(DocumentStorage):
    def __init__(self) -> None:
        session_kwargs = {}
        if settings.AWS_PROFILE_NAME:
            session_kwargs["profile_name"] = settings.AWS_PROFILE_NAME
        self.session = boto3.Session(**session_kwargs)
        self.client = self.session.client("s3", region_name=settings.AWS_REGION)

    def prepare(self) -> None:
        if not settings.DOCUMENT_S3_BUCKET:
            raise RuntimeError("DOCUMENT_S3_BUCKET must be set when DOCUMENT_STORAGE_BACKEND=s3")

    async def save(self, project_id: UUID, document_id: UUID, file, max_bytes: int) -> StoredObject:
        key = "/".join(
            part.strip("/")
            for part in [settings.DOCUMENT_S3_PREFIX, str(project_id), str(document_id)]
            if part.strip("/")
        )
        size = 0
        chunks = []
        while True:
            chunk = await file.read(1024 * 1024)
            if not chunk:
                break
            size += len(chunk)
            if size > max_bytes:
                raise HTTPException(
                    status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE,
                    detail="File too large",
                )
            chunks.append(chunk)

        try:
            self.client.put_object(
                Bucket=settings.DOCUMENT_S3_BUCKET,
                Key=key,
                Body=b"".join(chunks),
                ContentType=file.content_type or "application/octet-stream",
                Metadata={"project_id": str(project_id), "document_id": str(document_id)},
            )
        except NoCredentialsError as exc:
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail=f"AWS credentials not available for S3: {exc}",
            )
        except (BotoCoreError, ClientError) as exc:
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail=f"S3 upload failed: {exc}",
            )
        return StoredObject(storage_path=f"s3://{settings.DOCUMENT_S3_BUCKET}/{key}", size_bytes=size)

    def open(self, storage_path: str, content_type: str | None, filename: str) -> StorageObject:
        prefix = f"s3://{settings.DOCUMENT_S3_BUCKET}/"
        if not storage_path.startswith(prefix):
            raise HTTPException(status_code=status.HTTP_410_GONE, detail="Invalid S3 storage path")
        key = storage_path.removeprefix(prefix)
        try:
            obj = self.client.get_object(Bucket=settings.DOCUMENT_S3_BUCKET, Key=key)
        except self.client.exceptions.NoSuchKey:
            raise HTTPException(status_code=status.HTTP_410_GONE, detail="File content missing")
        except NoCredentialsError as exc:
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail=f"AWS credentials not available for S3: {exc}",
            )
        except (BotoCoreError, ClientError) as exc:
            raise HTTPException(
                status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                detail=f"S3 download failed: {exc}",
            )
        return StorageObject(
            body=obj["Body"],
            media_type=content_type or obj.get("ContentType") or "application/octet-stream",
            filename=filename,
        )

    def delete(self, storage_path: str) -> None:
        prefix = f"s3://{settings.DOCUMENT_S3_BUCKET}/"
        if not storage_path.startswith(prefix):
            return
        key = storage_path.removeprefix(prefix)
        try:
            self.client.delete_object(Bucket=settings.DOCUMENT_S3_BUCKET, Key=key)
        except (BotoCoreError, ClientError, NoCredentialsError):
            pass


def get_storage() -> DocumentStorage:
    backend = settings.DOCUMENT_STORAGE_BACKEND.lower()
    if backend == "local":
        return LocalDocumentStorage()
    if backend == "s3":
        return S3DocumentStorage()
    raise RuntimeError(f"Unsupported document storage backend: {settings.DOCUMENT_STORAGE_BACKEND}")
