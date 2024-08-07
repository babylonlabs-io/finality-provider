module github.com/babylonlabs-io/finality-provider/tools

go 1.21

toolchain go1.21.4

// Downgraded to stable version see: https://github.com/cosmos/cosmos-sdk/pull/14952
replace (
	// use cosmos fork of keyring
	github.com/99designs/keyring => github.com/cosmos/keyring v1.2.0
	github.com/babylonlabs-io/babylon => github.com/babylonlabs-io/babylon-private v0.9.0-rc.3.0.20240801001431-74a24c962ce2
	github.com/syndtr/goleveldb => github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
)
