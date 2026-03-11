#!/bin/bash
# entrypoint.sh
# Wrapper entrypoint for AMP agent containers.
# 1. Sets up iptables transparent proxy rules
# 2. Executes the original agent command

set -e

# Run iptables init to set up transparent proxy — fail closed if missing or failing
if [ -f /usr/local/bin/iptables-init.sh ]; then
    echo "[entrypoint] Setting up iptables transparent proxy..."
    /usr/local/bin/iptables-init.sh
else
    echo "[entrypoint] ERROR: iptables-init.sh missing; refusing to start without transparent proxy" >&2
    exit 1
fi

# Execute the original command
echo "[entrypoint] Starting agent process..."
exec "$@"
