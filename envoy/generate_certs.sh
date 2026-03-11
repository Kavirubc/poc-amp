#!/bin/bash
set -e

mkdir -p certs
cd certs

echo "Generating CA..."
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=AMP Proxy CA"

echo "Generating Server Key..."
openssl genrsa -out server.key 2048

echo "Generating Certificate Signing Request (CSR)..."
cat > openssl.cnf <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
req_extensions = req_ext
distinguished_name = dn

[dn]
CN = AMP Proxy Wildcard

[req_ext]
subjectAltName = @alt_names

[alt_names]
# Wildcard entries for broad interception coverage
DNS.1 = *
DNS.2 = *.*
DNS.3 = *.*.*
DNS.4 = *.*.*.*
# Common AI/API providers (explicit for client compatibility)
DNS.5 = localhost
DNS.6 = *.openai.com
DNS.7 = *.anthropic.com
DNS.8 = *.googleapis.com
DNS.9 = *.google.com
DNS.10 = *.stripe.com
DNS.11 = *.github.com
DNS.12 = *.httpbin.org
DNS.13 = *.amazonaws.com
DNS.14 = *.azure.com
DNS.15 = *.huggingface.co
EOF

openssl req -new -key server.key -out server.csr -config openssl.cnf

echo "Signing Server Certificate with CA..."
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 365 -extensions req_ext -extfile openssl.cnf

echo "Certificates generated successfully in ./certs/"
ls -la
