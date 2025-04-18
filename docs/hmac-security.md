# HMAC Security for Finality Provider

This document describes the HMAC (Hash-based Message Authentication Code) authentication system used to secure the
communication between the Finality Provider Daemon (FPD) and the Extractable One-Time Signature Daemon (EOTSD).
HMAC ensures that only authorized requests from FPD are processed by EOTSD, which handles sensitive key operations.

## Overview

The Finality Provider (FPD) and EOTS Manager (EOTSD) are separate components. EOTSD manages EOTS keys,
generates randomness, and signs EOTS signatures and schnorr signatures, making it a critical security target.
HMAC authentication adds a layer of protection, preventing unauthorized access to these signing capabilities.

## Security Classification of EOTSD Messages

The following gRPC methods exposed by EOTSD *require* HMAC authentication:

*   **`SignEOTS`**
*   **`SignSchnorrSig`**
*   **`CreateRandomnessPairList`**

The following gRPC method does *not* require HMAC authentication:

*   **`Ping`**: Used for health checks and basic connectivity testing.

**Key Management Note:**  Key *creation* (e.g., using the `eotsd keys add` command) is handled locally by `eotsd` 
and does *not* use gRPC.  Therefore, key creation does *not* use or require HMAC authentication.

## Configuring HMAC Authentication

HMAC authentication is strongly recommended for production deployments. It relies on a shared secret key 
(the HMAC key) known to both FPD and EOTSD.

### Generating a Strong HMAC Key

Generate a cryptographically secure random key (32 bytes is recommended) and encode it using base64.  
You can use the `openssl` command:

```bash
# Generate a 32-byte random key and encode it as base64
openssl rand -base64 32
```

Example output: Wt+Nxkn1DpNCFJtxQSxTKoSoKzx1C9XwHTMbT6ir9m0= (Your key will be different).

## Configuration Methods

### 1. FPD Configuration (fpd.conf)

Set the hmackey option within your fpd.conf file:

```
[metrics]
# ... other settings ...

hmackey=Wt+Nxkn1DpNCFJtxQSxTKoSoKzx1C9XwHTMbT6ir9m0=  # Your base64 encoded key
```

### 2. EOTSD Configuration (eotsd.conf):

Set the hmackey option within your eotsd.conf file:

```
hmackey=Wt+Nxkn1DpNCFJtxQSxTKoSoKzx1C9XwHTMbT6ir9m0=  # MUST match the FPD key, base64 encoded
```

### Important Considerations:

- Consistency: The HMAC key must be identical for both FPD and EOTSD.

- Security: Never commit the HMAC key to version control. Store it securely, treating it like a private key.

- If No Key is Provided: If no key is set in the configuration, the services will still start, but with gRPC authentication turned off. This is not recommended for production environments.

## Deployment Best Practices

Separate Machines: For maximum security, run FPD and EOTSD on separate machines. Restrict network access to the EOTSD
machine, allowing connections only from the FPD instance.

Key Rotation: Rotate the HMAC key periodically (e.g., every few months). Generate a new key, update the configuration
files, and restart both services. This requires a brief downtime.

## Troubleshooting

If you encounter HMAC authentication errors:

1. Key Mismatch: The most common cause is a difference between the HMAC keys configured for FPD and EOTSD. 
Double-check that they are identical, including any leading or trailing whitespace.

2. Configuration Issues: Ensure that the HMAC key is properly set in both configuration files.

3. Cloud Secret References: If you're using cloud secret references (AWS, GCP, Azure), ensure they are properly formatted and accessible.