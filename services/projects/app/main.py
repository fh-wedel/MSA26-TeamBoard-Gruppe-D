from typing import List
from uuid import UUID

from fastapi import Depends, FastAPI, HTTPException, status
from sqlalchemy import text
from sqlalchemy.orm import Session

from .auth_client import lookup_user
from .config import settings
from .db import Base, engine, get_db
from .models import Board, BoardColumn, Member, Project
from .schemas import (
    BoardCreate,
    BoardOut,
    ColumnCreate,
    ColumnInternalOut,
    ColumnOut,
    MemberCreate,
    MemberOut,
    ProjectCreate,
    ProjectOut,
)
from .security import get_current_user_id

app = FastAPI(title="TeamBoard Projects Service", version="0.1.0", root_path=settings.ROOT_PATH)

DEFAULT_KANBAN_COLUMNS = ["Backlog", "To Do", "In Progress", "In Review", "Ready for Release", "Done"]


@app.on_event("startup")
def on_startup() -> None:
    with engine.begin() as conn:
        conn.execute(text(f'CREATE SCHEMA IF NOT EXISTS "{settings.DB_SCHEMA}"'))
    Base.metadata.create_all(bind=engine)
    with Session(engine) as db:
        _ensure_default_kanban_columns(db)


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "projects"}


# ---------- helpers ----------

def _require_member(db: Session, project_id: UUID, user_id: UUID) -> Member:
    member = (
        db.query(Member)
        .filter(Member.project_id == project_id, Member.user_id == user_id)
        .first()
    )
    if not member:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Not a project member")
    return member


def _ensure_default_kanban_columns(db: Session) -> None:
    boards = db.query(Board).filter(Board.type == "kanban").all()
    for board in boards:
        columns = db.query(BoardColumn).filter(BoardColumn.board_id == board.id).all()
        by_name = {column.name: column for column in columns}
        for idx, name in enumerate(DEFAULT_KANBAN_COLUMNS):
            if name in by_name:
                by_name[name].order_idx = idx
            else:
                db.add(BoardColumn(board_id=board.id, name=name, order_idx=idx))
    db.commit()


def _require_owner(db: Session, project_id: UUID, user_id: UUID) -> None:
    member = _require_member(db, project_id, user_id)
    if member.role != "owner":
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Owner role required")


def _member_out(member: Member) -> MemberOut:
    user = lookup_user(member.user_id)
    return MemberOut(
        project_id=member.project_id,
        user_id=member.user_id,
        role=member.role,
        display_name=user.get("display_name") if user else None,
        email=user.get("email") if user else None,
    )


# ---------- projects ----------

@app.get("/projects", response_model=List[ProjectOut])
def list_projects(
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> List[ProjectOut]:
    rows = (
        db.query(Project)
        .join(Member, Member.project_id == Project.id)
        .filter(Member.user_id == user_id)
        .order_by(Project.created_at.desc())
        .all()
    )
    return rows


@app.post("/projects", response_model=ProjectOut, status_code=status.HTTP_201_CREATED)
def create_project(
    payload: ProjectCreate,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> ProjectOut:
    project = Project(name=payload.name, description=payload.description, owner_id=user_id)
    db.add(project)
    db.flush()
    db.add(Member(project_id=project.id, user_id=user_id, role="owner"))
    db.commit()
    db.refresh(project)
    return project


@app.get("/projects/{project_id}", response_model=ProjectOut)
def get_project(
    project_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> ProjectOut:
    _require_member(db, project_id, user_id)
    project = db.query(Project).filter(Project.id == project_id).first()
    if not project:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Project not found")
    return project


# ---------- members ----------

@app.get("/projects/{project_id}/members", response_model=List[MemberOut])
def list_members(
    project_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> List[MemberOut]:
    _require_member(db, project_id, user_id)
    members = db.query(Member).filter(Member.project_id == project_id).all()
    return [_member_out(member) for member in members]


@app.post("/projects/{project_id}/members", response_model=MemberOut, status_code=status.HTTP_201_CREATED)
def add_member(
    project_id: UUID,
    payload: MemberCreate,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> MemberOut:
    _require_owner(db, project_id, user_id)
    existing = (
        db.query(Member)
        .filter(Member.project_id == project_id, Member.user_id == payload.user_id)
        .first()
    )
    if existing:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="User already a member")
    member = Member(project_id=project_id, user_id=payload.user_id, role=payload.role)
    db.add(member)
    db.commit()
    db.refresh(member)
    return _member_out(member)


# ---------- boards ----------

@app.get("/projects/{project_id}/boards", response_model=List[BoardOut])
def list_boards(
    project_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> List[BoardOut]:
    _require_member(db, project_id, user_id)
    return (
        db.query(Board)
        .filter(Board.project_id == project_id)
        .order_by(Board.created_at.asc())
        .all()
    )


@app.post(
    "/projects/{project_id}/boards",
    response_model=BoardOut,
    status_code=status.HTTP_201_CREATED,
)
def create_board(
    project_id: UUID,
    payload: BoardCreate,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> BoardOut:
    _require_member(db, project_id, user_id)
    board = Board(project_id=project_id, name=payload.name, type=payload.type)
    db.add(board)
    db.flush()
    if payload.type == "kanban":
        for idx, name in enumerate(DEFAULT_KANBAN_COLUMNS):
            db.add(BoardColumn(board_id=board.id, name=name, order_idx=idx))
    db.commit()
    db.refresh(board)
    return board


@app.get("/boards/{board_id}", response_model=BoardOut)
def get_board(
    board_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> BoardOut:
    board = db.query(Board).filter(Board.id == board_id).first()
    if not board:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Board not found")
    _require_member(db, board.project_id, user_id)
    return board


@app.get("/boards/{board_id}/columns", response_model=List[ColumnOut])
def list_columns(
    board_id: UUID,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> List[ColumnOut]:
    board = db.query(Board).filter(Board.id == board_id).first()
    if not board:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Board not found")
    _require_member(db, board.project_id, user_id)
    return (
        db.query(BoardColumn)
        .filter(BoardColumn.board_id == board_id)
        .order_by(BoardColumn.order_idx.asc())
        .all()
    )


@app.post(
    "/boards/{board_id}/columns",
    response_model=ColumnOut,
    status_code=status.HTTP_201_CREATED,
)
def create_column(
    board_id: UUID,
    payload: ColumnCreate,
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> ColumnOut:
    board = db.query(Board).filter(Board.id == board_id).first()
    if not board:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Board not found")
    _require_member(db, board.project_id, user_id)
    col = BoardColumn(board_id=board_id, name=payload.name, order_idx=payload.order_idx)
    db.add(col)
    db.commit()
    db.refresh(col)
    return col


# ---------- internal endpoints (called by other services) ----------

@app.get("/internal/projects/{project_id}/members/{user_id}", response_model=MemberOut)
def internal_get_member(
    project_id: UUID,
    user_id: UUID,
    db: Session = Depends(get_db),
) -> MemberOut:
    member = (
        db.query(Member)
        .filter(Member.project_id == project_id, Member.user_id == user_id)
        .first()
    )
    if not member:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Not a member")
    return member


@app.get("/internal/boards/{board_id}", response_model=BoardOut)
def internal_get_board(board_id: UUID, db: Session = Depends(get_db)) -> BoardOut:
    board = db.query(Board).filter(Board.id == board_id).first()
    if not board:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Board not found")
    return board


@app.get("/internal/columns/{column_id}", response_model=ColumnInternalOut)
def internal_get_column(column_id: UUID, db: Session = Depends(get_db)) -> ColumnInternalOut:
    result = (
        db.query(BoardColumn, Board.project_id)
        .join(Board, Board.id == BoardColumn.board_id)
        .filter(BoardColumn.id == column_id)
        .first()
    )
    if not result:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Column not found")
    column, project_id = result
    return ColumnInternalOut(
        id=column.id,
        board_id=column.board_id,
        name=column.name,
        order_idx=column.order_idx,
        project_id=project_id,
    )
