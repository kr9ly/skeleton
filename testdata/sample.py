from typing import Optional, List
from dataclasses import dataclass
import os
import json
from pathlib import Path
from . import utils
from ..core import BaseService

__all__ = ["UserService", "create_user", "Role"]

Role = str  # type alias (uppercase, will be captured)

@dataclass
class User:
    id: int
    name: str
    email: Optional[str] = None

class UserService(BaseService):
    def __init__(self, db: "Database") -> None:
        self.db = db

    def find_by_id(self, user_id: int) -> Optional[User]:
        return self.db.query(User).filter_by(id=user_id).first()

    def _internal_method(self) -> None:
        pass

def create_user(name: str, email: Optional[str] = None) -> User:
    """Create a new user."""
    return User(id=generate_id(), name=name, email=email)

def list_users(
    limit: int = 100,
    offset: int = 0,
    active_only: bool = True,
) -> List[User]:
    pass

def _private_helper() -> None:
    pass

MAX_RETRIES: int = 3
DEFAULT_TIMEOUT = 30

_internal_state = {}
