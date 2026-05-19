from uuid import UUID

from fastapi import Depends, FastAPI, HTTPException, status
from sqlalchemy import text
from sqlalchemy.orm import Session

from .config import settings
from .db import Base, engine, get_db
from .models import User
from .schemas import (
    LoginRequest,
    RegisterRequest,
    TokenResponse,
    UserLookupOut,
    UserOut,
)
from .security import (
    create_access_token,
    get_current_user_id,
    hash_password,
    verify_password,
)

app = FastAPI(title="TeamBoard Auth Service", version="0.1.0", root_path=settings.ROOT_PATH)


@app.on_event("startup")
def on_startup() -> None:
    with engine.begin() as conn:
        conn.execute(text(f'CREATE SCHEMA IF NOT EXISTS "{settings.DB_SCHEMA}"'))
    Base.metadata.create_all(bind=engine)


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "auth"}


@app.post("/register", response_model=UserOut, status_code=status.HTTP_201_CREATED)
def register(payload: RegisterRequest, db: Session = Depends(get_db)) -> UserOut:
    existing = db.query(User).filter(User.email == payload.email).first()
    if existing:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="Email already registered")
    user = User(
        email=payload.email,
        password_hash=hash_password(payload.password),
        display_name=payload.display_name,
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user


@app.post("/login", response_model=TokenResponse)
def login(payload: LoginRequest, db: Session = Depends(get_db)) -> TokenResponse:
    user = db.query(User).filter(User.email == payload.email).first()
    if not user or not verify_password(payload.password, user.password_hash):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid credentials")
    token = create_access_token(subject=str(user.id), extra={"email": user.email})
    return TokenResponse(access_token=token)


@app.get("/me", response_model=UserOut)
def me(
    user_id: UUID = Depends(get_current_user_id),
    db: Session = Depends(get_db),
) -> UserOut:
    user = db.query(User).filter(User.id == user_id).first()
    if not user:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")
    return user


@app.get("/internal/users/{user_id}", response_model=UserLookupOut)
def lookup_user(user_id: UUID, db: Session = Depends(get_db)) -> UserLookupOut:
    user = db.query(User).filter(User.id == user_id).first()
    if not user:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="User not found")
    return user
