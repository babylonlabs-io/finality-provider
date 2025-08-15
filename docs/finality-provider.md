# Finality Provider

## 1. Overview

The Finality Provider Daemon is responsible for monitoring for new Babylon blocks,
committing public randomness for the blocks it intends to provide finality signatures
for, and submitting finality signatures.

The daemon can manage and perform the following operations for multiple finality
providers:

1. **Creation and Registration**: Creates and registers finality providers to
   Babylon.
2. **EOTS Randomness Commitment**: The daemon monitors the Babylon chain and commits
   EOTS public randomness for every Babylon block each finality provider intends to
   vote for. The commit intervals can be specified in the configuration. The EOTS
   public randomness is retrieved through the finality provider daemon's connection
   with the [EOTS daemon](eots.md).
3. **Finality Votes Submission**: The daemon monitors the Babylon chain and produces
   finality votes for each block each maintained finality provider has committed to
   vote for.

The daemon is controlled by the `fpd` tool, which has overall commands for
interacting with the running daemon.

## 2. Configuration

The `fpd init` command initializes a home directory for the finality provider daemon.
This directory is created in the default home location or in a location specified by
the `--home` flag.

```bash
fpd init --home /path/to/fpd/home/
```

After initialization, the home directory will have the following structure

```bash
ls /path/to/fpd/home/
  ├── fpd.conf # Fpd-specific configuration file.
  ├── logs     # Fpd logs
```

If the `--home` flag is not specified, then the default home directory will be used.
For different operating systems, those are:

- **MacOS** `~/Users/<username>/Library/Application Support/Fpd`
- **Linux** `~/.Fpd`
- **Windows** `C:\Users\<username>\AppData\Local\Fpd`

Below are some important parameters of the `fpd.conf` file.

**Note**:
The configuration below requires pointing to the path where this keyring is
stored `KeyDirectory`. This `Key` field stores the key name of the finality
provider wallet that is going to be used for interacting with the consumer
chain and to sign the messages of your finality provider. It will be specified
along with the `KeyringBackend` field in the next
[step](#3-add-key-for-the-consumer-chain). So we can ignore the setting of the
two fields in this step.

```bash
[Application Options]
# RPC Address of the EOTS Daemon
EOTSManagerAddress = 127.0.0.1:12582

# RPC Address of the Finality Provider Daemon
RpcListener = 127.0.0.1:12581

[babylon]
# Name of the key of the finality provider to sign transactions with
Key = <finality-provider-key-name-signer>

# Chain id of the chain to connect to
# Please verify the `ChainID` from the Babylon RPC node https://rpc.testnet3.babylonchain.io/status
ChainID = bbn-test-3

# RPC Address of Babylon node
RPCAddr = http://127.0.0.1:26657

# GRPC Address of Babylon node
GRPCAddr = https://127.0.0.1:9090

# Directory to store keys in
KeyDirectory = /path/to/fpd/home
```

To see the complete list of configuration options, check the `fpd.conf` file.

**Additional Notes:**

If you encounter any gas-related errors while performing staking operations, consider
adjusting the `GasAdjustment` and `GasPrices` parameters. For example, you can set:

```bash
GasAdjustment = 1.5
GasPrices = 0.002ubbn
```

## 3. Add key for the consumer chain

The finality provider daemon requires the existence of a keyring that contains an
account for the finality provider with Babylon token funds to pay and sign
transactions. This key identifies the finality provider and will be also used
to pay for fees of transactions to the consumer chain.

Use the following command to add the key:

```bash
fpd keys add my-finality-provider
```

After executing the above command, the key name will be saved in the config file
created in [step](#2-configuration).

## 4. Starting the Finality Provider Daemon

You can start the finality provider daemon using the following command:

```bash
fpd start --home /path/to/fpd/home
```

If the `--home` flag is not specified, then the default home location will be used.

This will start the Finality provider RPC server at the address specified
in `fpd.conf` under the `RpcListener` field, which has a default value
of `127.0.0.1:12581`. You can change this value in the configuration file or override
this value and specify a custom address using the `--rpc-listener` flag.

This will also start all the registered finality provider instances except for
slashed ones added in [step](#5-create-and-register-a-finality-provider). To start
the daemon with a specific finality provider instance, use the
`--eots-pk` flag followed by the hex string of the BTC public key of the finality
provider (`btc_pk_hex`) obtained
in [step](#5-create-and-register-a-finality-provider).

```bash
fpd start

2024-02-08T18:43:00.705008Z info successfully connected to a remote EOTS manager {"address": "127.0.0.1:12582"}
2024-02-08T18:43:00.712995Z info Starting FinalityProviderApp
2024-02-08T18:43:00.716682Z info RPC server listening {"address": "127.0.0.1:12581"}
2024-02-08T18:43:00.716979Z info Finality Provider Daemon is fully active!
```

All the available CLI options can be viewed using the `--help` flag. These options
can also be set in the configuration file.

## 5. Create and Register a Finality Provider

We create a finality provider instance through the
`fpd create-finality-provider` or `fpd cfp` command. The created instance is
associated with a BTC public key which serves as its unique identifier and a Babylon
account to which staking rewards will be directed. Note that if the `--key-name` flag
is not specified, the `Key` field of config specified
in [step](#3-add-key-for-the-consumer-chain) will be used.

```bash
fpd create-finality-provider --key-name my-finality-provider \
                --chain-id bbn-test-3 --moniker my-name
{
  "fp_addr": "bbn19khdh5vf8zv9x49f84cfuxx5t45m7klwq827mp",
  "btc_pk_hex": "d0fc4db48643fbb4339dc4bbf15f272411716b0d60f18bdfeb3861544bf5ef63",
  "description": {
    "moniker": "my-name"
  },
  "status": "CREATED"
}
```

We register a created finality provider in Babylon through
the `fpd register-finality-provider` or `fpd rfp` command. The output contains
the hash of the Babylon finality provider registration transaction.

```bash
fpd register-finality-provider \
  --eots-pk d0fc4db48643fbb4339dc4bbf15f272411716b0d60f18bdfeb3861544bf5ef63
{
  "tx_hash": "800AE5BBDADE974C5FA5BD44336C7F1A952FAB9F5F9B43F7D4850BA449319BAA"
}

```

A finality provider instance will be initiated and start running right after the
finality provider is successfully registered in Babylon.

We can view the status of all the running finality providers through
the `fpd list-finality-providers` or `fpd ls` command. The `status` field can
receive the following values:

- `CREATED`: The finality provider is created but not registered yet
- `REGISTERED`: The finality provider is registered but has not received any active
  delegations yet
- `ACTIVE`: The finality provider has active delegations and is empowered to send
  finality signatures
- `INACTIVE`: The finality provider used to be ACTIVE but the voting power is reduced
  to zero
- `SLASHED`: The finality provider is slashed due to malicious behavior

```bash
fpd list-finality-providers
{
    "finality-providers": [
        ...
        {
            "fp_addr": "bbn19khdh5vf8zv9x49f84cfuxx5t45m7klwq827mp",
            "btc_pk_hex": "d0fc4db48643fbb4339dc4bbf15f272411716b0d60f18bdfeb3861544bf5ef63",
            "description": {
                "moniker": "my-name"
            },
            "last_vote_height": 1
            "status": "REGISTERED"
        }
    ]
}
```

After the creation of the finality provider in the local db, it is possible
to export the finality provider information through the `fpd export-finality-provider` command.
This command connects with the `fpd` daemon to retrieve the finality
provider previously created using the flag `--eots-pk` as key.

This command also has several flag options:

- `--eots-pk` the hex string of the BTC public key.
- `--daemon-address` the RPC server address of `fpd` daemon.
- `--signed` signs the finality provider with the chain key of the PoS
chain secured as a proof of untempered exported data.
- `--key-name` identifies the name of the key to use to sign the finality provider.
- `--home` specifies the home directory of the finality provider daemon in which
the finality provider db is stored.
- `--passphrase` specifies the password used to encrypt the key, if such a
passphrase is required.
- `--hd-path` the hd derivation path of the private key.

```shell
$ fpd export-finality-provider --eots-pk 02face5996b2792114677604ec9dfad4fe66eeace3df92dab834754add5bdd7077 \
--home ./export-fp/fpd --key-name finality-provider --signed
```

The expected result is a JSON object corresponding to the finality provider information.

```json
{
  "description": {
    "moniker": "my-fp-nickname",
    "identity": "anyIdentity",
    "website": "www.my-public-available-website.com",
    "security_contact": "your.email@gmail.com",
    "details": "other overall info"
  },
  "commission": "0.050000000000000000",
  "fp_addr": "bbn19khdh5vf8zv9x49f84cfuxx5t45m7klwq827mp",
  "btc_pk": "02face5996b2792114677604ec9dfad4fe66eeace3df92dab834754add5bdd7077",
  "pop": {
    "btc_sig": "sHLpEHVTyTp9K55oeHxnPlkV4unc/r1obqzKn5S1gq95oXA3AgL1jyCzd/mGb23RfKbEyABjYUdcIBtZ02l5jg=="
  },
  "master_pub_rand": "xpub661MyMwAqRbcFLhUq9uPM7GncSytVZvoNg4w7LLx1Y74GeeAZerkpV1amvGBTcw4ECmrwFsTNMNf1LFBKkA2pmd8aJ5Jmp8uKD5xgVSezBq",
  "fp_sig_hex": "8ded8158bf65d492c5c6d1ff61c04a2176da9c55ea92dcce5638d11a177b999732a094db186964ab1b73c6a69aaa664672a36620dedb9da41c05e88ad981edda"
}
```
