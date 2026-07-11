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
    # S104: binding all interfaces is the deliberate container default for every
    # server in the fleet (the HOST env contract) — not an accidental exposure.
    HOST: str = "0.0.0.0"  # noqa: S104
    PORT: int = 8080
    POSTGRES_URL: str = "postgres://postgres:postgres@localhost:20001/benchmarks"
    MONGODB_URL: str = "mongodb://localhost:20002"
    MONGODB_DB: str = "benchmarks"
    REDIS_URL: str = "redis://localhost:20003"
    CASSANDRA_CONTACT_POINTS: list[str] = ["localhost:20004"]
    CASSANDRA_LOCAL_DATACENTER: str = "datacenter1"
    CASSANDRA_KEYSPACE: str = "benchmarks"
    # Shared HS256 secret for the web suite (/jwt/sign, /jwt/verify). The dev
    # default must match the other languages' shared env modules and the contract
    # harness (scripts/contract.mts JWT_SECRET / conformance.DefaultJWTSecret).
    JWT_SECRET: str = "benchmarks-shared-jwt-secret-dev-default"  # noqa: S105  (shared dev default secret, not a real credential)

    @field_validator("HOST", mode="before")
    @classmethod
    def normalize_host(cls, value: str | None) -> str:
        trimmed = (value or "").strip()
        return trimmed or "0.0.0.0"  # noqa: S104  (deliberate bind-all default, see HOST field)

    @field_validator("HOST")
    @classmethod
    def validate_host(cls, value: str) -> str:
        if value == "localhost":
            return "0.0.0.0"  # noqa: S104  (deliberate bind-all default, see HOST field)
        try:
            ipaddress.ip_address(value)
        except ValueError:
            raise ValueError("HOST must be a valid IP or 'localhost'") from None
        return value

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
