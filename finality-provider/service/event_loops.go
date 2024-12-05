package service

import (
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
)

// monitorStatusUpdate periodically check the status of the running finality provider and update
// it accordingly. We update the status by querying the latest voting power and the slashed_height.
// In particular, we perform the following status transitions (REGISTERED, ACTIVE, INACTIVE, SLASHED):
// 1. if power == 0 and slashed_height == 0, if status == ACTIVE, change to INACTIVE, otherwise remain the same
// 2. if power == 0 and slashed_height > 0, set status to SLASHED and stop and remove the finality-provider instance
// 3. if power > 0 (slashed_height must > 0), set status to ACTIVE
// NOTE: once error occurs, we log and continue as the status update is not critical to the entire program
func (app *FinalityProviderApp) monitorStatusUpdate() {
	defer app.wg.Done()

	if app.config.StatusUpdateInterval == 0 {
		app.logger.Info("the status update is disabled")
		return
	}

	statusUpdateTicker := time.NewTicker(app.config.StatusUpdateInterval)
	defer statusUpdateTicker.Stop()

	for {
		select {
		case <-statusUpdateTicker.C:
			fpi := app.fpIns
			if fpi == nil {
				continue
			}

			latestBlock, err := app.getLatestBlockWithRetry()
			if err != nil {
				app.logger.Debug("failed to get the latest block", zap.Error(err))
				continue
			}
			oldStatus := fpi.GetStatus()
			power, err := fpi.GetVotingPowerWithRetry(latestBlock.Height)
			if err != nil {
				app.logger.Debug(
					"failed to get the voting power",
					zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
					zap.Uint64("height", latestBlock.Height),
					zap.Error(err),
				)
				continue
			}
			// power > 0 (slashed_height must > 0), set status to ACTIVE
			if power > 0 {
				if oldStatus != proto.FinalityProviderStatus_ACTIVE {
					fpi.MustSetStatus(proto.FinalityProviderStatus_ACTIVE)
					app.logger.Debug(
						"the finality-provider status is changed to ACTIVE",
						zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
						zap.String("old_status", oldStatus.String()),
						zap.Uint64("power", power),
					)
				}
				continue
			}
			slashed, jailed, err := fpi.GetFinalityProviderSlashedOrJailedWithRetry()
			if err != nil {
				app.logger.Debug(
					"failed to get the slashed or jailed status",
					zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
					zap.Error(err),
				)
				continue
			}
			// power == 0 and slashed == true, set status to SLASHED, stop, and remove the finality-provider instance
			if slashed {
				app.setFinalityProviderSlashed(fpi)
				app.logger.Warn(
					"the finality-provider is slashed",
					zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
					zap.String("old_status", oldStatus.String()),
				)
				continue
			}
			// power == 0 and jailed == true, set status to JAILED, stop, and remove the finality-provider instance
			if jailed {
				app.setFinalityProviderJailed(fpi)
				app.logger.Warn(
					"the finality-provider is jailed",
					zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
					zap.String("old_status", oldStatus.String()),
				)
				continue
			}
			// power == 0 and slashed_height == 0, change to INACTIVE if the current status is ACTIVE
			if oldStatus == proto.FinalityProviderStatus_ACTIVE {
				fpi.MustSetStatus(proto.FinalityProviderStatus_INACTIVE)
				app.logger.Debug(
					"the finality-provider status is changed to INACTIVE",
					zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
					zap.String("old_status", oldStatus.String()),
				)
			}
		case <-app.quit:
			app.logger.Info("exiting monitor fp status update loop")
			return
		}
	}
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
			if errors.Is(criticalErr.err, ErrFinalityProviderJailed) {
				app.setFinalityProviderJailed(fpi)
				app.logger.Debug("the finality-provider has been jailed",
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

// event loop for metrics update
func (app *FinalityProviderApp) metricsUpdateLoop() {
	defer app.wg.Done()

	interval := app.config.Metrics.UpdateInterval
	app.logger.Info("starting metrics update loop",
		zap.Float64("interval seconds", interval.Seconds()))

	updateTicker := time.NewTicker(interval)
	defer updateTicker.Stop()

	for {
		select {
		case <-updateTicker.C:
			fps, err := app.fps.GetAllStoredFinalityProviders()
			if err != nil {
				app.logger.Error("failed to get finality-providers from the store", zap.Error(err))
				continue
			}
			app.metrics.UpdateFpMetrics(fps)
		case <-app.quit:
			app.logger.Info("exiting metrics update loop")
			return
		}
	}
}

// syncChainFpStatusLoop keeps querying the chain for the finality
// provider voting power and update the FP status accordingly.
// If there is some voting power it sets to active, for zero voting power
// it goes from: CREATED -> REGISTERED or ACTIVE -> INACTIVE.
// if there is any node running or a new finality provider instance
// is started, the loop stops.
func (app *FinalityProviderApp) syncChainFpStatusLoop() {
	defer app.wg.Done()

	interval := app.config.SyncFpStatusInterval
	app.logger.Info(
		"starting sync FP status loop",
		zap.Float64("interval seconds", interval.Seconds()),
	)
	syncFpStatusTicker := time.NewTicker(interval)
	defer syncFpStatusTicker.Stop()

	for {
		select {
		case <-syncFpStatusTicker.C:
			fpInstanceStarted, err := app.SyncFinalityProviderStatus()
			if err != nil {
				app.logger.Error("failed to sync finality-provider status", zap.Error(err))
			}
			if fpInstanceStarted {
				return
			}

		case <-app.quit:
			app.logger.Info("exiting sync FP status loop")
			return
		}
	}
}