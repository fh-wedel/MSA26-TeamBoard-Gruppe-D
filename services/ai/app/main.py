from uuid import UUID

from fastapi import Depends, FastAPI

from .bedrock_client import generate_project_summary
from .config import settings
from .schemas import ProjectSummaryRequest, ProjectSummaryResponse
from .security import AuthContext, get_auth_context
from .service_clients import collect_project_context

app = FastAPI(title="TeamBoard AI Service", version="0.1.0", root_path=settings.ROOT_PATH)


@app.get("/health")
def health() -> dict:
    return {
        "status": "ok",
        "service": "ai",
        "provider": settings.AI_PROVIDER,
        "model_id": settings.BEDROCK_MODEL_ID,
    }


@app.post("/projects/{project_id}/summary", response_model=ProjectSummaryResponse)
async def summarize_project(
    project_id: UUID,
    payload: ProjectSummaryRequest,
    auth: AuthContext = Depends(get_auth_context),
) -> ProjectSummaryResponse:
    context = await collect_project_context(
        project_id=project_id,
        token=auth.token,
        include_comments=payload.include_comments,
        include_documents=payload.include_documents,
    )
    ai_result = generate_project_summary(context, payload.focus)
    return ProjectSummaryResponse(
        project_id=project_id,
        provider=settings.AI_PROVIDER,
        model_id=settings.BEDROCK_MODEL_ID,
        summary=ai_result["summary"],
        risks=ai_result["risks"],
        next_actions=ai_result["next_actions"],
    )
