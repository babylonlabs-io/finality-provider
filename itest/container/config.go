package container

// ImageConfig contains all images and their respective tags
// needed for running e2e tests.
type ImageConfig struct {
	BabylonRepository string
	BabylonVersion    string
}

//nolint:deadcode
const (
	dockerBabylondRepository = "babylonlabs/babylond"
	dockerBabylondVersionTag = "v0.10.0"
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig() ImageConfig {
	return ImageConfig{
		BabylonRepository: dockerBabylondRepository,
		BabylonVersion:    dockerBabylondVersionTag,
	}
}
