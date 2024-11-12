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

* [#126](https://github.com/babylonlabs-io/finality-provider/pull/126) Adds linting config

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
