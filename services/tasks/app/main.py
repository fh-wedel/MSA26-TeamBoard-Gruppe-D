from typing import List
from uuid import UUID

from fastapi import Depends, FastAPI, HTTPException, status
from sqlalchemy import text
from sqlalchemy.orm import Session

from .config import settings
from .db import Base, engine, get_db
from .models import Comment, Task
from .projects_client import assert_member, get_board, get_column
from .schemas import (
    CommentCreate,
    CommentOut,
    TaskCreate,
    TaskOut,
    TaskUpdate,
)
from .security import AuthContext, get_auth_context

app = FastAPI(title="TeamBoard Tasks Service", version="0.1.0", root_path=settings.ROOT_PATH)


@app.on_event("startup")
def on_startup() -> None:
    with engine.begin() as conn:
        conn.execute(text(f'CREATE SCHEMA IF NOT EXISTS "{settings.DB_SCHEMA}"'))
    Base.metadata.create_all(bind=engine)
    with engine.begin() as conn:
        conn.execute(text(f'ALTER TABLE "{settings.DB_SCHEMA}".tasks ADD COLUMN IF NOT EXISTS sprint VARCHAR(64)'))
        conn.execute(text(f'CREATE INDEX IF NOT EXISTS ix_{settings.DB_SCHEMA}_tasks_sprint ON "{settings.DB_SCHEMA}".tasks (sprint)'))


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "tasks"}


def _board_project_id(board_id: UUID) -> UUID:
    board = get_board(board_id)
    return UUID(board["project_id"])


def _validate_column_for_board(column_id: UUID, board_id: UUID) -> None:
    column = get_column(column_id)
    if UUID(column["board_id"]) != board_id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Column does not belong to this board",
        )


@app.get("/boards/{board_id}/tasks", response_model=List[TaskOut])
def list_tasks(
    board_id: UUID,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> List[TaskOut]:
    project_id = _board_project_id(board_id)
    assert_member(project_id, auth.user_id)
    return (
        db.query(Task)
        .filter(Task.board_id == board_id)
        .order_by(Task.created_at.asc())
        .all()
    )


@app.post(
    "/boards/{board_id}/tasks",
    response_model=TaskOut,
    status_code=status.HTTP_201_CREATED,
)
def create_task(
    board_id: UUID,
    payload: TaskCreate,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> TaskOut:
    project_id = _board_project_id(board_id)
    assert_member(project_id, auth.user_id)
    _validate_column_for_board(payload.column_id, board_id)
    task = Task(
        board_id=board_id,
        column_id=payload.column_id,
        title=payload.title,
        description=payload.description,
        assignee_id=payload.assignee_id,
        due_date=payload.due_date,
        status=payload.status or "open",
        sprint=payload.sprint,
        created_by=auth.user_id,
    )
    db.add(task)
    db.commit()
    db.refresh(task)
    return task


def _get_task_or_404(db: Session, task_id: UUID) -> Task:
    task = db.query(Task).filter(Task.id == task_id).first()
    if not task:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Task not found")
    return task


@app.get("/tasks/{task_id}", response_model=TaskOut)
def get_task(
    task_id: UUID,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> TaskOut:
    task = _get_task_or_404(db, task_id)
    project_id = _board_project_id(task.board_id)
    assert_member(project_id, auth.user_id)
    return task


@app.patch("/tasks/{task_id}", response_model=TaskOut)
def update_task(
    task_id: UUID,
    payload: TaskUpdate,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> TaskOut:
    task = _get_task_or_404(db, task_id)
    project_id = _board_project_id(task.board_id)
    assert_member(project_id, auth.user_id)
    data = payload.model_dump(exclude_unset=True)
    if "column_id" in data:
        _validate_column_for_board(data["column_id"], task.board_id)
    for key, value in data.items():
        setattr(task, key, value)
    db.commit()
    db.refresh(task)
    return task


@app.delete("/tasks/{task_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_task(
    task_id: UUID,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> None:
    task = _get_task_or_404(db, task_id)
    project_id = _board_project_id(task.board_id)
    assert_member(project_id, auth.user_id)
    db.delete(task)
    db.commit()


@app.get("/tasks/{task_id}/comments", response_model=List[CommentOut])
def list_comments(
    task_id: UUID,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> List[CommentOut]:
    task = _get_task_or_404(db, task_id)
    project_id = _board_project_id(task.board_id)
    assert_member(project_id, auth.user_id)
    return (
        db.query(Comment)
        .filter(Comment.task_id == task_id)
        .order_by(Comment.created_at.asc())
        .all()
    )


@app.post(
    "/tasks/{task_id}/comments",
    response_model=CommentOut,
    status_code=status.HTTP_201_CREATED,
)
def create_comment(
    task_id: UUID,
    payload: CommentCreate,
    auth: AuthContext = Depends(get_auth_context),
    db: Session = Depends(get_db),
) -> CommentOut:
    task = _get_task_or_404(db, task_id)
    project_id = _board_project_id(task.board_id)
    assert_member(project_id, auth.user_id)
    comment = Comment(task_id=task_id, author_id=auth.user_id, body=payload.body)
    db.add(comment)
    db.commit()
    db.refresh(comment)
    return comment
