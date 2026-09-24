#!/bin/sh
# Starts the bundled Redis when REDIS_HOST is unset or local, then runs the app with the given
# arguments. Set REDIS_HOST to an external Redis to skip the bundled one (needed when the API and
# the worker run as separate services, since each container would otherwise get its own Redis).
set -e

# Accept a command written in full (e.g. Render's Docker Command "/app/audit-app -worker"):
# only the flags are the app's arguments
case "$1" in
  */audit-app|audit-app|*/docker-entrypoint.sh) shift ;;
esac

case "${REDIS_HOST:-localhost:6379}" in
  localhost:*|127.0.0.1:*|"[::1]":*)
    # IPv4 on purpose: "localhost" can resolve to ::1, where Redis is not listening
    export REDIS_HOST="127.0.0.1:6379"

    # Local only, in memory: queued jobs do not survive a restart, and the periodic ones are
    # re-scheduled when the worker starts
    if [ -n "$REDIS_PASSWORD" ]; then
      redis-server --bind 127.0.0.1 --port 6379 --dir /tmp --save "" --appendonly no \
        --daemonize yes --requirepass "$REDIS_PASSWORD"
    else
      redis-server --bind 127.0.0.1 --port 6379 --dir /tmp --save "" --appendonly no \
        --daemonize yes
    fi

    # Wait until Redis answers before starting the app
    tries=0
    until REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1; do
      tries=$((tries + 1))
      if [ "$tries" -ge 50 ]; then
        echo "Bundled Redis did not start" >&2
        exit 1
      fi
      sleep 0.2
    done
    echo "Bundled Redis is up on $REDIS_HOST"
    ;;
  *)
    echo "Using external Redis at $REDIS_HOST"
    ;;
esac

# exec so the app receives SIGTERM from Docker/Render and shuts down cleanly
exec /app/audit-app "$@"
