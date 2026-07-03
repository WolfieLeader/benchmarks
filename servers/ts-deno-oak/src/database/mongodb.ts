import { type Collection, MongoClient, ObjectId } from "mongodb";
import type { UserRepository } from "./repository.ts";
import { buildUser, type CreateUser, type UpdateUser, type User } from "./types.ts";

type UserDocument = {
  _id: ObjectId;
  name: string;
  email: string;
  favoriteNumber?: number;
};

export class MongoUserRepository implements UserRepository {
  private client: MongoClient;
  private dbName: string;
  private connected = false;
  private collection: Collection<UserDocument>;

  constructor(connectionString: string, dbName: string) {
    this.client = new MongoClient(connectionString);
    this.dbName = dbName;
    this.collection = this.client.db(this.dbName).collection<UserDocument>(
      "users"
    );
  }

  private async connect(): Promise<void> {
    if (this.connected) return;
    await this.client.connect();
    this.connected = true;
  }

  private parseObjectId(id: string): ObjectId | null {
    try {
      return new ObjectId(id);
    } catch {
      return null;
    }
  }

  private toUser(doc: UserDocument): User {
    const user: User = {
      id: doc._id.toString(),
      name: doc.name,
      email: doc.email
    };
    if (doc.favoriteNumber !== undefined) {
      user.favoriteNumber = doc.favoriteNumber;
    }
    return user;
  }

  async create(data: CreateUser): Promise<User> {
    await this.connect();
    const id = new ObjectId();
    const doc: UserDocument = { _id: id, name: data.name, email: data.email };
    if (data.favoriteNumber !== undefined) {
      doc.favoriteNumber = data.favoriteNumber;
    }
    await this.collection.insertOne(doc);
    return buildUser(id.toString(), data);
  }

  async findById(id: string): Promise<User | null> {
    await this.connect();
    const objectId = this.parseObjectId(id);
    if (!objectId) return null;

    const doc = await this.collection.findOne({ _id: objectId });
    return doc ? this.toUser(doc) : null;
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.connect();
    const objectId = this.parseObjectId(id);
    if (!objectId) return null;

    const updateFields: Record<string, unknown> = {};
    if (data.name !== undefined) updateFields.name = data.name;
    if (data.email !== undefined) updateFields.email = data.email;
    if (data.favoriteNumber !== undefined) {
      updateFields.favoriteNumber = data.favoriteNumber;
    }

    if (Object.keys(updateFields).length === 0) {
      return this.findById(id);
    }

    const doc = await this.collection.findOneAndUpdate(
      { _id: objectId },
      { $set: updateFields },
      { returnDocument: "after" }
    );
    return doc ? this.toUser(doc) : null;
  }

  async delete(id: string): Promise<boolean> {
    await this.connect();
    const objectId = this.parseObjectId(id);
    if (!objectId) return false;

    const result = await this.collection.deleteOne({ _id: objectId });
    return result.deletedCount > 0;
  }

  async deleteAll(): Promise<void> {
    await this.connect();
    await this.collection.deleteMany({});
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.connect();
      await this.client.db(this.dbName).command({ ping: 1 });
      return true;
    } catch {
      return false;
    }
  }

  async disconnect(): Promise<void> {
    await this.client.close();
    this.connected = false;
  }
}
