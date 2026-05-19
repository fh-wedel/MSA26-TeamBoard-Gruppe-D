from typing import List, Optional
from uuid import UUID

from pydantic import BaseModel, Field


class ProjectSummaryRequest(BaseModel):
    include_comments: bool = True
    include_documents: bool = True
    focus: Optional[str] = Field(default=None, max_length=500)


class ProjectSummaryResponse(BaseModel):
    project_id: UUID
    provider: str
    model_id: str
    summary: str
    risks: List[str]
    next_actions: List[str]
