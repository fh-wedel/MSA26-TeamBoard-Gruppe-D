# AWS-Zielarchitektur für TeamBoard

Diese Datei beschreibt, welche AWS-Services im TeamBoard-MVP sinnvoll eingesetzt werden und welche bewusst nicht in den kritischen Pfad gezwungen werden.

## Empfohlene Service-Zuordnung

| TeamBoard-Baustein | AWS-Service | Begründung |
| --- | --- | --- |
| FastAPI-Microservices (`gateway`, `auth`, `projects`, `tasks`, `documents`, `ai`) | ECS Fargate | Container passen direkt zur bestehenden Docker-Struktur, kein Servermanagement, sauber skalierbar pro Service. |
| PostgreSQL | Amazon RDS / Aurora PostgreSQL | Relationale Daten passen besser zu Projekten, Mitgliedschaften, Boards und Tasks als DynamoDB. |
| Dokument-Binaries | Amazon S3 | Best Practice für Dateiablage, günstig, skalierbar, langlebig, kompatibel mit S3 Events. |
| AI-Funktion | Amazon Bedrock | Bereits integriert; eignet sich für Projektzusammenfassungen, Risikoanalyse und nächste Schritte. |
| Öffentliche API | API Gateway HTTP API oder ALB | API Gateway ist gut für zentrale API-Policies, Throttling und später Cognito/JWT Authorizer; ALB ist einfacher für reines ECS-Routing. |
| Audit-/Event-Log | DynamoDB | Append-only Write-Events sind ein sehr guter DynamoDB-Use-Case: hoher Durchsatz, einfache Key-Struktur, TTL möglich. |
| Dokument-Nachverarbeitung | Lambda via S3 Event | Entkoppelte Verarbeitung nach Upload, z. B. Virenscan, Text-Extraktion, Thumbnailing, Bedrock-Embeddings. |
| Realtime/GraphQL | AppSync | Für spätere Live-Updates/Subscriptions sinnvoll; für den aktuellen REST-MVP noch nicht notwendig. |
| Frontend | S3 + CloudFront | Das statische HTML/JS-Frontend kann kostengünstig und performant ausgeliefert werden. |
| Secrets | Secrets Manager / SSM Parameter Store | JWT Secret, DB Passwort und externe Konfiguration sollten nicht in Images oder Git liegen. |
| Logs/Metriken | CloudWatch | Standard für ECS/Lambda Logs, Health-Metriken und Alarme. |

## Zielbild

```text
Browser
  ├─ Static UI: CloudFront + S3
  └─ API: API Gateway HTTP API oder ALB
        └─ ECS Fargate Services
             ├─ gateway
             ├─ auth-service ───────┐
             ├─ projects-service ───┤
             ├─ tasks-service ──────┼─ RDS/Aurora PostgreSQL
             ├─ documents-service ──┘
             │     └─ S3 document bucket ── S3 Event ── Lambda document processor
             └─ ai-service ── Bedrock

Gateway ── write event logs ── DynamoDB audit table
```

## Warum nicht alles sofort produktiv verdrahten?

- **AppSync** ist stark für GraphQL und Subscriptions, aber der aktuelle Client und die Services sind REST-basiert. Eine direkte Migration wäre mehr Aufwand als Nutzen. Sinnvoller nächster Schritt: AppSync nur für Live-Events/Subscriptions ergänzen.
- **DynamoDB** ersetzt hier nicht PostgreSQL, weil Boards/Tasks/Memberships relational sind. DynamoDB wird für Audit-/Event-Daten verwendet, wo es sehr gut passt.
- **Lambda** ersetzt die FastAPI-Services nicht, weil diese bereits containerisiert sind und dauerhaft REST-Anfragen bedienen. Lambda ist ideal für asynchrone Nebenjobs nach S3 Uploads.

## Bereits im Code vorbereitet

- `documents-service`: `DOCUMENT_STORAGE_BACKEND=local|s3`
- `gateway`: optionales Audit Logging mit `AUDIT_LOG_BACKEND=dynamodb`
- `ai-service`: Bedrock über `boto3`

## Lokale AWS-Nutzung

```bash
cp .env.example .env
```

Beispiele:

```bash
AWS_PROFILE_NAME=dein-profil
AWS_REGION=eu-central-1

DOCUMENT_STORAGE_BACKEND=s3
DOCUMENT_S3_BUCKET=dein-teamboard-bucket
DOCUMENT_S3_PREFIX=teamboard/documents

AUDIT_LOG_BACKEND=dynamodb
AUDIT_DYNAMODB_TABLE=teamboard-audit-events
```

Dann:

```bash
docker compose up --build
```

Die Compose-Datei mountet `${USERPROFILE}/.aws` read-only in Container, die AWS benötigen.

## Nächste sinnvolle Cloud-Schritte

1. ECR-Repositories für alle Service-Images anlegen.
2. RDS PostgreSQL bereitstellen.
3. S3 Document Bucket und DynamoDB Audit Table anlegen.
4. ECS Cluster + Fargate Services definieren.
5. API Gateway HTTP API oder ALB vor den Gateway-Service setzen.
6. Frontend nach S3 deployen und CloudFront davor setzen.
7. Lambda an S3 ObjectCreated Events hängen.
8. Optional: AppSync für Live-Updates ergänzen.
