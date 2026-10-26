#!/bin/sh
set -e

# Run redis inside the container unless an external one is configured.
# Pub/sub keeps nothing on disk anyway, so persistence is turned off.
if [ -z "$REDIS_URL" ]; then
  redis-server --bind 127.0.0.1 --port 6379 --save "" --appendonly no --daemonize yes
fi

exec ./telemetrypulse
