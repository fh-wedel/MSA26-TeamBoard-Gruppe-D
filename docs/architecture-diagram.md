# TeamBoard — Architecture Diagram

```mermaid
graph TB
    Browser(["🌐 Browser · React SPA"])

    subgraph GW["Traefik Gateway · :80"]
        T["Rate Limiting · CORS · Trace-ID Injection"]
    end

    subgraph SVC["Microservices"]
        Auth["**Auth** :8001\nJWT · JWKS · Users"]
        Project["**Project** :8002\nBoards · Members\n★ Authoritative Permissions"]
        Task["**Task** :8003\nTasks · Comments\nStatus Transitions"]
        Document["**Document** :8004\nMetadata · Versions\nPre-signed URLs"]
        Notification["**Notification** :8005\nWebSocket Push\nRedis Backplane"]
        Plugin["**Plugin/Webhook** :8006\nHMAC Signing · Retry"]
    end

    subgraph INFRA["Infrastructure"]
        PG[("PostgreSQL\nauth_db · project_db · task_db\ndocument_db · notification_db · plugin_db")]
        Redis[("Redis\nPermission Cache · WS Backplane")]
        MQ{{"RabbitMQ\nteamboard.events\ntopic exchange"}}
        S3[("MinIO / S3\nDocument Storage")]
    end

    Browser -- "HTTPS" --> GW
    Browser <-- "WebSocket" --> Notification
    GW -- "JWT + Trace-ID" --> Auth & Project & Task & Document & Notification & Plugin

    Task & Document & Notification & Plugin -- "GET /internal/permissions" --> Project

    Auth & Project & Task & Document & Notification & Plugin -- "SQL + Outbox" --> PG

    Project & Task & Document -- "outbox → publish" --> MQ
    MQ -- "consume (idempotent)" --> Task & Notification & Plugin & Project

    Notification & Project -- "cache TTL 30s / backplane" --> Redis
    Document -- "upload / download" --> S3
```
