"""Sync DB user-repository cores shared by the sync Python servers (py-flask
directly from gthread handlers, py-django via in-server sync_to_async bridges).

Import the per-driver module directly — nothing is re-exported here, so a
server that never touches a given DB never pays that driver's import.
"""
