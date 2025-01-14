package service

import (
	"errors"
	"fmt"
	"time"

	sdkmath "cosmossdk.io/math"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
)

type CreateFinalityProviderRequest struct {
	fpAddr          sdk.AccAddress
	btcPubKey       *bbntypes.BIP340PubKey
	pop             *btcstakingtypes.ProofOfPossessionBTC
	description     *stakingtypes.Description
	commission      *sdkmath.LegacyDec
	errResponse     chan error
	successResponse chan *RegisterFinalityProviderResponse
}

type RegisterFinalityProviderResponse struct {
	txHash string
}

type CreateFinalityProviderResult struct {
	FpInfo *proto.FinalityProviderInfo
	TxHash string
}

type UnjailFinalityProviderRequest struct {
	btcPubKey       *bbntypes.BIP340PubKey
	errResponse     chan error
	successResponse chan *UnjailFinalityProviderResponse
}

type UnjailFinalityProviderResponse struct {
	TxHash string
}

// event loop for critical errors
func (app *FinalityProviderApp) monitorCriticalErr() {
	defer app.wg.Done()

	var criticalErr *CriticalError

	for {
		select {
		case criticalErr = <-app.criticalErrChan:
			fpi, err := app.GetFinalityProviderInstance()
			if err != nil {
				app.logger.Debug("the finality-provider instance is already shutdown",
					zap.String("pk", criticalErr.fpBtcPk.MarshalHex()))

				continue
			}
			if errors.Is(criticalErr.err, ErrFinalityProviderSlashed) {
				app.setFinalityProviderSlashed(fpi)
				app.logger.Debug("the finality-provider has been slashed",
					zap.String("pk", criticalErr.fpBtcPk.MarshalHex()))

				continue
			}
			app.logger.Fatal(instanceTerminatingMsg,
				zap.String("pk", criticalErr.fpBtcPk.MarshalHex()), zap.Error(criticalErr.err))
		case <-app.quit:
			app.logger.Info("exiting monitor critical error loop")

			return
		}
	}
}

// event loop for handling fp registration
func (app *FinalityProviderApp) registrationLoop() {
	defer app.wg.Done()
	for {
		select {
		case req := <-app.createFinalityProviderRequestChan:
			// we won't do any retries here to not block the loop for more important messages.
			// Most probably it fails due so some user error so we just return the error to the user.
			// TODO: need to start passing context here to be able to cancel the request in case of app quiting
			popBytes, err := req.pop.Marshal()
			if err != nil {
				req.errResponse <- err

				continue
			}

			desBytes, err := req.description.Marshal()
			if err != nil {
				req.errResponse <- err

				continue
			}
			res, err := app.cc.RegisterFinalityProvider(
				req.btcPubKey.MustToBTCPK(),
				popBytes,
				req.commission,
				desBytes,
			)

			if err != nil {
				app.logger.Error(
					"failed to register finality-provider",
					zap.String("pk", req.btcPubKey.MarshalHex()),
					zap.Error(err),
				)
				req.errResponse <- err

				continue
			}

			app.logger.Info(
				"successfully registered finality-provider on babylon",
				zap.String("btc_pk", req.btcPubKey.MarshalHex()),
				zap.String("fp_addr", req.fpAddr.String()),
				zap.String("txHash", res.TxHash),
			)

			app.metrics.RecordFpStatus(req.btcPubKey.MarshalHex(), proto.FinalityProviderStatus_REGISTERED)

			req.successResponse <- &RegisterFinalityProviderResponse{
				txHash: res.TxHash,
			}
		case <-app.quit:
			app.logger.Info("exiting registration loop")

			return
		}
	}
}

// event loop for unjailing fp
func (app *FinalityProviderApp) unjailFpLoop() {
	defer app.wg.Done()
	for {
		select {
		case req := <-app.unjailFinalityProviderRequestChan:
			pkHex := req.btcPubKey.MarshalHex()
			isSlashed, isJailed, err := app.cc.QueryFinalityProviderSlashedOrJailed(req.btcPubKey.MustToBTCPK())
			if err != nil {
				req.errResponse <- fmt.Errorf("failed to query jailing status of the finality provider %s: %w", pkHex, err)

				continue
			}
			if isSlashed {
				req.errResponse <- fmt.Errorf("the finality provider %s is already slashed", pkHex)

				continue
			}
			if !isJailed {
				req.errResponse <- fmt.Errorf("the finality provider %s is not jailed", pkHex)

				continue
			}

			res, err := app.cc.UnjailFinalityProvider(
				req.btcPubKey.MustToBTCPK(),
			)

			if err != nil {
				app.logger.Error(
					"failed to unjail finality-provider",
					zap.String("pk", req.btcPubKey.MarshalHex()),
					zap.Error(err),
				)
				req.errResponse <- err

				continue
			}

			app.logger.Info(
				"successfully unjailed finality-provider on babylon",
				zap.String("btc_pk", req.btcPubKey.MarshalHex()),
				zap.String("txHash", res.TxHash),
			)

			// set the status to INACTIVE by default
			// the status will be changed to ACTIVE
			// if it has voting power for the next height
			app.fps.MustSetFpStatus(req.btcPubKey.MustToBTCPK(), proto.FinalityProviderStatus_INACTIVE)

			req.successResponse <- &UnjailFinalityProviderResponse{
				TxHash: res.TxHash,
			}
		case <-app.quit:
			app.logger.Info("exiting unjailing fp loop")

			return
		}
	}
}

// event loop for metrics update
func (app *FinalityProviderApp) metricsUpdateLoop() {
	defer app.wg.Done()

	interval := app.config.Metrics.UpdateInterval
	app.logger.Info("starting metrics update loop",
		zap.Float64("interval seconds", interval.Seconds()))

	updateTicker := time.NewTicker(interval)
	defer updateTicker.Stop()

	for {
		if app.fpIns != nil {
			app.metrics.UpdateFpMetrics(app.fpIns.GetStoreFinalityProvider())
		}
		select {
		case <-updateTicker.C:
			continue
		case <-app.quit:
			app.logger.Info("exiting metrics update loop")

			return
		}
	}
}
