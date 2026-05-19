from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    JWT_SECRET: str = "change-me"
    JWT_ALGORITHM: str = "HS256"

    AUTH_SERVICE_URL: str = "http://auth-service:8000"
    PROJECTS_SERVICE_URL: str = "http://projects-service:8000"
    TASKS_SERVICE_URL: str = "http://tasks-service:8000"
    DOCUMENTS_SERVICE_URL: str = "http://documents-service:8000"
    AI_SERVICE_URL: str = "http://ai-service:8000"

    AUDIT_LOG_BACKEND: str = "none"
    AUDIT_DYNAMODB_TABLE: str = ""
    AWS_REGION: str = "eu-central-1"
    AWS_PROFILE_NAME: str = ""

    FRONTEND_DIR: str = "/app/frontend"


settings = Settings()
