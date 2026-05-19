from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    DATABASE_URL: str = "postgresql+psycopg://teamboard:teamboard@postgres:5432/teamboard"
    DB_SCHEMA: str = "documents"
    JWT_SECRET: str = "change-me"
    JWT_ALGORITHM: str = "HS256"
    PROJECTS_SERVICE_URL: str = "http://projects-service:8000"
    DOCUMENT_STORAGE_BACKEND: str = "local"
    DOCUMENT_STORAGE_DIR: str = "/data/documents"
    DOCUMENT_S3_BUCKET: str = ""
    DOCUMENT_S3_PREFIX: str = "teamboard/documents"
    AWS_REGION: str = "eu-central-1"
    AWS_PROFILE_NAME: str = ""
    MAX_UPLOAD_BYTES: int = 25 * 1024 * 1024  # 25 MB
    ROOT_PATH: str = ""


settings = Settings()
