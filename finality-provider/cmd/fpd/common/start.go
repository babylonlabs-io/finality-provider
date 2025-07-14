package common

import (
	"fmt"

	"github.com/babylonlabs-io/babylon/v3/types"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
)

// StartApp starts the app and the handle of finality providers if needed based on flags.
func StartApp(
	fpApp *service.FinalityProviderApp,
	fpPkStr string,
) error {
	// only start the app without starting any finality provider instance
	// this is needed for new finality provider registration or unjailing
	// finality providers
	if err := fpApp.Start(); err != nil {
		return fmt.Errorf("failed to start the finality provider app: %w", err)
	}

	// fp instance will be started if public key is specified
	if fpPkStr != "" {
		// start the finality-provider instance with the given public key
		fpPk, err := types.NewBIP340PubKeyFromHex(fpPkStr)
		if err != nil {
			return fmt.Errorf("invalid finality provider public key %s: %w", fpPkStr, err)
		}

		return fpApp.StartFinalityProvider(fpPk)
	}

	storedFps, err := fpApp.GetFinalityProviderStore().GetAllStoredFinalityProviders()
	if err != nil {
		return err
	}

	if len(storedFps) == 1 {
		return fpApp.StartFinalityProvider(types.NewBIP340PubKeyFromBTCPK(storedFps[0].BtcPk))
	}

	if len(storedFps) > 1 {
		return fmt.Errorf("%d finality providers found in DB. Please specify the EOTS public key", len(storedFps))
	}

	fpApp.Logger().Info("No finality providers found in DB. Waiting for registration.")

	return nil
}
