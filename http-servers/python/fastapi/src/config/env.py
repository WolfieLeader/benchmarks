from __future__ import annotations

import ipaddress
import os
from typing import Literal

from dotenv import load_dotenv
from pydantic import BaseModel, ConfigDict, field_validator

load_dotenv()


class Env(BaseModel):
    model_config = ConfigDict(extra="ignore")

    ENV: Literal["dev", "prod"] = "dev"
    HOST: str = "0.0.0.0"
    PORT: int = 4001
    POSTGRES_URL: str = "postgres://postgres:postgres@localhost:5432/benchmarks"
    MONGODB_URL: str = "mongodb://localhost:27017"
    MONGODB_DB: str = "benchmarks"
    REDIS_URL: str = "redis://localhost:6379"
    CASSANDRA_CONTACT_POINTS: list[str] = ["localhost"]
    CASSANDRA_LOCAL_DATACENTER: str = "datacenter1"
    CASSANDRA_KEYSPACE: str = "benchmarks"

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
        raise ValueError("HOST must be a valid IP or 'localhost'")

    @field_validator("PORT", mode="before")
    @classmethod
    def parse_port(cls, value: str | int | None) -> int:
        msg = "PORT must be an integer between 1 and 65535"
        if isinstance(value, str):
            stripped = value.strip()
            if not stripped.isdigit():
                raise ValueError(msg)
            value = int(stripped)
        if not isinstance(value, int):
            raise ValueError(msg)
        return value

    @field_validator("PORT")
    @classmethod
    def validate_port(cls, value: int) -> int:
        if not 1 <= value <= 65535:
            raise ValueError("PORT must be an integer between 1 and 65535")
        return value

    @field_validator("CASSANDRA_CONTACT_POINTS", mode="before")
    @classmethod
    def parse_contact_points(cls, value: str | list[str] | None) -> list[str]:
        if value is None:
            return ["localhost"]
        if isinstance(value, list):
            return value
        return [cp.strip() for cp in value.split(",") if cp.strip()]


env = Env.model_validate(os.environ)
