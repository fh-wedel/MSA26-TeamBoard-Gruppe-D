#!/usr/bin/env python3
import subprocess, time, sys, json

for attempt in range(24):
    result = subprocess.run(
        ["docker", "compose", "ps", "--format", "json"],
        capture_output=True, text=True
    )
    services = [json.loads(l) for l in result.stdout.splitlines() if l.strip()]
    unhealthy = [s["Name"] for s in services if s.get("Health", "") not in ("", "healthy")]
    if not unhealthy:
        print("All services healthy.")
        sys.exit(0)
    print(f"Waiting... ({attempt + 1}/24) — not yet healthy: {unhealthy}")
    time.sleep(5)

print("Some services not healthy after 120s. Run 'docker compose ps' and 'docker compose logs'.")
sys.exit(1)
