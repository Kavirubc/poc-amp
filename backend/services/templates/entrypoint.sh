#!/bin/bash
# entrypoint.sh
# Wrapper entrypoint for AMP agent containers.
# 1. Sets up iptables transparent proxy rules
# 2. Executes the original agent command

set -e

# Run iptables init to set up transparent proxy
if [ -f /usr/local/bin/iptables-init.sh ]; then
    echo "[entrypoint] Setting up iptables transparent proxy..."
    /usr/local/bin/iptables-init.sh || echo "[entrypoint] WARNING: iptables setup failed, continuing without transparent proxy"
else
    echo "[entrypoint] No iptables-init.sh found, skipping transparent proxy setup"
fi

# Execute the original command
echo "[entrypoint] Starting agent process..."
exec "$@"
