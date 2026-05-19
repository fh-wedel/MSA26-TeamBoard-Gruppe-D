from datetime import datetime
from typing import Literal, Optional
from uuid import UUID

from pydantic import BaseModel, Field


class ProjectCreate(BaseModel):
    name: str = Field(min_length=1, max_length=255)
    description: Optional[str] = Field(default=None, max_length=2000)


class ProjectOut(BaseModel):
    id: UUID
    name: str
    description: Optional[str]
    owner_id: UUID
    created_at: datetime

    class Config:
        from_attributes = True


class MemberCreate(BaseModel):
    user_id: UUID
    role: Literal["owner", "member"] = "member"


class MemberOut(BaseModel):
    project_id: UUID
    user_id: UUID
    role: str
    display_name: Optional[str] = None
    email: Optional[str] = None

    class Config:
        from_attributes = True


class BoardCreate(BaseModel):
    name: str = Field(min_length=1, max_length=255)
    type: Literal["kanban"] = "kanban"


class BoardOut(BaseModel):
    id: UUID
    project_id: UUID
    name: str
    type: str
    created_at: datetime

    class Config:
        from_attributes = True


class ColumnInternalOut(BaseModel):
    id: UUID
    board_id: UUID
    name: str
    order_idx: int
    project_id: UUID


class ColumnCreate(BaseModel):
    name: str = Field(min_length=1, max_length=255)
    order_idx: int = 0


class ColumnOut(BaseModel):
    id: UUID
    board_id: UUID
    name: str
    order_idx: int

    class Config:
        from_attributes = True
