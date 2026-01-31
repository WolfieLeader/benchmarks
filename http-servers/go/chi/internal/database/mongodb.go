package database

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type userDocument struct {
	Id             bson.ObjectID `bson:"_id"`
	Name           string        `bson:"name"`
	Email          string        `bson:"email"`
	FavoriteNumber *int          `bson:"favoriteNumber,omitempty"`
}

type MongoRepository struct {
	client     *mongo.Client
	database   *mongo.Database
	collection *mongo.Collection
	url        string
	dbName     string
	once       sync.Once
	initErr    error
	mu         sync.Mutex
}

func NewMongoRepository(connectionString, dbName string) *MongoRepository {
	return &MongoRepository{url: connectionString, dbName: dbName}
}

func (r *MongoRepository) connect(ctx context.Context) error {
	r.once.Do(func() {
		client, err := mongo.Connect(options.Client().ApplyURI(r.url))
		if err != nil {
			r.initErr = err
			return
		}
		r.client = client
		r.database = client.Database(r.dbName)
		r.collection = r.database.Collection("users")
	})
	return r.initErr
}

func (r *MongoRepository) toUser(doc *userDocument) *User {
	user := &User{Id: doc.Id.Hex(), Name: doc.Name, Email: doc.Email}
	if doc.FavoriteNumber != nil {
		user.FavoriteNumber = doc.FavoriteNumber
	}
	return user
}

func (r *MongoRepository) parseObjectId(id string) (bson.ObjectID, bool) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, false
	}
	return oid, true
}

func (r *MongoRepository) Create(data *CreateUser) (*User, error) {
	ctx := context.Background()
	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	id := bson.NewObjectID()
	doc := userDocument{Id: id, Name: data.Name, Email: data.Email, FavoriteNumber: data.FavoriteNumber}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return nil, err
	}

	return BuildUser(id.Hex(), data), nil
}

func (r *MongoRepository) FindById(id string) (*User, error) {
	ctx := context.Background()
	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	oid, ok := r.parseObjectId(id)
	if !ok {
		return nil, nil
	}

	var doc userDocument
	err := r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return r.toUser(&doc), nil
}

func (r *MongoRepository) Update(id string, data *UpdateUser) (*User, error) {
	ctx := context.Background()
	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	oid, ok := r.parseObjectId(id)
	if !ok {
		return nil, nil
	}

	updateFields := bson.M{}
	if data.Name != nil {
		updateFields["name"] = *data.Name
	}
	if data.Email != nil {
		updateFields["email"] = *data.Email
	}
	if data.FavoriteNumber != nil {
		updateFields["favoriteNumber"] = *data.FavoriteNumber
	}

	if len(updateFields) == 0 {
		return r.FindById(id)
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var doc userDocument
	err := r.collection.FindOneAndUpdate(ctx, bson.M{"_id": oid}, bson.M{"$set": updateFields}, opts).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return r.toUser(&doc), nil
}

func (r *MongoRepository) Delete(id string) (bool, error) {
	ctx := context.Background()
	if err := r.connect(ctx); err != nil {
		return false, err
	}

	oid, ok := r.parseObjectId(id)
	if !ok {
		return false, nil
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return false, err
	}

	return result.DeletedCount > 0, nil
}

func (r *MongoRepository) DeleteAll() error {
	ctx := context.Background()
	if err := r.connect(ctx); err != nil {
		return err
	}

	_, err := r.collection.DeleteMany(ctx, bson.M{})
	return err
}

func (r *MongoRepository) HealthCheck() (bool, error) {
	ctx := context.Background()
	if err := r.connect(ctx); err != nil {
		return false, err
	}

	err := r.database.RunCommand(ctx, bson.M{"ping": 1}).Err()
	return err == nil, err
}

func (r *MongoRepository) Disconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		ctx := context.Background()
		err := r.client.Disconnect(ctx)
		r.client = nil
		r.database = nil
		r.collection = nil
		return err
	}
	return nil
}
