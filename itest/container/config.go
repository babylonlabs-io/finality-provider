package container

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/testutil"
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
	// NOTE: currently there's no tag for the latest API breaking changes
	// on babylon node. Because of this, we're using the commit hash instead of
	// the version tag. There's a docker image pushed to the registry with every PR
	// merged to main.
	// TODO on creation of the v1rc7 tag (or other useful tag for these tests), we should use the GetBabylonVersion() back again
	// babylondVersion, err := testutil.GetBabylonVersion()
	babylondVersion, err := testutil.GetBabylonCommitHash()
	require.NoError(t, err)
	return ImageConfig{
		BabylonRepository: dockerBabylondRepository,
		BabylonVersion:    babylondVersion,
	}
}
