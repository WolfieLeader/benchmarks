from __future__ import annotations

from bson import ObjectId
from motor.motor_asyncio import AsyncIOMotorClient

from src.database.types import CreateUser, UpdateUser, User, build_user


class MongoUserRepository:
    def __init__(self, connection_string: str, db_name: str):
        self._url = connection_string
        self._db_name = db_name
        self._client: AsyncIOMotorClient | None = None

    def _connect(self) -> None:
        if self._client is not None:
            return
        self._client = AsyncIOMotorClient(self._url)

    def _parse_object_id(self, id: str) -> ObjectId | None:
        try:
            return ObjectId(id)
        except Exception:
            return None

    def _to_user(self, doc: dict) -> User:
        return User(
            id=str(doc["_id"]),
            name=doc["name"],
            email=doc["email"],
            favoriteNumber=doc.get("favoriteNumber"),
        )

    async def create(self, data: CreateUser) -> User:
        self._connect()
        assert self._client is not None

        collection = self._client[self._db_name]["users"]
        id = ObjectId()
        doc = {"_id": id, "name": data.name, "email": data.email}
        if data.favoriteNumber is not None:
            doc["favoriteNumber"] = data.favoriteNumber

        await collection.insert_one(doc)
        return build_user(str(id), data)

    async def find_by_id(self, id: str) -> User | None:
        self._connect()
        assert self._client is not None

        oid = self._parse_object_id(id)
        if oid is None:
            return None

        collection = self._client[self._db_name]["users"]
        doc = await collection.find_one({"_id": oid})
        if doc is None:
            return None
        return self._to_user(doc)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        self._connect()
        assert self._client is not None

        oid = self._parse_object_id(id)
        if oid is None:
            return None

        update_fields = {}
        if data.name is not None:
            update_fields["name"] = data.name
        if data.email is not None:
            update_fields["email"] = data.email
        if data.favoriteNumber is not None:
            update_fields["favoriteNumber"] = data.favoriteNumber

        if not update_fields:
            return await self.find_by_id(id)

        collection = self._client[self._db_name]["users"]
        doc = await collection.find_one_and_update({"_id": oid}, {"$set": update_fields}, return_document=True)
        if doc is None:
            return None
        return self._to_user(doc)

    async def delete(self, id: str) -> bool:
        self._connect()
        assert self._client is not None

        oid = self._parse_object_id(id)
        if oid is None:
            return False

        collection = self._client[self._db_name]["users"]
        result = await collection.delete_one({"_id": oid})
        return result.deleted_count > 0

    async def delete_all(self) -> None:
        self._connect()
        assert self._client is not None

        collection = self._client[self._db_name]["users"]
        await collection.delete_many({})

    async def health_check(self) -> bool:
        try:
            self._connect()
            assert self._client is not None
            await self._client.admin.command("ping")
            return True
        except Exception:
            return False

    async def disconnect(self) -> None:
        if self._client is not None:
            self._client.close()
            self._client = None
