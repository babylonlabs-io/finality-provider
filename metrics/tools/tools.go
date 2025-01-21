//go:build tools
// +build tools

package finalityprovider

import (
	_ "github.com/CosmWasm/wasmd/cmd/wasmd"
	_ "github.com/babylonlabs-io/babylon-sdk/demo/cmd/bcd"
	_ "github.com/babylonlabs-io/babylon/cmd/babylond"
)
