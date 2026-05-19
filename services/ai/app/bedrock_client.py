import json
from typing import Any

import boto3
from botocore.exceptions import BotoCoreError, ClientError, NoCredentialsError
from fastapi import HTTPException, status

from .config import settings


SYSTEM_PROMPT = """You are TeamBoard's project assistant.
Summarize collaboration state for a software team based only on the supplied JSON.
Return concise, actionable output in German by default.
Return valid JSON with exactly these keys:
- summary: string in Markdown, 3-6 short bullets
- risks: array of short strings
- next_actions: array of short strings
Do not invent facts that are not present in the input."""


def build_prompt(context: dict[str, Any], focus: str | None) -> str:
    payload = json.dumps(context, ensure_ascii=False, default=str)
    focus_text = f"\nSpecial focus requested by the user: {focus}\n" if focus else ""
    return f"""Analyze this TeamBoard project context.{focus_text}
Project context JSON:
{payload}
"""


def _mock_response(context: dict[str, Any]) -> dict[str, Any]:
    boards = context.get("boards", [])
    tasks = [task for board in boards for task in board.get("tasks", [])]
    open_tasks = [task for task in tasks if task.get("status") != "done"]
    documents = context.get("documents", [])
    return {
        "summary": (
            f"- Projekt: {context.get('project', {}).get('name', 'Unbekannt')}\n"
            f"- Boards: {len(boards)}\n"
            f"- Tasks gesamt: {len(tasks)}\n"
            f"- Nicht abgeschlossene Tasks: {len(open_tasks)}\n"
            f"- Dokumente: {len(documents)}"
        ),
        "risks": ["Mock-Modus aktiv: keine Bedrock-Analyse durchgeführt."],
        "next_actions": ["AWS/BEDROCK-Konfiguration prüfen und AI_PROVIDER=bedrock setzen."],
    }


def generate_project_summary(context: dict[str, Any], focus: str | None) -> dict[str, Any]:
    if settings.AI_PROVIDER.lower() == "mock":
        return _mock_response(context)

    session_kwargs = {}
    if settings.AWS_PROFILE_NAME:
        session_kwargs["profile_name"] = settings.AWS_PROFILE_NAME
    session = boto3.Session(**session_kwargs)
    client = session.client("bedrock-runtime", region_name=settings.AWS_REGION)
    try:
        response = client.converse(
            modelId=settings.BEDROCK_MODEL_ID,
            system=[{"text": SYSTEM_PROMPT}],
            messages=[
                {
                    "role": "user",
                    "content": [{"text": build_prompt(context, focus)}],
                }
            ],
            inferenceConfig={
                "maxTokens": settings.BEDROCK_MAX_TOKENS,
                "temperature": settings.BEDROCK_TEMPERATURE,
            },
        )
    except NoCredentialsError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"AWS credentials not available for Bedrock: {exc}",
        )
    except (BotoCoreError, ClientError) as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"Bedrock request failed: {exc}",
        )

    text = response["output"]["message"]["content"][0]["text"]
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError:
        parsed = {
            "summary": text,
            "risks": ["Model response was not valid JSON."],
            "next_actions": [],
        }
    return {
        "summary": str(parsed.get("summary", "")),
        "risks": [str(item) for item in parsed.get("risks", [])],
        "next_actions": [str(item) for item in parsed.get("next_actions", [])],
    }
