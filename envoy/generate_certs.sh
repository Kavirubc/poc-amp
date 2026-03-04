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
CN = *

[req_ext]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = httpbin.org
DNS.3 = *.httpbin.org
DNS.4 = api.openai.com
DNS.5 = *.openai.com
DNS.6 = api.github.com
DNS.7 = *.github.com
DNS.8 = *.stripe.com
EOF

openssl req -new -key server.key -out server.csr -config openssl.cnf

echo "Signing Server Certificate with CA..."
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 365 -extensions req_ext -extfile openssl.cnf

echo "Certificates generated successfully in ./certs/"
ls -la
