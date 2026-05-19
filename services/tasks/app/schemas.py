from datetime import date, datetime
from typing import Optional
from uuid import UUID

from pydantic import BaseModel, Field


class TaskCreate(BaseModel):
    title: str = Field(min_length=1, max_length=255)
    description: Optional[str] = None
    column_id: UUID
    assignee_id: Optional[UUID] = None
    due_date: Optional[date] = None
    status: Optional[str] = Field(default=None, max_length=32)
    sprint: Optional[str] = Field(default=None, max_length=64)


class TaskUpdate(BaseModel):
    title: Optional[str] = Field(default=None, min_length=1, max_length=255)
    description: Optional[str] = None
    column_id: Optional[UUID] = None
    assignee_id: Optional[UUID] = None
    due_date: Optional[date] = None
    status: Optional[str] = Field(default=None, max_length=32)
    sprint: Optional[str] = Field(default=None, max_length=64)


class TaskOut(BaseModel):
    id: UUID
    board_id: UUID
    column_id: UUID
    title: str
    description: Optional[str]
    assignee_id: Optional[UUID]
    due_date: Optional[date]
    status: str
    sprint: Optional[str]
    created_by: UUID
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class CommentCreate(BaseModel):
    body: str = Field(min_length=1, max_length=4000)


class CommentOut(BaseModel):
    id: UUID
    task_id: UUID
    author_id: UUID
    body: str
    created_at: datetime

    class Config:
        from_attributes = True
