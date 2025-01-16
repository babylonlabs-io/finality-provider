// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Heavily inspired by https://github.com/btcsuite/btcd/blob/master/version.go
// Copyright (C) 2015-2022 The Lightning Network Developers
package version

import (
	"fmt"
	"runtime/debug"
)

// version set at build-time
var version = "main"

func CommitInfo() (string, string) {
	hash, timestamp := "unknown", "unknown"
	hashLen := 7

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return hash, timestamp
	}

	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) < hashLen {
				hashLen = len(s.Value)
			}
			hash = s.Value[:hashLen]
		} else if s.Key == "vcs.time" {
			timestamp = s.Value
		}
	}

	return hash, timestamp
}

// Version returns the version
func Version() string {
	return version
}

func RPC() string {
	commit, ts := CommitInfo()

	return fmt.Sprintf("version: %s, commit: %s, timestamp: %s ", version, commit, ts)
}
