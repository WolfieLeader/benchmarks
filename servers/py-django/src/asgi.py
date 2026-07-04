import os

from django.core.asgi import get_asgi_application

# get_asgi_application() reads DJANGO_SETTINGS_MODULE at call time (not import
# time), so setting it before the call keeps every import at module top (no E402).
os.environ.setdefault("DJANGO_SETTINGS_MODULE", "src.settings")

application = get_asgi_application()
