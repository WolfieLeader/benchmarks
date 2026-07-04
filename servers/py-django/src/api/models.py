from __future__ import annotations

from django.db import models


class UserModel(models.Model):
    # The `users` table is provisioned by the shared infra migration
    # (infra/databases/postgres/init.sql) — identical schema for every server in
    # the fleet — so this model is unmanaged: Django maps it, it does not own it.
    id = models.UUIDField(primary_key=True)
    name = models.CharField(max_length=255)
    email = models.CharField(max_length=255)
    favorite_number = models.IntegerField(null=True)

    class Meta:
        db_table = "users"
        managed = False
