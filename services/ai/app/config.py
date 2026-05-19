from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    JWT_SECRET: str = "change-me"
    JWT_ALGORITHM: str = "HS256"
    ROOT_PATH: str = ""

    PROJECTS_SERVICE_URL: str = "http://projects-service:8000"
    TASKS_SERVICE_URL: str = "http://tasks-service:8000"
    DOCUMENTS_SERVICE_URL: str = "http://documents-service:8000"

    AI_PROVIDER: str = "bedrock"
    AWS_REGION: str = "eu-central-1"
    AWS_PROFILE_NAME: str = ""
    BEDROCK_MODEL_ID: str = "anthropic.claude-3-haiku-20240307-v1:0"
    BEDROCK_MAX_TOKENS: int = 1200
    BEDROCK_TEMPERATURE: float = 0.2


settings = Settings()
