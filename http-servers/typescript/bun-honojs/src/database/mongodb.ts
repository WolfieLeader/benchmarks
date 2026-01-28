import { MongoClient, ObjectId } from "mongodb";
import type { CreateUser, UpdateUser, User, UserRepository } from "./repo";

type UserDocument = {
  _id: ObjectId;
  name: string;
  email: string;
};

export class MongoUserRepository implements UserRepository {
  private client: MongoClient;
  private dbName: string;
  private ready: Promise<void> | null = null;

  constructor(connectionString: string, dbName: string) {
    this.client = new MongoClient(connectionString);
    this.dbName = dbName;
  }

  private async connect(): Promise<void> {
    if (!this.ready) {
      this.ready = this.client.connect().then(() => undefined);
    }
    await this.ready;
  }

  private collection() {
    return this.client.db(this.dbName).collection<UserDocument>("users");
  }

  async create(data: CreateUser): Promise<User> {
    await this.connect();
    const id = new ObjectId();
    await this.collection().insertOne({ _id: id, name: data.name, email: data.email });
    return { id: id.toString(), name: data.name, email: data.email };
  }

  async findById(id: string): Promise<User | null> {
    await this.connect();
    let objectId: ObjectId;
    try {
      objectId = new ObjectId(id);
    } catch {
      return null;
    }

    const user = await this.collection().findOne({ _id: objectId });
    if (!user) {
      return null;
    }

    return { id: user._id.toString(), name: user.name, email: user.email };
  }

  async update(id: string, data: UpdateUser): Promise<User | null> {
    await this.connect();
    let objectId: ObjectId;
    try {
      objectId = new ObjectId(id);
    } catch {
      return null;
    }

    const user = await this.collection().findOneAndUpdate(
      { _id: objectId },
      { $set: { name: data.name, email: data.email } },
      { returnDocument: "after" },
    );

    if (!user) {
      return null;
    }

    return { id: user._id.toString(), name: user.name, email: user.email };
  }

  async delete(id: string): Promise<boolean> {
    await this.connect();
    let objectId: ObjectId;
    try {
      objectId = new ObjectId(id);
    } catch {
      return false;
    }

    const result = await this.collection().deleteOne({ _id: objectId });
    return result.deletedCount > 0;
  }

  async deleteAll(): Promise<void> {
    await this.connect();
    await this.collection().deleteMany({});
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
  }
}
