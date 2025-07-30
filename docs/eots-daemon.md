# EOTS Daemon Setup

This guide covers the installation and setup of the EOTS (Extractable One-Time
Signature) daemon, which is a core component of the finality provider stack
responsible for managing your EOTS keys and producing EOTS signatures.

## Table of Contents

1. [Install Finality Provider Toolset](#1-install-finality-provider-toolset)
2. [Setting up the EOTS Daemon](#2-setting-up-the-eots-daemon)
   1. [Initialize the EOTS Daemon](#21-initialize-the-eots-daemon)
   2. [Add an EOTS Key](#22-add-an-eots-key)
      1. [Create an EOTS key](#221-create-an-eots-key)
      2. [Import an existing EOTS key](#222-import-an-existing-eots-key)
   3. [Starting the EOTS Daemon](#23-starting-the-eots-daemon)
       1. [Migration guide test to file keyring backend](#231-migration-guide-test-to-file-keyring-backend)
       2. [Unlock file-based keyring](#232-unlock-file-based-keyring)

## 1. Install Finality Provider Toolset

The finality provider toolset requires [Golang 1.23](https://go.dev/dl) to be
installed. Please follow the installation instructions
[here](https://go.dev/dl). You can verify the installation with the following
command:

```shell
go version
```

### 1.1. Clone the Finality Provider Repository

Subsequently, clone the finality provider
[repository](https://github.com/babylonlabs-io/finality-provider) and checkout
to the version you want to install.

```shell
git clone https://github.com/babylonlabs-io/finality-provider.git
cd finality-provider
git checkout <version>
```

### 1.2. Install Finality Provider Toolset Binaries

Run the following command to build the binaries and install them to your
`$GOPATH/bin` directory:

```shell
make install
```

This command will:

* Build and compile all Go packages.
* Install binaries to `$GOPATH/bin`:
  * `eotsd`: EOTS manager daemon
  * `fpd`: Finality provider daemon.
* Make commands globally accessible from your terminal.

### 1.3. Verify Installation

Run the following command to verify the installation:

```shell
fpd version
Version:       <version>
Git Commit:    <commit>
Git Timestamp: <timestamp>
```

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile
depending on your shell.

```shell
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
source ~/.profile
```

## 2. Setting up the EOTS Daemon

The EOTS manager daemon is a core component of the finality provider stack
responsible for managing your EOTS keys and producing EOTS signatures to be used
for votes. In this section, we are going to go through its setup and key
generation process.

### 2.1. Initialize the EOTS Daemon

If you haven't already, initialize a home directory for the EOTS Manager with
the following command:

```shell
eotsd init --home <path>
```

If the home directory already exists, `init` will not succeed.
> ⚡ Specifying `--force` to `init` will overwrite `eotsd.conf` with default
> config values if the home directory already exists. Please backup `eotsd.conf`
> before you run `init` with `--force`.

Parameters:

* `--home`: Directory for EOTS Manager configuration and data
  * Default: `DefaultEOTSDir` the default EOTS home directory:
    * `C:\Users\<username>\AppData\Local\ on Windows`
    * `~/.eotsd` on Linux
    * `~/Library/Application Support/Eotsd` on MacOS
  * Example: `--home ./eotsHome`

**Home directory structure:**

```shell
~/.eotsd/
├── config/
│   └── eotsd.conf      # Configuration file for the EOTS manager
├── data/
│   └── eotsd.db        # Database containing EOTS keys and mappings
├── keyring-*/          # Directory containing EOTS keyring data
└── logs/
    └── eotsd.log       # Log file for the EOTS manager daemon
```

* **eotsd.conf** This configuration file controls the core settings of the EOTS
  daemon. It uses the Cosmos SDK keyring system to securely store and manage
  EOTS keys. The file configures how the daemon interacts with its database,
  manages keys through the keyring backend, and exposes its RPC interface.
  Essential settings include database paths, keyring backend selection
  (test/file/os), RPC listener address, logging levels, and metrics
  configuration.

* **eotsd.db**:
  * EOTS key to key name mappings
  * EOTS signing history for [Slashing Protection](./slashing-protection.md).

* **keyring-directory***:
  * EOTS private keys are securely stored using Cosmos SDK's keyring system
  * `test` keyring-backend should only be used for test environments.
  * `file` keyring-backend is used for production environments but requires call
  to `Unlock` command. See the section [Unlock file-based keyring](#232-unlock-file-based-keyring)
  * Keys are used for EOTS signatures

* **eotsd.log**:
  * Key creation and import events
  * Signature generation requests
  * Error messages and debugging information
  * Service status updates

### 2.2. Add an EOTS Key

This section explains the process of creating EOTS keys using the EOTS manager.

The EOTS manager uses [Cosmos
SDK](https://docs.cosmos.network/v0.50/user/run-node/keyring) backends for key
storage. Since this key is accessed by an automated daemon process, it must be
stored unencrypted on disk and associated with the `test` keyring backend. This
ensures that we can access the eots keys when requested to promptly submit
transactions, such as block votes and public randomness submissions that are
essential for its liveness and earning of rewards.

#### 2.2.1. Create an EOTS key

If you have not created an EOTS key, use the following command to create a new
one:

``` shell
eotsd keys add <key-name> --home <path> --keyring-backend test
```

Parameters:

* `<key-name>`: Name for your EOTS key (e.g., "eots-key-1"). We do not allow the
same `keyname` for an existing keyname.
* `--home`: Path to your EOTS daemon home directory (e.g., "~/.eotsHome")
* `--keyring-backend`: Type of keyring storage (`test`)

The command will return a JSON response containing your EOTS key details:

```shell
{
    "name": "eots",
    "pub_key_hex":
    "e1e72d270b90b24f395e76b417218430a75683bd07cf98b91cf9219d1c777c19",
    "mnemonic": "parade hybrid century project toss gun undo ocean exercise
      figure decorate basket peace raw spot gap dose daring patch ski purchase
      prefer can pair"
}
```

> **🔒 Security Tip**: The mnemonic phrase must be stored securely and kept
> private. It is the only way to recover your EOTS key if you lose access to it
> and if lost it can be used by third parties to get access to your key.

#### 2.2.2. Import an existing EOTS key

> ⚡ This section is for Finality Providers who already possess an EOTS key. If
> you don't have keys or want to create new ones, you can skip this section.

There are 2 supported methods of loading your existing EOTS keys:

1. using a mnemonic phrase
2. importing the `.asc` file

We have outlined each of these two paths for you below.

#### Option 1: Using your mnemonic phrase

If you are using your mnemonic seed phrase, use the following command to import
your key:

```shell
eotsd keys add <key-name> --home <path> --recover --keyring-backend test
```

You'll be prompted to enter:

1. Your BIP39 mnemonic phrase (24 words)
2. HD path (optional - press Enter to use the default)

> ⚡ The HD path is optional. If you used the default path when creating your
key, you can skip this by pressing `Enter` , which by default uses your original
private key.

#### Option 2: Using your `.asc` file

If you exported your key to a `.asc` file. The `.asc` file should be in the
following format:

```shell
-----BEGIN TENDERMINT PRIVATE KEY-----
salt: 35ED0BBC00376EC7FC696838F34A7C36
type: secp256k1
kdf: argon2

8VOGhpuaZhTPZrKHysx24OhaxuBhVnKqb3WcTwJY+jvfNv/EJRoqmrHZfCnNgd13
VP88GFE=
=D87O
-----END TENDERMINT PRIVATE KEY-----
```

To load the key, use the following command:

```shell
eotsd keys import <name> <path-to-key> --home <path> --keyring-backend test
```

#### Verify the Key Import

After importing, you can verify that your EOTS key was successfully loaded:

```shell
eotsd keys list --home <path> --keyring-backend test
```

Parameters:

* `--keyring-backend`: Type of keyring backend to use (`test`)
* `--home`: Directory containing EOTS Manager configuration and data

You should see your EOTS key listed with the correct details if the import is
successful.

### 2.3. Starting the EOTS Daemon

To start the EOTS daemon, use the following command:

```shell
eotsd start --home <path>
```

This command starts the EOTS RPC server at the address specified in `eotsd.conf`
under the `RPCListener` field (default: `127.0.0.1:12582`). You can override
this value by specifying a custom address with the `--rpc-listener` flag.

```shell
2024-10-30T12:42:29.393259Z     info  Metrics server is starting
{"addr": "127.0.0.1:2113"}
2024-10-30T12:42:29.393278Z     info  RPC server listening{"address": "127.0.0.1:12582"}
2024-10-30T12:42:29.393363Z     info  EOTS Manager Daemon is fully active!
EOTS Manager Daemon is fully active!
```

#### 2.3.1. Migration guide test to file keyring backend

You will first need to stop eotsd and export your private EOTS key in hex format
by executing the following command:

```shell
eotsd keys export <name> --keyring-backend=test --unsafe --unarmored-hex --home <path>
```

As a result of the `export` command, you will receive the EOTS key as a hex
string, which you can use to import the key into the `file` keyring backend.

Keyring backed `file` requires a setting up a password, this password will be
used in the `unlock` command. You can then import the key into the `file`
keyring backend using the following command:

```shell
eotsd keys import-hex <name> <hexstring> --keyring-backend=file --home <path>
```

To confirm that you have successfully imported your EOTS key to the `file`
keyring backend, you can run the following command:

```shell
eotsd keys show <name> --home <path> --keyring-backend file
```

In the `eots.conf` change the `keyring-backend` to `file`:

```diff
+ KeyringBackend = file
- KeyringBackend = test
```
After you have changed the keyring backend to `file`, you can start the eotsd
and run the unlock command.

In order to run eotsd with the `file` keyring, please read the [Unlock
file-based keyring](#232-unlock-file-based-keyring) section below.

#### 2.3.2. Unlock file-based keyring

⚠️⚠️⚠️ Mandatory step for file-based keyring backend ⚠️⚠️⚠️

If you are using a `file` based keyring-backend, you need to unlock the keyring
by executing the `unlock` command. Unlock is required after the eotsd daemon is
started, otherwise if the unlock command is not run the daemon will error and
not be able to sign. This only applies to the `file` keyring backend, if you are
using the `test` keyring backend, you can skip this step.

```shell
eotsd unlock --eots-pk <eots-pk> --rpc-client <eotsd-address>
```

You will be prompted to enter the password for the keyring. After which the
signing operations will be available and eotsd can run uninterrupted.

An alternative to providing the password as an input is specifying the password
with the environment variable `EOTS_KEYRING_PASSWORD`:

```shell
export EOTSD_KEYRING_PASSWORD=<your-password>
```

If you have HMAC security enabled, you can also specify the HMAC key either
with:
* `HMAC_KEY` environment variable, or
* providing the `--home` path to the eotsd home directory which contains the
  config file with hmac key set up.

---
>**🔒 Security Tip**:
>
> * `eotsd` holds your private keys which are used for signing
> * operate the daemon in a separate machine or network segment with enhanced
>   security
> * only allow access to the RPC server specified by the `RPCListener` port to
>   trusted sources. You can edit the `EOTSManagerAddress` in the configuration
>   file of the finality provider to reference the address of the machine where
>   `eotsd` is running
> * setup HMAC to secure the communication between `eotsd` and `fpd`. 
>   See [HMAC Security](./hmac-security.md). 