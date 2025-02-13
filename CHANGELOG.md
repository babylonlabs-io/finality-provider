<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning.

Usage:

Change log entries are to be added to the Unreleased section under the
appropriate stanza (see below). Each entry should have following format:

* [#PullRequestNumber](PullRequestLink) message

Types of changes (Stanzas):

"Features" for new features.
"Improvements" for changes in existing functionality.
"Deprecated" for soon-to-be removed features.
"Bug Fixes" for any bug fixes.
"Client Breaking" for breaking CLI commands and REST routes used by end-users.
"API Breaking" for breaking exported APIs used by developers building on SDK.
"State Machine Breaking" for any changes that result in a different AppState
given same genesisState and txList.
Ref: https://keepachangelog.com/en/1.0.0/
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)

## Unreleased

### Improvements

* [#335](https://github.com/babylonlabs-io/finality-provider/pull/335) chore: fix CosmWasm controller
* [#331](https://github.com/babylonlabs-io/finality-provider/pull/331) Bump Cosmos integration dependencies

### Bug Fixes

* [#333](https://github.com/babylonlabs-io/finality-provider/pull/333) poller: skip if no new block is polled
* [#328](https://github.com/babylonlabs-io/finality-provider/pull/328) Fix small bias in EOTS private key generation

## v1.0.0-rc.1

## v0.15.1

### Improvements

* [#62](https://github.com/babylonlabs-io/finality-provider/pull/62) **Consumer chain support.**
This PR contains a series of PRs on BTC staking integration, with support for OP stack chains and
Cosmos chains.
* [#314](https://github.com/babylonlabs-io/finality-provider/pull/314) nit: Dockerfile AS casing

### Bug Fixes

* [#316](https://github.com/babylonlabs-io/finality-provider/pull/316) fix: typo in config validation

## v0.15.0

### Bug Fixes

* [#296](https://github.com/babylonlabs-io/finality-provider/pull/296) fix: edit finality provider commission-rate
* [#307](https://github.com/babylonlabs-io/finality-provider/pull/307) fix: increment fp_total_failed_votes

### Improvements

* [#251](https://github.com/babylonlabs-io/finality-provider/pull/251) Add nlreturn lint
* [#252](https://github.com/babylonlabs-io/finality-provider/pull/252) Remove interceptors and use context
* [#266](https://github.com/babylonlabs-io/finality-provider/pull/266) Change default config
* [#262](https://github.com/babylonlabs-io/finality-provider/pull/262) Add new command to export pop
* [#284](https://github.com/babylonlabs-io/finality-provider/pull/284) Add new command to delete pop
* [#277](https://github.com/babylonlabs-io/finality-provider/pull/277) Poll many blocks in poller
* [#291](https://github.com/babylonlabs-io/finality-provider/pull/291) chore: remove skip height
* [#294](https://github.com/babylonlabs-io/finality-provider/pull/294) chore: Improve fpd start
* [#297](https://github.com/babylonlabs-io/finality-provider/pull/297) Add new command to validate pop
* [#302](https://github.com/babylonlabs-io/finality-provider/pull/302) Update pop commands to write to a file
* [#301](https://github.com/babylonlabs-io/finality-provider/pull/301) chore: check tx index enabled
* [#308](https://github.com/babylonlabs-io/finality-provider/issues/308) chore: bump babylon to v1.0.0-rc.4

## v0.14.3

### Improvements

* [#253](https://github.com/babylonlabs-io/finality-provider/issues/253) Refactor to start from the last finalized height
* [#260](https://github.com/babylonlabs-io/finality-provider/pull/260) Allow running of jailed fp

## v0.14.2

### Bug Fixes

* [#244](https://github.com/babylonlabs-io/finality-provider/pull/244) fix: save key name mapping
verifies if there is a eots client running
* [#246](https://github.com/babylonlabs-io/finality-provider/pull/246) fix: start fp after register

## v0.14.1

### Bug Fixes

* [#240](https://github.com/babylonlabs-io/finality-provider/pull/240) fix removed printf in cmd command

## v0.14.0

### Improvements

* [#207](https://github.com/babylonlabs-io/finality-provider/pull/207) create finality provider from JSON file
* [#208](https://github.com/babylonlabs-io/finality-provider/pull/208) Remove sync fp status loop
* [#211](https://github.com/babylonlabs-io/finality-provider/pull/211) Clean up unused cmd
* [#214](https://github.com/babylonlabs-io/finality-provider/pull/214) Gradual benchmark
* [#216](https://github.com/babylonlabs-io/finality-provider/pull/216) Add multiple fpd connecting to one eotsd in e2e tests
* [#218](https://github.com/babylonlabs-io/finality-provider/pull/218) Prune used merkle proof
* [#221](https://github.com/babylonlabs-io/finality-provider/pull/221) Cleanup TODOs
* [#228](https://github.com/babylonlabs-io/finality-provider/pull/228) Save key name mapping in eotsd import commands
* [#227](https://github.com/babylonlabs-io/finality-provider/pull/227) Fix FP submission loop
* [#226](https://github.com/babylonlabs-io/finality-provider/pull/226) Update local fp before register
* [#233](https://github.com/babylonlabs-io/finality-provider/pull/233) Refactor CommitPubRand
* [#234](https://github.com/babylonlabs-io/finality-provider/pull/234) eotsd ls command
* [#238](https://github.com/babylonlabs-io/finality-provider/pull/238) bump babylon v1.0.0-rc.1

## v0.13.1

### Bug Fixes

* [#199](https://github.com/babylonlabs-io/finality-provider/pull/199) EOTS signing for multiple finality providers
* [#203](https://github.com/babylonlabs-io/finality-provider/pull/203) fpd cli: Withdraw rewards and set withdraw addr

## v0.13.0

### Improvements

* [#175](https://github.com/babylonlabs-io/finality-provider/pull/175) adds: `eotsd version` command
* [#179](https://github.com/babylonlabs-io/finality-provider/pull/179) Change `btc_pk` text to `eots_pk` in CLI
* [#182](https://github.com/babylonlabs-io/finality-provider/pull/182) Remove fp manager
* [#184](https://github.com/babylonlabs-io/finality-provider/pull/184) eots manager sign record store
* [#189](https://github.com/babylonlabs-io/finality-provider/pull/189) Remove `fpd register-finality-provider` cmd
* [#190](https://github.com/babylonlabs-io/finality-provider/pull/190) Benchmark pub rand
* [#193](https://github.com/babylonlabs-io/finality-provider/pull/193) adds unsafeSignEOTS for e2e tests
* [#195](https://github.com/babylonlabs-io/finality-provider/pull/195) Not block unjailing
* [#197](https://github.com/babylonlabs-io/finality-provider/pull/197) Bump Babylon to v0.18.0

### Bug Fixes

* [#166](https://github.com/babylonlabs-io/finality-provider/pull/166) fix: `eotsd keys add` `--output` flag

### Improvements

* [#149](https://github.com/babylonlabs-io/finality-provider/pull/149) Remove update of config after `fpd keys add`
* [#148](https://github.com/babylonlabs-io/finality-provider/pull/148) Allow command `eotsd keys add` to use
empty HD path to derive new key and use master private key.
* [#153](https://github.com/babylonlabs-io/finality-provider/pull/153) Add `unsafe-commit-pubrand` command
* [#154](https://github.com/babylonlabs-io/finality-provider/pull/154) Use sign schnorr instead of getting private key from EOTS manager
* [#167](https://github.com/babylonlabs-io/finality-provider/pull/167) Remove last processed height
* [#168](https://github.com/babylonlabs-io/finality-provider/pull/168) Remove key creation in `create-finality-provider`
* [#176](https://github.com/babylonlabs-io/finality-provider/pull/176) Refactor
determining start height based on [ADR-35](https://github.com/babylonlabs-io/pm/blob/main/adr/adr-035-slashing-protection.md)

### v0.12.1

### Bug Fixes

* [#158](https://github.com/babylonlabs-io/finality-provider/pull/158) Remove start height validation

## v0.12.0

### Bug Fixes

* [#139](https://github.com/babylonlabs-io/finality-provider/pull/139) Ignore voting power not updated error

### Improvements

* [#127](https://github.com/babylonlabs-io/finality-provider/pull/127) Bump docker workflow version and fix some dockerfile issue
* [#132](https://github.com/babylonlabs-io/finality-provider/pull/132) Replace fast sync with batch processing
* [#146](https://github.com/babylonlabs-io/finality-provider/pull/146) Upgrade Babylon to v0.17.1

### Documentation

[#120](https://github.com/babylonlabs-io/finality-provider/pull/120) Spec of
finality vote submission

## v0.11.0

### Improvements

* [#126](https://github.com/babylonlabs-io/finality-provider/pull/126) Adds linting config
* [#128](https://github.com/babylonlabs-io/finality-provider/pull/128) Upgrade Babylon to v0.16.0

### Documentation

* [#117](https://github.com/babylonlabs-io/finality-provider/pull/117) Spec of
commit public randomness
* [#130](https://github.com/babylonlabs-io/finality-provider/pull/130) Finality
Provider operation documentation

### Bug Fixes

* [#124](https://github.com/babylonlabs-io/finality-provider/pull/124) Ignore
duplicated finality vote error

## v0.10.0

### Improvements

* [#114](https://github.com/babylonlabs-io/finality-provider/pull/114) Bump Babylon version to v0.15.0
* [#102](https://github.com/babylonlabs-io/finality-provider/pull/102) Improve `eotsd keys add` command
* [#104](https://github.com/babylonlabs-io/finality-provider/pull/104) Print fpd binary version
* [#87](https://github.com/babylonlabs-io/finality-provider/pull/87) Rename ChainName to ChainType

## v0.9.1

### Bug Fixes

* [#107](https://github.com/babylonlabs-io/finality-provider/pull/107) Fix commit
start height when the finality activation height is higher than the current
block tip

## v0.9.0

### Improvements

* [#101](https://github.com/babylonlabs-io/finality-provider/pull/101) Add finality activation
height check in finality voting and commit pub rand start height and bump Babylon version to
v0.14.0

## v0.8.0

### Improvements

* [#97](https://github.com/babylonlabs-io/finality-provider/pull/97) Bump Babylon version to v0.13.0
* [#90](https://github.com/babylonlabs-io/finality-provider/pull/90) CLI edit finality provider
* [#91](https://github.com/babylonlabs-io/finality-provider/pull/91) Go releaser setup
  and move changelog reminder out
* [#86](https://github.com/babylonlabs-io/finality-provider/pull/86) Remove running multiple fp instances support
