import os
import time
from typing import Dict

import httpx
from fastapi import FastAPI, Request, Response
from fastapi.responses import FileResponse, JSONResponse, PlainTextResponse

from .audit import AuditLogger
from .config import settings

app = FastAPI(title="TeamBoard Gateway", version="0.1.0")

ROUTE_MAP: Dict[str, str] = {
    "auth": settings.AUTH_SERVICE_URL,
    "projects": settings.PROJECTS_SERVICE_URL,
    "tasks": settings.TASKS_SERVICE_URL,
    "documents": settings.DOCUMENTS_SERVICE_URL,
    "ai": settings.AI_SERVICE_URL,
}

HOP_BY_HOP = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
    "host",
    "content-length",
}

client = httpx.AsyncClient(timeout=30.0)
audit_logger = AuditLogger()


@app.on_event("shutdown")
async def _shutdown() -> None:
    await client.aclose()


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "gateway"}


@app.get("/health/services")
async def services_health() -> dict:
    results = {"gateway": {"status": "ok", "service": "gateway"}}
    for name, url in ROUTE_MAP.items():
        try:
            response = await client.get(f"{url}/health")
            if response.status_code >= 400:
                results[name] = {"status": "error", "status_code": response.status_code}
            else:
                results[name] = response.json()
        except httpx.HTTPError as exc:
            results[name] = {"status": "unreachable", "detail": str(exc)}
    overall = "ok" if all(item.get("status") == "ok" for item in results.values()) else "degraded"
    return {"status": overall, "services": results}


def _service_for(prefix: str) -> str | None:
    return ROUTE_MAP.get(prefix)


def _filter_headers(headers) -> dict:
    return {k: v for k, v in headers.items() if k.lower() not in HOP_BY_HOP}


@app.api_route(
    "/api/{service}/{path:path}",
    methods=["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
)
async def proxy(service: str, path: str, request: Request) -> Response:
    start = time.perf_counter()
    target = _service_for(service)
    if target is None:
        return JSONResponse({"detail": f"Unknown service '{service}'"}, status_code=404)
    if path == "internal" or path.startswith("internal/"):
        return JSONResponse({"detail": "Internal endpoints are not exposed through the gateway"}, status_code=404)

    url = f"{target}/{path}"
    body = await request.body()
    headers = _filter_headers(request.headers)

    try:
        upstream = await client.request(
            method=request.method,
            url=url,
            params=request.query_params,
            content=body,
            headers=headers,
        )
    except httpx.HTTPError as exc:
        audit_logger.log_api_call(
            request=request,
            service=service,
            path=path,
            status_code=503,
            duration_ms=int((time.perf_counter() - start) * 1000),
        )
        return JSONResponse(
            {"detail": f"Upstream {service} unreachable: {exc}"},
            status_code=503,
        )

    response_headers = _filter_headers(upstream.headers)
    audit_logger.log_api_call(
        request=request,
        service=service,
        path=path,
        status_code=upstream.status_code,
        duration_ms=int((time.perf_counter() - start) * 1000),
    )
    return Response(
        content=upstream.content,
        status_code=upstream.status_code,
        headers=response_headers,
        media_type=upstream.headers.get("content-type"),
    )


# ---------- static frontend ----------

@app.get("/")
def index() -> Response:
    index_path = os.path.join(settings.FRONTEND_DIR, "index.html")
    if not os.path.exists(index_path):
        return PlainTextResponse("Frontend not mounted", status_code=404)
    return FileResponse(index_path)


@app.get("/{filename}")
def static_file(filename: str) -> Response:
    if filename.startswith("api") or "/" in filename or filename in {"health"}:
        return PlainTextResponse("Not found", status_code=404)
    full_path = os.path.join(settings.FRONTEND_DIR, filename)
    if os.path.isfile(full_path):
        return FileResponse(full_path)
    return PlainTextResponse("Not found", status_code=404)
