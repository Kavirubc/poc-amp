#!/bin/bash
# iptables-init.sh
# Sets up iptables DNAT rules inside agent containers to transparently
# redirect outbound HTTPS traffic to the Envoy proxy for interception.
#
# This script runs as part of the agent container entrypoint before the
# agent process starts. It requires NET_ADMIN capability on the container.

set -e

ENVOY_PORT=${ENVOY_TRANSPARENT_PORT:-10443}
ENVOY_RETRIES=${ENVOY_WAIT_SECONDS:-30}

# Resolve Envoy's IP on the Docker network with retry to handle startup races
ENVOY_IP=""
for _ in $(seq 1 "$ENVOY_RETRIES"); do
    ENVOY_IP=$(getent hosts envoy | awk '{print $1; exit}')
    [ -n "$ENVOY_IP" ] && break
    sleep 1
done

if [ -z "$ENVOY_IP" ]; then
    echo "[iptables-init] ERROR: Could not resolve 'envoy' after ${ENVOY_RETRIES}s; refusing to start without transparent proxy." >&2
    exit 1
fi

echo "[iptables-init] Envoy resolved to ${ENVOY_IP}"
echo "[iptables-init] Redirecting outbound HTTPS (port 443) -> ${ENVOY_IP}:${ENVOY_PORT}"

# Create a custom chain for AMP traffic management
iptables -t nat -N AMP_OUTPUT 2>/dev/null || true
iptables -t nat -F AMP_OUTPUT

# Hook into the OUTPUT chain
iptables -t nat -C OUTPUT -p tcp -j AMP_OUTPUT 2>/dev/null || \
    iptables -t nat -A OUTPUT -p tcp -j AMP_OUTPUT

# --- Exclusions (traffic that should NOT be redirected) ---

# Don't redirect loopback traffic
iptables -t nat -A AMP_OUTPUT -o lo -j RETURN

# Don't redirect traffic to Docker internal networks (backend, DB, etc.)
iptables -t nat -A AMP_OUTPUT -d 10.0.0.0/8 -j RETURN
iptables -t nat -A AMP_OUTPUT -d 172.16.0.0/12 -j RETURN
iptables -t nat -A AMP_OUTPUT -d 192.168.0.0/16 -j RETURN

# Don't redirect traffic to Envoy itself (prevent loops)
iptables -t nat -A AMP_OUTPUT -d "$ENVOY_IP" -j RETURN

# Don't redirect DNS traffic
iptables -t nat -A AMP_OUTPUT -p tcp --dport 53 -j RETURN

# --- Redirect rules ---

# Redirect all outbound HTTPS (port 443) to Envoy's transparent listener
iptables -t nat -A AMP_OUTPUT -p tcp --dport 443 -j DNAT --to-destination "${ENVOY_IP}:${ENVOY_PORT}"

# Optionally redirect HTTP (port 80) for non-TLS interception
# Uncomment for full HTTP interception:
# iptables -t nat -A AMP_OUTPUT -p tcp --dport 80 -j DNAT --to-destination "${ENVOY_IP}:${ENVOY_HTTP_PORT:-10080}"

echo "[iptables-init] iptables rules applied successfully:"
iptables -t nat -L AMP_OUTPUT -n -v
