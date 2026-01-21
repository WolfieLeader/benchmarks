from __future__ import annotations

import ipaddress
import os
from typing import Literal
from urllib.parse import urlparse

from dotenv import load_dotenv
from pydantic import BaseModel, ConfigDict, field_validator

load_dotenv()


def _is_url(value: str) -> bool:
    try:
        parsed = urlparse(value)
    except ValueError:
        return False
    return bool(parsed.scheme and parsed.netloc)


class Env(BaseModel):
    model_config = ConfigDict(extra="ignore")

    ENV: Literal["dev", "prod"] = "dev"
    HOST: str = "0.0.0.0"
    PORT: int = 4001

    @field_validator("HOST", mode="before")
    @classmethod
    def normalize_host(cls, value: str | None) -> str:
        trimmed = (value or "").strip()
        return trimmed or "0.0.0.0"

    @field_validator("HOST")
    @classmethod
    def validate_host(cls, value: str) -> str:
        if value == "localhost":
            return "0.0.0.0"
        try:
            ipaddress.ip_address(value)
            return value
        except ValueError:
            pass
        if _is_url(value):
            return value
        raise ValueError("HOST must be a valid URL, IP, or 'localhost'")

    @field_validator("PORT", mode="before")
    @classmethod
    def parse_port(cls, value: str | int | None) -> int:
        if value is None:
            raise ValueError("PORT must be an integer between 1 and 65535")
        if isinstance(value, str):
            value = value.strip()
            if value == "":
                raise ValueError("PORT must be an integer between 1 and 65535")
            if not value.isdigit():
                raise ValueError("PORT must be an integer between 1 and 65535")
            value = int(value)
        if not isinstance(value, int):
            raise ValueError("PORT must be an integer between 1 and 65535")
        return value

    @field_validator("PORT")
    @classmethod
    def validate_port(cls, value: int) -> int:
        if value < 1 or value > 65535:
            raise ValueError("PORT must be an integer between 1 and 65535")
        return value


env = Env.model_validate(os.environ)
