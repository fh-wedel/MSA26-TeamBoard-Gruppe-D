import uuid
from typing import BinaryIO, Iterator, List
from uuid import UUID

from fastapi import Depends, FastAPI, File, HTTPException, UploadFile, status
from fastapi.responses import StreamingResponse
from sqlalchemy import text
from sqlalchemy.orm import Session

from .config import settings
from .db import Base, engine, get_db
from .models import Document
from .projects_client import assert_member
from .schemas import DocumentOut
from .security import get_current_user_id
from .storage import get_storage

app = FastAPI(title="TeamBoard Documents Service", version="0.1.0", root_path=settings.ROOT_PATH)
storage = get_storage()


@app.on_event("startup")
def on_startup() -> None:
    with engine.begin() as conn:
        conn.execute(text(f'CREATE SCHEMA IF NOT EXISTS "{settings.DB_SCHEMA}"'))
    Base.metadata.create_all(bind=engine)
    storage.prepare()


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "documents", "storage_backend": settings.DOCUMENT_STORAGE_BACKEND}


@app.get("/projects/{project_id}/documents", response_model=List[DocumentOut])
def list_documents(
    project_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> List[DocumentOut]:
    assert_member(project_id, user_id)
    return (
        db.query(Document)
        .filter(Document.project_id == project_id)
        .order_by(Document.created_at.desc())
        .all()
    )


@app.post(
    "/projects/{project_id}/documents",
    response_model=DocumentOut,
    status_code=status.HTTP_201_CREATED,
)
async def upload_document(
    project_id: UUID,
    file: UploadFile = File(...),
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> DocumentOut:
    assert_member(project_id, user_id)

    doc_id = uuid.uuid4()
    stored = await storage.save(project_id, doc_id, file, settings.MAX_UPLOAD_BYTES)

    doc = Document(
        id=doc_id,
        project_id=project_id,
        uploader_id=user_id,
        filename=file.filename or "unnamed",
        content_type=file.content_type or "application/octet-stream",
        size_bytes=stored.size_bytes,
        storage_path=stored.storage_path,
    )
    db.add(doc)
    db.commit()
    db.refresh(doc)
    return doc


def _get_doc_or_404(db: Session, document_id: UUID) -> Document:
    doc = db.query(Document).filter(Document.id == document_id).first()
    if not doc:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Document not found")
    return doc


@app.get("/documents/{document_id}", response_model=DocumentOut)
def get_document(
    document_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> DocumentOut:
    doc = _get_doc_or_404(db, document_id)
    assert_member(doc.project_id, user_id)
    return doc


def _stream_body(body: BinaryIO) -> Iterator[bytes]:
    try:
        while True:
            chunk = body.read(1024 * 1024)
            if not chunk:
                break
            yield chunk
    finally:
        body.close()


@app.get("/documents/{document_id}/content")
def download_document(
    document_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> StreamingResponse:
    doc = _get_doc_or_404(db, document_id)
    assert_member(doc.project_id, user_id)
    obj = storage.open(doc.storage_path, doc.content_type, doc.filename)
    return StreamingResponse(
        _stream_body(obj.body),
        media_type=obj.media_type,
        headers={"Content-Disposition": f'attachment; filename="{obj.filename}"'},
    )


@app.delete("/documents/{document_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_document(
    document_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> None:
    doc = _get_doc_or_404(db, document_id)
    assert_member(doc.project_id, user_id)
    storage.delete(doc.storage_path)
    db.delete(doc)
    db.commit()
