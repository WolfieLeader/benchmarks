from django.urls import path

from src.api.views import basic, db, params

urlpatterns = [
    path("", basic.root),
    path("health", basic.health),
    path("params/search", params.search_params),
    path("params/url/<str:dynamic>", params.url_params),
    path("params/header", params.header_params),
    path("params/body", params.body_params),
    path("params/cookie", params.cookie_params),
    path("params/form", params.form_params),
    path("params/file", params.file_params),
    path("db/<str:database>/health", db.database_health),
    path("db/<str:database>/reset", db.reset_database),
    path("db/<str:database>/users", db.UsersCollectionView.as_view()),
    path("db/<str:database>/users/<str:user_id>", db.UserDetailView.as_view()),
]
