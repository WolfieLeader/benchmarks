from __future__ import annotations

from bench_shared.env import env
from src.app import create_app


def main() -> None:
    # Dev server only (Werkzeug). Production / benchmark runs under gunicorn — see
    # gunicorn.conf.py and the Dockerfile CMD. threaded=True mirrors the sync
    # thread-pool concurrency the prod server uses.
    app = create_app()
    app.run(host=env.HOST, port=env.PORT, threaded=True)


if __name__ == "__main__":
    main()
