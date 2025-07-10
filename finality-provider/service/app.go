package service

import (
	"errors"
	"fmt"
	"github.com/babylonlabs-io/finality-provider/types"
	"strings"
	"sync"

	"github.com/babylonlabs-io/finality-provider/finality-provider/signingcontext"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	bstypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"

	fpcc "github.com/babylonlabs-io/finality-provider/clientcontroller"
	ccapi "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	fpkr "github.com/babylonlabs-io/finality-provider/keyring"
	"github.com/babylonlabs-io/finality-provider/metrics"
)

type FinalityProviderApp struct {
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
	quit      chan struct{}

	cc           ccapi.ClientController
	consumerCon  ccapi.ConsumerController
	kr           keyring.Keyring
	fps          *store.FinalityProviderStore
	pubRandStore *store.PubRandProofStore
	config       *fpcfg.Config
	logger       *zap.Logger
	poller       types.BlockPoller[types.BlockDescription]

	fpInsMu     sync.RWMutex // Protects fpIns
	fpIns       *FinalityProviderInstance
	eotsManager eotsmanager.EOTSManager

	metrics *metrics.FpMetrics

	createFinalityProviderRequestChan chan *CreateFinalityProviderRequest
	unjailFinalityProviderRequestChan chan *UnjailFinalityProviderRequest
	criticalErrChan                   chan *CriticalError
}

// NewFinalityProviderAppFromConfig creates a new FinalityProviderApp instance from the given configuration.
func NewFinalityProviderAppFromConfig(
	cfg *fpcfg.Config,
	db kvdb.Backend,
	logger *zap.Logger,
) (*FinalityProviderApp, error) {
	cc, err := fpcc.NewBabylonController(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client for the Babylon chain: %w", err)
	}
	if err := cc.Start(); err != nil {
		return nil, fmt.Errorf("failed to start rpc client for the Babylon chain: %w", err)
	}

	consumerCon, err := fpcc.NewConsumerController(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client for the consumer chain %s: %w", cfg.ChainType, err)
	}

	// if the EOTSManagerAddress is empty, run a local EOTS manager;
	// otherwise connect a remote one with a gRPC client
	em, err := InitEOTSManagerClient(cfg.EOTSManagerAddress, cfg.HMACKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	logger.Info("successfully connected to a remote EOTS manager", zap.String("address", cfg.EOTSManagerAddress))

	fpMetrics := metrics.NewFpMetrics()
	poller := NewChainPoller(logger, cfg.PollerConfig, consumerCon, fpMetrics)

	return NewFinalityProviderApp(cfg, cc, consumerCon, em, poller, fpMetrics, db, logger)
}

func NewFinalityProviderApp(
	config *fpcfg.Config,
	cc ccapi.ClientController,
	consumerCon ccapi.ConsumerController,
	em eotsmanager.EOTSManager,
	poller types.BlockPoller[types.BlockDescription],
	metrics *metrics.FpMetrics,
	db kvdb.Backend,
	logger *zap.Logger,
) (*FinalityProviderApp, error) {
	fpStore, err := store.NewFinalityProviderStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate finality provider store: %w", err)
	}
	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate public randomness store: %w", err)
	}

	kr, err := fpkr.CreateKeyring(
		config.BabylonConfig.KeyDirectory,
		config.BabylonConfig.ChainID,
		config.BabylonConfig.KeyringBackend,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	return &FinalityProviderApp{
		cc:                                cc,
		consumerCon:                       consumerCon,
		fps:                               fpStore,
		pubRandStore:                      pubRandStore,
		kr:                                kr,
		config:                            config,
		logger:                            logger,
		eotsManager:                       em,
		poller:                            poller,
		metrics:                           metrics,
		quit:                              make(chan struct{}),
		unjailFinalityProviderRequestChan: make(chan *UnjailFinalityProviderRequest),
		createFinalityProviderRequestChan: make(chan *CreateFinalityProviderRequest),
		criticalErrChan:                   make(chan *CriticalError),
	}, nil
}

func (app *FinalityProviderApp) GetConfig() *fpcfg.Config {
	return app.config
}

func (app *FinalityProviderApp) GetBabylonController() ccapi.ClientController {
	return app.cc
}

func (app *FinalityProviderApp) GetConsumerController() ccapi.ConsumerController {
	return app.consumerCon
}

func (app *FinalityProviderApp) GetFinalityProviderStore() *store.FinalityProviderStore {
	return app.fps
}

func (app *FinalityProviderApp) GetPubRandProofStore() *store.PubRandProofStore {
	return app.pubRandStore
}

func (app *FinalityProviderApp) GetFinalityProviderInfo(fpPk *bbntypes.BIP340PubKey) (*proto.FinalityProviderInfo, error) {
	storedFp, err := app.fps.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return nil, err
	}

	fpInfo := storedFp.ToFinalityProviderInfo()

	if app.IsFinalityProviderRunning(fpPk) {
		fpInfo.IsRunning = true
	}

	return fpInfo, nil
}

func (app *FinalityProviderApp) ListAllFinalityProvidersInfo() ([]*proto.FinalityProviderInfo, error) {
	storedFps, err := app.fps.GetAllStoredFinalityProviders()
	if err != nil {
		return nil, err
	}

	fpsInfo := make([]*proto.FinalityProviderInfo, 0, len(storedFps))
	for _, fp := range storedFps {
		fpInfo := fp.ToFinalityProviderInfo()

		if app.IsFinalityProviderRunning(fp.GetBIP340BTCPK()) {
			fpInfo.IsRunning = true
		}

		fpsInfo = append(fpsInfo, fpInfo)
	}

	return fpsInfo, nil
}

// GetFinalityProviderInstance returns the finality-provider instance with the given Babylon public key
func (app *FinalityProviderApp) GetFinalityProviderInstance() (*FinalityProviderInstance, error) {
	app.fpInsMu.RLock()
	defer app.fpInsMu.RUnlock()

	if app.fpIns == nil {
		return nil, fmt.Errorf("finality provider does not exist")
	}

	return app.fpIns, nil
}

func (app *FinalityProviderApp) Logger() *zap.Logger {
	return app.logger
}

// StartFinalityProvider starts a finality provider instance with the given EOTS public key
// Note: this should be called right after the finality-provider is registered
func (app *FinalityProviderApp) StartFinalityProvider(fpPk *bbntypes.BIP340PubKey) error {
	app.logger.Info("starting finality provider", zap.String("pk", fpPk.MarshalHex()))

	if err := app.startFinalityProviderInstance(fpPk); err != nil {
		return err
	}

	app.logger.Info("finality provider is started", zap.String("pk", fpPk.MarshalHex()))

	return nil
}

// SyncAllFinalityProvidersStatus syncs the status of all the stored finality providers with the chain.
// it should be called before a fp instance is started
func (app *FinalityProviderApp) SyncAllFinalityProvidersStatus() error {
	fps, err := app.fps.GetAllStoredFinalityProviders()
	if err != nil {
		return err
	}

	for _, fp := range fps {
		latestBlockHeight, err := app.consumerCon.QueryLatestBlockHeight()
		if err != nil {
			return err
		}

		pkHex := fp.GetBIP340BTCPK().MarshalHex()
		hasPower, err := app.consumerCon.QueryFinalityProviderHasPower(fp.BtcPk, latestBlockHeight)
		if err != nil {
			return fmt.Errorf("failed to query voting power for finality provider %s at height %d: %w",
				fp.GetBIP340BTCPK().MarshalHex(), latestBlockHeight, err)
		}

		// power > 0 (slashed_height must > 0), set status to ACTIVE
		oldStatus := fp.Status
		if hasPower {
			if oldStatus != proto.FinalityProviderStatus_ACTIVE {
				fp.Status = proto.FinalityProviderStatus_ACTIVE
				app.fps.MustSetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_ACTIVE)
				app.logger.Debug(
					"the finality-provider status is changed to ACTIVE",
					zap.String("fp_btc_pk", pkHex),
					zap.String("old_status", oldStatus.String()),
				)
			}

			continue
		}
		slashed, jailed, err := app.consumerCon.QueryFinalityProviderSlashedOrJailed(fp.BtcPk)
		if err != nil {
			return err
		}
		if slashed {
			app.fps.MustSetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_SLASHED)

			app.logger.Debug(
				"the finality-provider status is changed to SLAHED",
				zap.String("fp_btc_pk", pkHex),
				zap.String("old_status", oldStatus.String()),
			)

			continue
		}
		if jailed {
			app.fps.MustSetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_JAILED)

			app.logger.Debug(
				"the finality-provider status is changed to JAILED",
				zap.String("fp_btc_pk", pkHex),
				zap.String("old_status", oldStatus.String()),
			)

			continue
		}

		app.fps.MustSetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_INACTIVE)

		app.logger.Debug(
			"the finality-provider status is changed to INACTIVE",
			zap.String("fp_btc_pk", pkHex),
			zap.String("old_status", oldStatus.String()),
		)
	}

	return nil
}

// Start starts only the finality-provider daemon without any finality-provider instances
func (app *FinalityProviderApp) Start() error {
	var startErr error
	app.startOnce.Do(func() {
		app.logger.Info("Starting FinalityProviderApp")

		startErr = app.SyncAllFinalityProvidersStatus()
		if startErr != nil {
			return
		}

		app.wg.Add(4)
		go app.metricsUpdateLoop()
		go app.monitorCriticalErr()
		go app.registrationLoop()
		go app.unjailFpLoop()
	})

	return startErr
}

func (app *FinalityProviderApp) Stop() error {
	var stopErr error
	app.stopOnce.Do(func() {
		app.logger.Info("Stopping FinalityProviderApp")

		close(app.quit)
		app.wg.Wait()

		if app.fpIns != nil && app.fpIns.IsRunning() {
			pkHex := app.fpIns.GetBtcPkHex()
			app.logger.Info("stopping finality provider", zap.String("pk", pkHex))

			if err := app.fpIns.Stop(); err != nil {
				stopErr = fmt.Errorf("failed to close the fp instance: %w", err)

				return
			}

			app.logger.Info("finality provider is stopped", zap.String("pk", pkHex))
		}

		app.logger.Debug("Stopping EOTS manager")
		if err := app.eotsManager.Close(); err != nil {
			stopErr = fmt.Errorf("failed to close the EOTS manager: %w", err)

			return
		}

		app.logger.Debug("FinalityProviderApp successfully stopped")
	})

	return stopErr
}

func (app *FinalityProviderApp) CreateFinalityProvider(
	keyName, chainID string,
	eotsPk *bbntypes.BIP340PubKey,
	description *stakingtypes.Description,
	commission bstypes.CommissionRates,
) (*CreateFinalityProviderResult, error) {
	// 1. check if the chain key exists
	kr, err := fpkr.NewChainKeyringControllerWithKeyring(app.kr, keyName)
	if err != nil {
		return nil, err
	}

	fpAddr, err := kr.Address()
	if err != nil {
		// the chain key does not exist, should create the chain key first
		return nil, fmt.Errorf("the keyname %s does not exist, add the key first: %w", keyName, err)
	}

	// 2. create proof-of-possession
	if eotsPk == nil {
		return nil, fmt.Errorf("eots pk cannot be nil")
	}

	var signCtx string
	nextHeight := app.poller.NextHeight()
	//  nextHeight-1 might underflow if the nextHeight is 0
	if (nextHeight == 0 && app.config.ContextSigningHeight > 0) ||
		(nextHeight > 0 && app.config.ContextSigningHeight > nextHeight-1) {
		signCtx = signingcontext.FpPopContextV0(chainID, signingcontext.AccBTCStaking.String())
	}

	pop, err := app.CreatePop(fpAddr, eotsPk, signCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create proof-of-possession of the finality-provider: %w", err)
	}

	// Query the consumer chain to check if the fp is already registered
	// if true, update db with the fp info from the consumer chain
	// otherwise, proceed registration
	resp, err := app.cc.QueryFinalityProvider(eotsPk.MustToBTCPK())
	if err != nil {
		if !strings.Contains(err.Error(), "the finality provider is not found") {
			return nil, fmt.Errorf("err getting finality provider: %w", err)
		}
	}
	if resp != nil {
		app.logger.Info("finality-provider already registered on the consumer chain",
			zap.String("eots_pk", resp.FinalityProvider.BtcPk.MarshalHex()),
			zap.String("addr", resp.FinalityProvider.Addr),
		)

		if err := app.putFpFromResponse(resp.FinalityProvider, chainID); err != nil {
			return nil, err
		}

		// get updated fp from db
		storedFp, err := app.fps.GetFinalityProvider(eotsPk.MustToBTCPK())
		if err != nil {
			return nil, err
		}

		return &CreateFinalityProviderResult{
			FpInfo: storedFp.ToFinalityProviderInfo(),
		}, nil
	}

	request := &CreateFinalityProviderRequest{
		chainID:         chainID,
		fpAddr:          fpAddr,
		btcPubKey:       eotsPk,
		pop:             pop,
		description:     description,
		commission:      commission,
		errResponse:     make(chan error, 1),
		successResponse: make(chan *RegisterFinalityProviderResponse, 1),
	}

	app.createFinalityProviderRequestChan <- request

	select {
	case err := <-request.errResponse:
		return nil, err
	case successResponse := <-request.successResponse:
		pkHex := eotsPk.MarshalHex()
		btcPk := eotsPk.MustToBTCPK()
		// save the fp info to db after successful registration
		// this ensures the data saved in db is consistent with that on the consumer chain
		// if the program crashes in the middle, the user can retry registration
		// which will update db use the information from the consumer chain without
		// submitting a registration again
		if err := app.fps.CreateFinalityProvider(fpAddr, btcPk, description, commission, chainID); err != nil {
			return nil, fmt.Errorf("failed to save finality-provider: %w", err)
		}

		app.metrics.RecordFpStatus(pkHex, proto.FinalityProviderStatus_REGISTERED)

		app.logger.Info("successfully saved the finality-provider",
			zap.String("eots_pk", pkHex),
			zap.String("addr", fpAddr.String()),
		)

		storedFp, err := app.fps.GetFinalityProvider(btcPk)
		if err != nil {
			return nil, err
		}

		return &CreateFinalityProviderResult{
			FpInfo: storedFp.ToFinalityProviderInfo(),
			TxHash: successResponse.txHash,
		}, nil
	case <-app.quit:
		return nil, fmt.Errorf("finality-provider app is shutting down")
	}
}

// UnjailFinalityProvider sends a transaction to unjail a finality-provider
func (app *FinalityProviderApp) UnjailFinalityProvider(fpPk *bbntypes.BIP340PubKey) (*UnjailFinalityProviderResponse, error) {
	// send request to the loop to avoid blocking the main thread
	request := &UnjailFinalityProviderRequest{
		btcPubKey:       fpPk,
		errResponse:     make(chan error, 1),
		successResponse: make(chan *UnjailFinalityProviderResponse, 1),
	}

	app.unjailFinalityProviderRequestChan <- request

	select {
	case err := <-request.errResponse:
		return nil, err
	case successResponse := <-request.successResponse:
		_, err := app.fps.GetFinalityProvider(fpPk.MustToBTCPK())
		if err != nil {
			return nil, fmt.Errorf("failed to get finality provider from db: %w", err)
		}

		// Update finality-provider status in the local store
		// set it to INACTIVE for now and it will be updated to
		// ACTIVE if the fp has voting power
		err = app.fps.SetFpStatus(fpPk.MustToBTCPK(), proto.FinalityProviderStatus_INACTIVE)
		if err != nil {
			return nil, fmt.Errorf("failed to update finality-provider status after unjailing: %w", err)
		}

		app.metrics.RecordFpStatus(fpPk.MarshalHex(), proto.FinalityProviderStatus_INACTIVE)

		return successResponse, nil
	case <-app.quit:
		return nil, fmt.Errorf("finality-provider app is shutting down")
	}
}

func (app *FinalityProviderApp) CreatePop(fpAddress sdk.AccAddress, fpPk *bbntypes.BIP340PubKey, signCtx string) (*bstypes.ProofOfPossessionBTC, error) {
	pop := &bstypes.ProofOfPossessionBTC{
		BtcSigType: bstypes.BTCSigType_BIP340, // by default, we use BIP-340 encoding for BTC signature
	}
	// generate pop.BtcSig = schnorr_sign(sk_BTC, hash(bbnAddress))
	// NOTE: *schnorr.Sign has to take the hash of the message.
	// So we have to hash the address before signing
	hasher := tmhash.New()
	if len(signCtx) > 0 {
		if _, err := hasher.Write([]byte(signCtx)); err != nil {
			return nil, fmt.Errorf("failed to write signing context to the hash: %w", err)
		}
	}

	if _, err := hasher.Write(fpAddress.Bytes()); err != nil {
		return nil, fmt.Errorf("failed to write fp address to the hash: %w", err)
	}

	sig, err := app.eotsManager.SignSchnorrSig(fpPk.MustMarshal(), hasher.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("failed to get schnorr signature from the EOTS manager: %w", err)
	}

	pop.BtcSig = bbntypes.NewBIP340SignatureFromBTCSig(sig).MustMarshal()

	return pop, nil
}

func (app *FinalityProviderApp) startFinalityProviderInstance(
	pk *bbntypes.BIP340PubKey,
) error {
	pkHex := pk.MarshalHex()
	app.fpInsMu.Lock()
	defer app.fpInsMu.Unlock()

	if app.fpIns == nil {
		fpIns, err := NewFinalityProviderInstance(
			pk, app.config, app.fps, app.pubRandStore, app.cc, app.consumerCon, app.eotsManager, app.poller,
			app.metrics, app.criticalErrChan, app.logger,
		)
		if err != nil {
			return fmt.Errorf("failed to create finality provider instance %s: %w", pkHex, err)
		}

		app.fpIns = fpIns
	} else if !pk.Equals(app.fpIns.btcPk) {
		return fmt.Errorf("the finality provider daemon is already bonded with the finality provider %s,"+
			"please restart the daemon to switch to another instance", app.fpIns.btcPk.MarshalHex())
	}

	return app.fpIns.Start()
}

func (app *FinalityProviderApp) IsFinalityProviderRunning(fpPk *bbntypes.BIP340PubKey) bool {
	app.fpInsMu.RLock()
	defer app.fpInsMu.RUnlock()

	if app.fpIns == nil {
		return false
	}

	if app.fpIns.GetBtcPkHex() != fpPk.MarshalHex() {
		return false
	}

	return app.fpIns.IsRunning()
}

func (app *FinalityProviderApp) removeFinalityProviderInstance() error {
	app.fpInsMu.Lock()
	defer app.fpInsMu.Unlock()

	fpi := app.fpIns
	if fpi == nil {
		return fmt.Errorf("the finality provider instance does not exist")
	}
	if fpi.IsRunning() {
		if err := fpi.Stop(); err != nil {
			return fmt.Errorf("failed to stop the finality provider instance %s", fpi.GetBtcPkHex())
		}
	}

	app.fpIns = nil

	return nil
}

func (app *FinalityProviderApp) setFinalityProviderSlashed(fpi *FinalityProviderInstance) {
	fpi.MustSetStatus(proto.FinalityProviderStatus_SLASHED)
	if err := app.removeFinalityProviderInstance(); err != nil {
		panic(fmt.Errorf("failed to terminate a slashed finality-provider %s: %w", fpi.GetBtcPkHex(), err))
	}
}

// putFpFromResponse creates or updates finality-provider in the local store
func (app *FinalityProviderApp) putFpFromResponse(fp *bstypes.FinalityProviderResponse, chainID string) error {
	btcPk := fp.BtcPk.MustToBTCPK()
	_, err := app.fps.GetFinalityProvider(btcPk)
	if err != nil {
		if errors.Is(err, store.ErrFinalityProviderNotFound) {
			addr, err := sdk.AccAddressFromBech32(fp.Addr)
			if err != nil {
				return fmt.Errorf("err converting fp addr: %w", err)
			}

			if fp.Commission == nil {
				return errors.New("nil Commission in FinalityProviderResponse")
			}

			if fp.CommissionInfo == nil {
				return errors.New("nil CommissionInfo in FinalityProviderResponse")
			}

			commRates := bstypes.NewCommissionRates(*fp.Commission, fp.CommissionInfo.MaxRate, fp.CommissionInfo.MaxChangeRate)

			if err := app.fps.CreateFinalityProvider(addr, btcPk, fp.Description, commRates, chainID); err != nil {
				return fmt.Errorf("failed to save finality-provider: %w", err)
			}

			app.logger.Info("finality-provider successfully saved the local db",
				zap.String("eots_pk", fp.BtcPk.MarshalHex()),
				zap.String("addr", fp.Addr),
			)

			return nil
		}

		return err
	}

	if err := app.fps.SetFpDescription(btcPk, fp.Description, fp.Commission); err != nil {
		return err
	}

	if err := app.fps.SetFpLastVotedHeight(btcPk, uint64(fp.HighestVotedHeight)); err != nil {
		return err
	}

	hasPower, err := app.consumerCon.QueryFinalityProviderHasPower(btcPk, fp.Height)
	if err != nil {
		return fmt.Errorf("failed to query voting power for finality provider %s: %w",
			fp.BtcPk.MarshalHex(), err)
	}

	var status proto.FinalityProviderStatus
	switch {
	case hasPower:
		status = proto.FinalityProviderStatus_ACTIVE
	case fp.SlashedBtcHeight > 0:
		status = proto.FinalityProviderStatus_SLASHED
	case fp.Jailed:
		status = proto.FinalityProviderStatus_JAILED
	default:
		status = proto.FinalityProviderStatus_INACTIVE
	}

	if err := app.fps.SetFpStatus(btcPk, status); err != nil {
		return fmt.Errorf("failed to update status for finality provider %s: %w", fp.BtcPk.MarshalHex(), err)
	}

	app.logger.Info("finality-provider successfully updated the local db",
		zap.String("eots_pk", fp.BtcPk.MarshalHex()),
		zap.String("addr", fp.Addr),
	)

	return nil
}
