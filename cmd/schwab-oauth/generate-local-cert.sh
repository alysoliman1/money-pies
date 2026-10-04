#!/bin/bash

# Generate a self-signed certificate for 127.0.0.1
echo "Generating self-signed certificate for 127.0.0.1..."

mkdir -p local-cert

openssl req -x509 -newkey rsa:4096 -keyout local-cert/key.pem -out local-cert/cert.pem -days 365 -nodes \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=127.0.0.1" \
  -addext "subjectAltName=IP:127.0.0.1"

echo ""
echo "Certificate generated successfully!"
echo "Files created: local-cert/cert.pem and local-cert/key.pem"