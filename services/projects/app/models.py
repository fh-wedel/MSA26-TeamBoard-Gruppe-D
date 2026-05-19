import uuid
from datetime import datetime

from sqlalchemy import Column, DateTime, ForeignKey, Integer, String
from sqlalchemy.dialects.postgresql import UUID

from .db import Base


class Project(Base):
    __tablename__ = "projects"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    name = Column(String(255), nullable=False)
    description = Column(String(2000), nullable=True)
    owner_id = Column(UUID(as_uuid=True), nullable=False, index=True)
    created_at = Column(DateTime, default=datetime.utcnow, nullable=False)


class Member(Base):
    __tablename__ = "members"

    project_id = Column(
        UUID(as_uuid=True),
        ForeignKey(f"{Base.metadata.schema}.projects.id", ondelete="CASCADE"),
        primary_key=True,
    )
    user_id = Column(UUID(as_uuid=True), primary_key=True, index=True)
    role = Column(String(32), nullable=False, default="member")
    created_at = Column(DateTime, default=datetime.utcnow, nullable=False)


class Board(Base):
    __tablename__ = "boards"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    project_id = Column(
        UUID(as_uuid=True),
        ForeignKey(f"{Base.metadata.schema}.projects.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    name = Column(String(255), nullable=False)
    type = Column(String(32), nullable=False, default="kanban")
    created_at = Column(DateTime, default=datetime.utcnow, nullable=False)


class BoardColumn(Base):
    __tablename__ = "columns"

    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    board_id = Column(
        UUID(as_uuid=True),
        ForeignKey(f"{Base.metadata.schema}.boards.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    name = Column(String(255), nullable=False)
    order_idx = Column(Integer, nullable=False, default=0)
