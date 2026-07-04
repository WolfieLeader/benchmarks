//! `MongoDB` repository (official `mongodb` driver).
//!
//! Ids are `ObjectId` hex strings (the contract's `$id` matcher accepts a UUID or
//! an `ObjectId`). The `Client` is internally `Arc`-shared and cheap to clone; it is
//! built once at startup with the driver's default pool (matching the Go/TS mongo
//! clients, which also use the driver default).

use mongodb::bson::{Bson, Document, doc, oid::ObjectId};
use mongodb::options::ReturnDocument;
use mongodb::{Client, Collection, Database};

use crate::error::DbError;
use crate::model::{CreateUser, UpdateUser, User};

pub struct MongoRepo {
    db: Database,
    collection: Collection<Document>,
}

impl MongoRepo {
    pub async fn connect(url: &str, db_name: &str) -> Result<Self, DbError> {
        let client = Client::with_uri_str(url).await?;
        let db = client.database(db_name);
        let collection = db.collection::<Document>("users");
        Ok(Self { db, collection })
    }

    pub async fn create(&self, data: &CreateUser) -> Result<User, DbError> {
        let oid = ObjectId::new();
        let mut document =
            doc! { "_id": oid, "name": data.name.clone(), "email": data.email.clone() };
        if let Some(n) = data.favorite_number {
            document.insert("favoriteNumber", n);
        }
        self.collection.insert_one(document).await?;
        Ok(User::from_create(oid.to_hex(), data))
    }

    pub async fn find_by_id(&self, id: &str) -> Result<Option<User>, DbError> {
        let Ok(oid) = ObjectId::parse_str(id) else {
            return Ok(None);
        };
        let found = self.collection.find_one(doc! { "_id": oid }).await?;
        Ok(found.as_ref().and_then(doc_to_user))
    }

    pub async fn update(&self, id: &str, data: &UpdateUser) -> Result<Option<User>, DbError> {
        let Ok(oid) = ObjectId::parse_str(id) else {
            return Ok(None);
        };
        if data.is_empty() {
            return self.find_by_id(id).await;
        }
        let mut set = Document::new();
        if let Some(name) = &data.name {
            set.insert("name", name.clone());
        }
        if let Some(email) = &data.email {
            set.insert("email", email.clone());
        }
        if let Some(n) = data.favorite_number {
            set.insert("favoriteNumber", n);
        }
        let updated = self
            .collection
            .find_one_and_update(doc! { "_id": oid }, doc! { "$set": set })
            .return_document(ReturnDocument::After)
            .await?;
        Ok(updated.as_ref().and_then(doc_to_user))
    }

    pub async fn delete(&self, id: &str) -> Result<bool, DbError> {
        let Ok(oid) = ObjectId::parse_str(id) else {
            return Ok(false);
        };
        let result = self.collection.delete_one(doc! { "_id": oid }).await?;
        Ok(result.deleted_count > 0)
    }

    pub async fn delete_all(&self) -> Result<(), DbError> {
        self.collection.delete_many(doc! {}).await?;
        Ok(())
    }

    pub async fn health_check(&self) -> Result<bool, DbError> {
        self.db.run_command(doc! { "ping": 1 }).await?;
        Ok(true)
    }
}

fn doc_to_user(document: &Document) -> Option<User> {
    let id = document.get_object_id("_id").ok()?.to_hex();
    let name = document.get_str("name").ok()?.to_string();
    let email = document.get_str("email").ok()?.to_string();
    let favorite_number = match document.get("favoriteNumber") {
        Some(Bson::Int32(n)) => Some(*n),
        Some(Bson::Int64(n)) => i32::try_from(*n).ok(),
        _ => None,
    };
    Some(User {
        id,
        name,
        email,
        favorite_number,
    })
}
