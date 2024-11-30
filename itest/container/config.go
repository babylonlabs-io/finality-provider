package container

import (
	"testing"
)

// ImageConfig contains all images and their respective tags
// needed for running e2e tests.
type ImageConfig struct {
	BabylonRepository string
	BabylonVersion    string
}

//nolint:deadcode
const (
	dockerBabylondRepository = "babylonlabs/babylond"
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig(t *testing.T) ImageConfig {
	// TODO: currently use specific commit, should uncomment after having a new release
	// babylondVersion, err := testutil.GetBabylonVersion()
	// require.NoError(t, err)
	return ImageConfig{
		BabylonRepository: dockerBabylondRepository,
		BabylonVersion:    "457909d8c43c8483655c2d3a3a01cd2190344fd4",
	}
}
