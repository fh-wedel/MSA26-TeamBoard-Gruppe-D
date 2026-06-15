#!/usr/bin/env python3
"""Seed script — works on Windows, Mac, and Linux without shell quoting issues."""
import json
import os
import sys
import urllib.error
import urllib.request

API = os.environ.get("API_BASE_URL", "http://localhost")
ALICE_PASSWORD = os.environ.get("SEED_ALICE_PASSWORD", "AliceSecret123!")
BOB_PASSWORD = os.environ.get("SEED_BOB_PASSWORD", "BobSecret123!")


def post(path, body, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(
        f"{API}{path}",
        data=json.dumps(body).encode(),
        headers=headers,
        method="POST",
    )
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        body_bytes = e.read()
        try:
            return json.loads(body_bytes)
        except Exception:
            return {"error": e.code, "body": body_bytes.decode(errors="replace")}


print("Creating users...")

alice = post("/api/v1/auth/register", {"email": "alice@teamboard.local", "password": ALICE_PASSWORD})
print(f"  alice: {alice.get('data', {}).get('id', alice)}")

bob = post("/api/v1/auth/register", {"email": "bob@teamboard.local", "password": BOB_PASSWORD})
print(f"  bob: {bob.get('data', {}).get('id', bob)}")

print("\nLogging in as Alice...")
login = post("/api/v1/auth/login", {"email": "alice@teamboard.local", "password": ALICE_PASSWORD})
token = login.get("data", {}).get("access_token")

if not token:
    print(f"Login response: {login}")
    print("ERROR: Could not get Alice's token. Is the auth service running?")
    sys.exit(1)

print("Creating project 'Demo Project'...")
proj = post(
    "/api/v1/projects",
    {"name": "Demo Project", "description": "Created by seed script"},
    token=token,
)
print(f"  project: {proj.get('data', {}).get('id', proj)}")

print("\nSeed complete.")
print(f"  Login: alice@teamboard.local / {ALICE_PASSWORD}")
print(f"  Login: bob@teamboard.local / {BOB_PASSWORD}")
