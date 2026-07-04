import os

import uvicorn

from bench_shared.env import env


def main() -> None:
    os.environ.setdefault("DJANGO_SETTINGS_MODULE", "src.settings")
    uvicorn.run("src.asgi:application", host=env.HOST, port=env.PORT)


if __name__ == "__main__":
    main()
