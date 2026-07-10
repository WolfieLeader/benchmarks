from __future__ import annotations

import atexit
import logging
import time

from flask import Flask, Response, g, jsonify, request
from flask.typing import ResponseReturnValue
from werkzeug.exceptions import HTTPException, RequestEntityTooLarge

from bench_shared.consts import MAX_REQUEST_BYTES
from bench_shared.env import env
from bench_shared.errors import INTERNAL_ERROR, NOT_FOUND, REQUEST_TOO_LARGE, make_error
from src.blueprints import basic, db, params, web
from src.repositories.base import close_all

logger = logging.getLogger("flask.request")


# Error handlers — every failure goes through make_error so the fleet-wide
# {"error", "details"?} shape survives; no default Werkzeug HTML page reaches the
# wire. Params are typed `Exception` (the Flask ErrorHandlerCallable contract) and
# narrowed where a subtype's attributes are needed.
def _too_large(_: Exception) -> ResponseReturnValue:
    return jsonify(make_error(REQUEST_TOO_LARGE)), 413


def _http_error(exc: Exception) -> ResponseReturnValue:
    if not isinstance(exc, HTTPException):
        return jsonify(make_error(INTERNAL_ERROR)), 500
    code = exc.code or 500
    if code == 404:
        return jsonify(make_error(NOT_FOUND)), 404
    return jsonify(make_error(exc.name.lower() if exc.name else INTERNAL_ERROR)), code


def _unhandled(_: Exception) -> ResponseReturnValue:
    return jsonify(make_error(INTERNAL_ERROR)), 500


def _log_start() -> None:
    g.start_time = time.perf_counter()


def _log_after(response: Response) -> Response:
    start = g.get("start_time")
    if start is not None:
        elapsed_ms = (time.perf_counter() - start) * 1000
        logger.info("%s %s %s %.2fms", request.method, request.path, response.status_code, elapsed_ms)
    return response


def create_app() -> Flask:
    """Flask application factory (the WSGI entrypoint gunicorn calls per worker).

    Building the app inside a factory keeps DB pools out of the pre-fork master:
    each gunicorn worker calls create_app() after the fork, so no connection is
    ever shared across processes.
    """
    app = Flask(__name__)

    # Global request-body cap (fleet parity with py-fastapi/py-django): oversized
    # non-file bodies fail loud. The file route enforces its own smaller 1MB limit
    # for bodies under this cap.
    app.config["MAX_CONTENT_LENGTH"] = MAX_REQUEST_BYTES

    app.register_blueprint(basic.bp)
    app.register_blueprint(params.bp, url_prefix="/params")
    app.register_blueprint(db.bp, url_prefix="/db")
    app.register_blueprint(web.bp)

    app.register_error_handler(RequestEntityTooLarge, _too_large)
    app.register_error_handler(HTTPException, _http_error)
    app.register_error_handler(Exception, _unhandled)

    # Logger off in prod (fleet-wide env contract): request logging is installed
    # only under the dev profile, matching py-fastapi/py-django.
    if env.ENV == "dev":
        app.before_request(_log_start)
        app.after_request(_log_after)

    # Close DB pools on graceful worker shutdown (gunicorn drains, then exits).
    atexit.register(close_all)
    return app
