from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    DATABASE_URL: str = "postgresql+psycopg://teamboard:teamboard@postgres:5432/teamboard"
    DB_SCHEMA: str = "tasks"
    JWT_SECRET: str = "change-me"
    JWT_ALGORITHM: str = "HS256"
    PROJECTS_SERVICE_URL: str = "http://projects-service:8000"
    ROOT_PATH: str = ""


settings = Settings()
