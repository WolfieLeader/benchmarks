package com.bench.shared.db

import com.bench.shared.model.CreateUser
import com.bench.shared.model.UpdateUser
import com.bench.shared.model.User
import com.mongodb.client.model.Filters
import com.mongodb.client.model.FindOneAndUpdateOptions
import com.mongodb.client.model.ReturnDocument
import com.mongodb.client.model.Updates
import com.mongodb.kotlin.client.coroutine.MongoClient
import com.mongodb.kotlin.client.coroutine.MongoCollection
import com.mongodb.kotlin.client.coroutine.MongoDatabase
import kotlinx.coroutines.flow.firstOrNull
import org.bson.Document
import org.bson.types.ObjectId

/**
 * MongoDB repository — the official coroutine-native driver
 * (`mongodb-driver-kotlin-coroutine`: `insertOne` is a `suspend fun`, `find`
 * returns a `Flow`). Ids are `ObjectId` hex strings (the contract's `$id` matcher
 * accepts a UUID or an ObjectId). The client is built once with the driver default
 * pool (matching the Go/Rust mongo clients).
 */
class MongoRepository private constructor(
    private val client: MongoClient,
    private val database: MongoDatabase,
    private val collection: MongoCollection<Document>,
) : UserRepository {
    override suspend fun create(data: CreateUser): User {
        val oid = ObjectId()
        val document =
            Document("_id", oid)
                .append("name", data.name)
                .append("email", data.email)
        if (data.favoriteNumber != null) document.append("favoriteNumber", data.favoriteNumber)
        collection.insertOne(document)
        return User(oid.toHexString(), data.name, data.email, data.favoriteNumber)
    }

    override suspend fun findById(id: String): User? {
        val oid = parseObjectId(id) ?: return null
        return collection.find(Filters.eq("_id", oid)).firstOrNull()?.toUser()
    }

    override suspend fun update(
        id: String,
        data: UpdateUser,
    ): User? {
        val oid = parseObjectId(id) ?: return null
        if (data.isEmpty) return findById(id)
        val updates =
            buildList {
                data.name?.let { add(Updates.set("name", it)) }
                data.email?.let { add(Updates.set("email", it)) }
                data.favoriteNumber?.let { add(Updates.set("favoriteNumber", it)) }
            }
        val options = FindOneAndUpdateOptions().returnDocument(ReturnDocument.AFTER)
        return collection.findOneAndUpdate(Filters.eq("_id", oid), Updates.combine(updates), options)?.toUser()
    }

    override suspend fun delete(id: String): Boolean {
        val oid = parseObjectId(id) ?: return false
        return collection.deleteOne(Filters.eq("_id", oid)).deletedCount > 0
    }

    override suspend fun deleteAll() {
        collection.deleteMany(Filters.empty())
    }

    override suspend fun healthCheck(): Boolean {
        database.runCommand(Document("ping", 1))
        return true
    }

    override fun close() = client.close()

    companion object {
        fun connect(
            url: String,
            dbName: String,
        ): MongoRepository {
            val client = MongoClient.create(url)
            val database = client.getDatabase(dbName)
            return MongoRepository(client, database, database.getCollection<Document>("users"))
        }

        private fun parseObjectId(id: String): ObjectId? = if (ObjectId.isValid(id)) ObjectId(id) else null
    }
}

private fun Document.toUser(): User {
    val favoriteNumber =
        when (val value = get("favoriteNumber")) {
            is Int -> value
            is Long -> value.toInt()
            else -> null
        }
    return User(getObjectId("_id").toHexString(), getString("name"), getString("email"), favoriteNumber)
}
