package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
)

const instanceTerminatingMsg = "terminating the finality-provider instance due to critical error"

type CriticalError struct {
	err     error
	fpBtcPk *bbntypes.BIP340PubKey
}

func (ce *CriticalError) Error() string {
	return fmt.Sprintf("critical err on finality-provider %s: %s", ce.fpBtcPk.MarshalHex(), ce.err.Error())
}

type FinalityProviderManager struct {
	isStarted *atomic.Bool

	// mutex to acess map of fp instances (fpis)
	mu sync.Mutex
	wg sync.WaitGroup

	// running finality-provider instances map keyed by the hex string of the BTC public key
	fpis map[string]*FinalityProviderInstance

	// needed for initiating finality-provider instances
	fps          *store.FinalityProviderStore
	pubRandStore *store.PubRandProofStore
	config       *fpcfg.Config
	cc           clientcontroller.ClientController
	em           eotsmanager.EOTSManager
	logger       *zap.Logger

	metrics *metrics.FpMetrics

	criticalErrChan chan *CriticalError

	quit chan struct{}
}

func NewFinalityProviderManager(
	fps *store.FinalityProviderStore,
	pubRandStore *store.PubRandProofStore,
	config *fpcfg.Config,
	cc clientcontroller.ClientController,
	em eotsmanager.EOTSManager,
	metrics *metrics.FpMetrics,
	logger *zap.Logger,
) (*FinalityProviderManager, error) {
	return &FinalityProviderManager{
		fpis:            make(map[string]*FinalityProviderInstance),
		criticalErrChan: make(chan *CriticalError),
		isStarted:       atomic.NewBool(false),
		fps:             fps,
		pubRandStore:    pubRandStore,
		config:          config,
		cc:              cc,
		em:              em,
		metrics:         metrics,
		logger:          logger,
		quit:            make(chan struct{}),
	}, nil
}

// monitorCriticalErr takes actions when it receives critical errors from a finality-provider instance
// if the finality-provider is slashed, it will be terminated and the program keeps running in case
// new finality providers join
// otherwise, the program will panic
func (fpm *FinalityProviderManager) monitorCriticalErr() {
	defer fpm.wg.Done()

	var criticalErr *CriticalError
	for {
		select {
		case criticalErr = <-fpm.criticalErrChan:
			fpi, err := fpm.GetFinalityProviderInstance(criticalErr.fpBtcPk)
			if err != nil {
				fpm.logger.Debug("the finality-provider instance is already shutdown",
					zap.String("pk", criticalErr.fpBtcPk.MarshalHex()))
				continue
			}
			// cannot use error.Is because the unwrapped error
			// is not the expected error type
			if errors.Is(criticalErr.err, ErrFinalityProviderSlashed) {
				fpm.setFinalityProviderSlashed(fpi)
				fpm.logger.Debug("the finality-provider has been slashed",
					zap.String("pk", criticalErr.fpBtcPk.MarshalHex()))
				continue
			}
			if errors.Is(criticalErr.err, ErrFinalityProviderJailed) {
				fpm.setFinalityProviderJailed(fpi)
				fpm.logger.Debug("the finality-provider has been jailed",
					zap.String("pk", criticalErr.fpBtcPk.MarshalHex()))
				continue
			}
			fpm.logger.Fatal(instanceTerminatingMsg,
				zap.String("pk", criticalErr.fpBtcPk.MarshalHex()), zap.Error(criticalErr.err))
		case <-fpm.quit:
			return
		}
	}
}

// monitorStatusUpdate periodically check the status of each managed finality providers and update
// it accordingly. We update the status by querying the latest voting power and the slashed_height.
// In particular, we perform the following status transitions (REGISTERED, ACTIVE, INACTIVE, SLASHED):
// 1. if power == 0 and slashed_height == 0, if status == ACTIVE, change to INACTIVE, otherwise remain the same
// 2. if power == 0 and slashed_height > 0, set status to SLASHED and stop and remove the finality-provider instance
// 3. if power > 0 (slashed_height must > 0), set status to ACTIVE
// NOTE: once error occurs, we log and continue as the status update is not critical to the entire program
func (fpm *FinalityProviderManager) monitorStatusUpdate() {
	defer fpm.wg.Done()

	if fpm.config.StatusUpdateInterval == 0 {
		fpm.logger.Info("the status update is disabled")
		return
	}

	statusUpdateTicker := time.NewTicker(fpm.config.StatusUpdateInterval)
	defer statusUpdateTicker.Stop()

	for {
		select {
		case <-statusUpdateTicker.C:
			latestBlock, err := fpm.getLatestBlockWithRetry()
			if err != nil {
				fpm.logger.Debug("failed to get the latest block", zap.Error(err))
				continue
			}
			fpis := fpm.ListFinalityProviderInstances()
			for _, fpi := range fpis {
				oldStatus := fpi.GetStatus()
				power, err := fpi.GetVotingPowerWithRetry(latestBlock.Height)
				if err != nil {
					fpm.logger.Debug(
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
						fpm.logger.Debug(
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
					fpm.logger.Debug(
						"failed to get the slashed or jailed status",
						zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
						zap.Error(err),
					)
					continue
				}
				// power == 0 and slashed == true, set status to SLASHED, stop, and remove the finality-provider instance
				if slashed {
					fpm.setFinalityProviderSlashed(fpi)
					fpm.logger.Warn(
						"the finality-provider is slashed",
						zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
						zap.String("old_status", oldStatus.String()),
					)
					continue
				}
				// power == 0 and jailed == true, set status to JAILED, stop, and remove the finality-provider instance
				if jailed {
					fpm.setFinalityProviderJailed(fpi)
					fpm.logger.Warn(
						"the finality-provider is jailed",
						zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
						zap.String("old_status", oldStatus.String()),
					)
					continue
				}
				// power == 0 and slashed_height == 0, change to INACTIVE if the current status is ACTIVE
				if oldStatus == proto.FinalityProviderStatus_ACTIVE {
					fpi.MustSetStatus(proto.FinalityProviderStatus_INACTIVE)
					fpm.logger.Debug(
						"the finality-provider status is changed to INACTIVE",
						zap.String("fp_btc_pk", fpi.GetBtcPkHex()),
						zap.String("old_status", oldStatus.String()),
					)
				}
			}
		case <-fpm.quit:
			return
		}
	}
}

func (fpm *FinalityProviderManager) setFinalityProviderSlashed(fpi *FinalityProviderInstance) {
	fpi.MustSetStatus(proto.FinalityProviderStatus_SLASHED)
	if err := fpm.removeFinalityProviderInstance(fpi.GetBtcPkBIP340()); err != nil {
		panic(fmt.Errorf("failed to terminate a slashed finality-provider %s: %w", fpi.GetBtcPkHex(), err))
	}
}

func (fpm *FinalityProviderManager) setFinalityProviderJailed(fpi *FinalityProviderInstance) {
	fpi.MustSetStatus(proto.FinalityProviderStatus_JAILED)
	if err := fpm.removeFinalityProviderInstance(fpi.GetBtcPkBIP340()); err != nil {
		panic(fmt.Errorf("failed to terminate a jailed finality-provider %s: %w", fpi.GetBtcPkHex(), err))
	}
}

func (fpm *FinalityProviderManager) StartFinalityProvider(fpPk *bbntypes.BIP340PubKey, passphrase string) error {
	if !fpm.isStarted.Load() {
		fpm.isStarted.Store(true)

		fpm.wg.Add(1)
		go fpm.monitorCriticalErr()

		fpm.wg.Add(1)
		go fpm.monitorStatusUpdate()
	}

	if fpm.numOfRunningFinalityProviders() >= int(fpm.config.MaxNumFinalityProviders) {
		return fmt.Errorf("reaching maximum number of running finality providers %v", fpm.config.MaxNumFinalityProviders)
	}

	if err := fpm.addFinalityProviderInstance(fpPk, passphrase); err != nil {
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) StartAll() error {
	if !fpm.isStarted.Load() {
		fpm.isStarted.Store(true)

		fpm.wg.Add(1)
		go fpm.monitorCriticalErr()

		fpm.wg.Add(1)
		go fpm.monitorStatusUpdate()
	}

	storedFps, err := fpm.fps.GetAllStoredFinalityProviders()
	if err != nil {
		return err
	}

	for _, fp := range storedFps {
		fpBtcPk := fp.GetBIP340BTCPK()
		if !fp.ShouldStart() {
			fpm.logger.Info(
				"the finality provider cannot be started with status",
				zap.String("eots-pk", fpBtcPk.MarshalHex()),
				zap.String("status", fp.Status.String()),
			)
			continue
		}
		if err := fpm.StartFinalityProvider(fpBtcPk, ""); err != nil {
			return err
		}
	}

	return nil
}

func (fpm *FinalityProviderManager) Stop() error {
	if !fpm.isStarted.Swap(false) {
		return fmt.Errorf("the finality-provider manager has already stopped")
	}

	var stopErr error
	for _, fpi := range fpm.fpis {
		if !fpi.IsRunning() {
			continue
		}
		if err := fpi.Stop(); err != nil {
			stopErr = err
			break
		}
		fpm.metrics.DecrementRunningFpGauge()
	}

	close(fpm.quit)
	fpm.wg.Wait()

	return stopErr
}

func (fpm *FinalityProviderManager) ListFinalityProviderInstances() []*FinalityProviderInstance {
	fpm.mu.Lock()
	defer fpm.mu.Unlock()

	fpisList := make([]*FinalityProviderInstance, 0, len(fpm.fpis))
	for _, fpi := range fpm.fpis {
		fpisList = append(fpisList, fpi)
	}

	return fpisList
}

func (fpm *FinalityProviderManager) AllFinalityProviders() ([]*proto.FinalityProviderInfo, error) {
	storedFps, err := fpm.fps.GetAllStoredFinalityProviders()
	if err != nil {
		return nil, err
	}

	fpsInfo := make([]*proto.FinalityProviderInfo, 0, len(storedFps))
	for _, fp := range storedFps {
		fpInfo := fp.ToFinalityProviderInfo()

		if fpm.IsFinalityProviderRunning(fp.GetBIP340BTCPK()) {
			fpInfo.IsRunning = true
		}

		fpsInfo = append(fpsInfo, fpInfo)
	}

	return fpsInfo, nil
}

func (fpm *FinalityProviderManager) FinalityProviderInfo(fpPk *bbntypes.BIP340PubKey) (*proto.FinalityProviderInfo, error) {
	storedFp, err := fpm.fps.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return nil, err
	}

	fpInfo := storedFp.ToFinalityProviderInfo()

	if fpm.IsFinalityProviderRunning(fpPk) {
		fpInfo.IsRunning = true
	}

	return fpInfo, nil
}

func (fpm *FinalityProviderManager) IsFinalityProviderRunning(fpPk *bbntypes.BIP340PubKey) bool {
	fpm.mu.Lock()
	defer fpm.mu.Unlock()

	_, exists := fpm.fpis[fpPk.MarshalHex()]
	return exists
}

func (fpm *FinalityProviderManager) GetFinalityProviderInstance(fpPk *bbntypes.BIP340PubKey) (*FinalityProviderInstance, error) {
	fpm.mu.Lock()
	defer fpm.mu.Unlock()

	keyHex := fpPk.MarshalHex()
	v, exists := fpm.fpis[keyHex]
	if !exists {
		return nil, fmt.Errorf("cannot find the finality-provider instance with PK: %s", keyHex)
	}

	return v, nil
}

func (fpm *FinalityProviderManager) removeFinalityProviderInstance(fpPk *bbntypes.BIP340PubKey) error {
	fpm.mu.Lock()
	defer fpm.mu.Unlock()

	keyHex := fpPk.MarshalHex()
	fpi, exists := fpm.fpis[keyHex]
	if !exists {
		return fmt.Errorf("cannot find the finality-provider instance with PK: %s", keyHex)
	}
	if fpi.IsRunning() {
		if err := fpi.Stop(); err != nil {
			return fmt.Errorf("failed to stop the finality-provider instance %s", keyHex)
		}
	}

	delete(fpm.fpis, keyHex)
	fpm.metrics.DecrementRunningFpGauge()
	return nil
}

func (fpm *FinalityProviderManager) numOfRunningFinalityProviders() int {
	fpm.mu.Lock()
	defer fpm.mu.Unlock()

	return len(fpm.fpis)
}

// addFinalityProviderInstance creates a finality-provider instance, starts it and adds it into the finality-provider manager
func (fpm *FinalityProviderManager) addFinalityProviderInstance(
	pk *bbntypes.BIP340PubKey,
	passphrase string,
) error {
	fpm.mu.Lock()
	defer fpm.mu.Unlock()

	pkHex := pk.MarshalHex()
	if _, exists := fpm.fpis[pkHex]; exists {
		return fmt.Errorf("finality-provider instance already exists")
	}

	fpIns, err := NewFinalityProviderInstance(pk, fpm.config, fpm.fps, fpm.pubRandStore, fpm.cc, fpm.em, fpm.metrics, passphrase, fpm.criticalErrChan, fpm.logger)
	if err != nil {
		return fmt.Errorf("failed to create finality-provider %s instance: %w", pkHex, err)
	}

	if err := fpIns.Start(); err != nil {
		return fmt.Errorf("failed to start finality-provider %s instance: %w", pkHex, err)
	}

	fpm.fpis[pkHex] = fpIns
	fpm.metrics.IncrementRunningFpGauge()

	return nil
}

func (fpm *FinalityProviderManager) getLatestBlockWithRetry() (*types.BlockInfo, error) {
	var (
		latestBlock *types.BlockInfo
		err         error
	)

	if err := retry.Do(func() error {
		latestBlock, err = fpm.cc.QueryBestBlock()
		if err != nil {
			return err
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		fpm.logger.Debug(
			"failed to query the consumer chain for the latest block",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return nil, err
	}

	return latestBlock, nil
}
