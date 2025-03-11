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
*   **`SaveEOTSKeyName`**

The following gRPC method does *not* require HMAC authentication:

*   **`Ping`**: Used for health checks and basic connectivity testing.

**Key Management Note:**  Key *creation* (e.g., using the `eotsd keys add` command) is handled locally by `eotsd` 
and does *not* use gRPC.  Therefore, key creation does *not* use or require HMAC authentication.

## Configuring HMAC Authentication

HMAC authentication is strongly recommended for production deployments.  It relies on a shared secret key 
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

### 1. FPD (fpd.conf - Recommended):
The preferred method is to set the hmackey option within your fpd.conf file:

```
[metrics]
# ... other settings ...

hmackey=Wt+Nxkn1DpNCFJtxQSxTKoSoKzx1C9XwHTMbT6ir9m0=  # Your base64 encoded key
```

### 2. EOTSD (Environment Variable - Required):

EOTSD exclusively reads the HMAC key from the HMAC_KEY environment variable. There is no configuration file 
option for EOTSD. You must set this:

```
export HMAC_KEY="Wt+Nxkn1DpNCFJtxQSxTKoSoKzx1C9XwHTMbT6ir9m0="  # MUST match the FPD key, base64 encoded
```


### Important Considerations:

- Consistency: The HMAC key must be identical for both FPD and EOTSD.

- fpd.conf Priority: If hmackey is set in fpd.conf, the HMAC_KEY environment variable is ignored by FPD. 
The configuration file takes precedence.

- Security: Never commit the HMAC key to version control. Store it securely, treating it like a private key.

- If No Key is Provided: If no key is set (either via the config or via the environment), fpd will still start, 
but with gRPC authentication turned off.


## Deployment Best Practices

Separate Machines: For maximum security, run FPD and EOTSD on separate machines. Restrict network access to the EOTSD
machine, allowing connections only from the FPD instance.

Key Rotation: Rotate the HMAC key periodically (e.g., every few months). Generate a new key, update the fpd.conf 
file (and/or the HMAC_KEY environment variable), and restart both services. This requires a brief downtime.

## Troubleshooting

If you encounter HMAC authentication errors:

1. Key Mismatch: The most common cause is a difference between the HMAC keys configured for FPD and EOTSD. 
Double-check that they are identical, including any leading or trailing whitespace.

2. Environment Variable Issues:

   - Ensure the HMAC_KEY environment variable is correctly set in the environment where eotsd is running.

   - If using systemd or a similar service manager, make sure the environment variable is set within the service 
   definition, not just in your interactive shell.

   - If using fpd.conf, make sure the hmackey is set correctly, and that no HMAC_KEY environment variable with 
   an incorrect value is present (as it will be ignored if hmackey is set).

3. Logs: Check the logs of both FPD and EOTSD for error messages related to HMAC. Look for "invalid HMAC signature."

4. Connectivity: Use the Ping method (which bypasses HMAC) to test basic network connectivity between FPD and EOTSD.

5. Restart: After making any configuration changes, restart both FPD and EOTSD.

By following these guidelines, you can secure the communication between FPD and EOTSD, protecting your finality 
provider from unauthorized signature requests and potential slashing events.