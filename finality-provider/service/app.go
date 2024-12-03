package service

import (
	"fmt"
	"strings"
	"sync"

	sdkmath "cosmossdk.io/math"
	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	fpkr "github.com/babylonlabs-io/finality-provider/keyring"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/types"
)

type FinalityProviderApp struct {
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
	quit      chan struct{}

	cc           clientcontroller.ClientController
	kr           keyring.Keyring
	fps          *store.FinalityProviderStore
	pubRandStore *store.PubRandProofStore
	config       *fpcfg.Config
	logger       *zap.Logger
	input        *strings.Reader

	fpIns       *FinalityProviderInstance
	eotsManager eotsmanager.EOTSManager

	metrics *metrics.FpMetrics

	createFinalityProviderRequestChan   chan *createFinalityProviderRequest
	registerFinalityProviderRequestChan chan *registerFinalityProviderRequest
	finalityProviderRegisteredEventChan chan *finalityProviderRegisteredEvent
	criticalErrChan                     chan *CriticalError
}

func NewFinalityProviderAppFromConfig(
	cfg *fpcfg.Config,
	db kvdb.Backend,
	logger *zap.Logger,
) (*FinalityProviderApp, error) {
	cc, err := clientcontroller.NewClientController(cfg.ChainType, cfg.BabylonConfig, &cfg.BTCNetParams, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client for the consumer chain %s: %w", cfg.ChainType, err)
	}

	// if the EOTSManagerAddress is empty, run a local EOTS manager;
	// otherwise connect a remote one with a gRPC client
	em, err := client.NewEOTSManagerGRpcClient(cfg.EOTSManagerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	logger.Info("successfully connected to a remote EOTS manager", zap.String("address", cfg.EOTSManagerAddress))
	return NewFinalityProviderApp(cfg, cc, em, db, logger)
}

func NewFinalityProviderApp(
	config *fpcfg.Config,
	cc clientcontroller.ClientController,
	em eotsmanager.EOTSManager,
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

	input := strings.NewReader("")
	kr, err := fpkr.CreateKeyring(
		config.BabylonConfig.KeyDirectory,
		config.BabylonConfig.ChainID,
		config.BabylonConfig.KeyringBackend,
		input,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	fpMetrics := metrics.NewFpMetrics()

	return &FinalityProviderApp{
		cc:                                  cc,
		fps:                                 fpStore,
		pubRandStore:                        pubRandStore,
		kr:                                  kr,
		config:                              config,
		logger:                              logger,
		input:                               input,
		fpIns:                               nil,
		eotsManager:                         em,
		metrics:                             fpMetrics,
		quit:                                make(chan struct{}),
		createFinalityProviderRequestChan:   make(chan *createFinalityProviderRequest),
		registerFinalityProviderRequestChan: make(chan *registerFinalityProviderRequest),
		finalityProviderRegisteredEventChan: make(chan *finalityProviderRegisteredEvent),
		criticalErrChan:                     make(chan *CriticalError),
	}, nil
}

func (app *FinalityProviderApp) GetConfig() *fpcfg.Config {
	return app.config
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
	if app.fpIns == nil {
		return nil, fmt.Errorf("finality provider does not exist")
	}

	return app.fpIns, nil
}

func (app *FinalityProviderApp) RegisterFinalityProvider(fpPkStr string) (*RegisterFinalityProviderResponse, error) {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(fpPkStr)
	if err != nil {
		return nil, err
	}

	fp, err := app.fps.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return nil, err
	}

	if fp.Status != proto.FinalityProviderStatus_CREATED {
		return nil, fmt.Errorf("finality-provider is already registered")
	}

	btcSig, err := bbntypes.NewBIP340Signature(fp.Pop.BtcSig)
	if err != nil {
		return nil, err
	}

	pop := &bstypes.ProofOfPossessionBTC{
		BtcSig:     btcSig.MustMarshal(),
		BtcSigType: bstypes.BTCSigType_BIP340,
	}

	fpAddr, err := sdk.AccAddressFromBech32(fp.FPAddr)
	if err != nil {
		return nil, err
	}

	request := &registerFinalityProviderRequest{
		fpAddr:          fpAddr,
		btcPubKey:       bbntypes.NewBIP340PubKeyFromBTCPK(fp.BtcPk),
		pop:             pop,
		description:     fp.Description,
		commission:      fp.Commission,
		errResponse:     make(chan error, 1),
		successResponse: make(chan *RegisterFinalityProviderResponse, 1),
	}

	app.registerFinalityProviderRequestChan <- request

	select {
	case err := <-request.errResponse:
		return nil, err
	case successResponse := <-request.successResponse:
		return successResponse, nil
	case <-app.quit:
		return nil, fmt.Errorf("finality-provider app is shutting down")
	}
}

// StartFinalityProvider starts a finality provider instance with the given EOTS public key
// Note: this should be called right after the finality-provider is registered
func (app *FinalityProviderApp) StartFinalityProvider(fpPk *bbntypes.BIP340PubKey, passphrase string) error {
	app.logger.Info("starting finality provider", zap.String("pk", fpPk.MarshalHex()))

	if err := app.startFinalityProviderInstance(fpPk, passphrase); err != nil {
		return err
	}

	app.logger.Info("finality provider is started", zap.String("pk", fpPk.MarshalHex()))

	return nil
}

// SyncFinalityProviderStatus syncs the status of the finality-providers with the chain.
func (app *FinalityProviderApp) SyncFinalityProviderStatus() (bool, error) {
	var fpInstanceRunning bool
	latestBlock, err := app.cc.QueryBestBlock()
	if err != nil {
		return false, err
	}

	fps, err := app.fps.GetAllStoredFinalityProviders()
	if err != nil {
		return false, err
	}

	for _, fp := range fps {
		vp, err := app.cc.QueryFinalityProviderVotingPower(fp.BtcPk, latestBlock.Height)
		if err != nil {
			continue
		}

		bip340PubKey := fp.GetBIP340BTCPK()
		if app.IsFinalityProviderRunning(bip340PubKey) {
			// there is a instance running, no need to keep syncing
			fpInstanceRunning = true
			// if it is already running, no need to update status
			continue
		}

		oldStatus := fp.Status
		newStatus, err := app.fps.UpdateFpStatusFromVotingPower(vp, fp)
		if err != nil {
			return false, err
		}

		if oldStatus != newStatus {
			app.logger.Info(
				"Update FP status",
				zap.String("fp_addr", fp.FPAddr),
				zap.String("old_status", oldStatus.String()),
				zap.String("new_status", newStatus.String()),
			)
			fp.Status = newStatus
		}

		if !fp.ShouldStart() {
			continue
		}

		if err := app.StartFinalityProvider(bip340PubKey, ""); err != nil {
			return false, err
		}
		fpInstanceRunning = true
	}

	return fpInstanceRunning, nil
}

// Start starts only the finality-provider daemon without any finality-provider instances
func (app *FinalityProviderApp) Start() error {
	var startErr error
	app.startOnce.Do(func() {
		app.logger.Info("Starting FinalityProviderApp")

		app.wg.Add(6)
		go app.syncChainFpStatusLoop()
		go app.eventLoop()
		go app.registrationLoop()
		go app.metricsUpdateLoop()
		go app.monitorCriticalErr()
		go app.monitorStatusUpdate()
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
				stopErr = err
				return
			}

			app.logger.Info("finality provider is stopped", zap.String("pk", pkHex))
		}

		app.logger.Debug("Stopping EOTS manager")
		if err := app.eotsManager.Close(); err != nil {
			stopErr = err
			return
		}

		app.logger.Debug("FinalityProviderApp successfully stopped")
	})
	return stopErr
}

func (app *FinalityProviderApp) CreateFinalityProvider(
	keyName, chainID, passPhrase, hdPath string,
	eotsPk *bbntypes.BIP340PubKey,
	description *stakingtypes.Description,
	commission *sdkmath.LegacyDec,
) (*CreateFinalityProviderResult, error) {
	req := &createFinalityProviderRequest{
		keyName:         keyName,
		chainID:         chainID,
		passPhrase:      passPhrase,
		hdPath:          hdPath,
		eotsPk:          eotsPk,
		description:     description,
		commission:      commission,
		errResponse:     make(chan error, 1),
		successResponse: make(chan *createFinalityProviderResponse, 1),
	}

	app.createFinalityProviderRequestChan <- req

	select {
	case err := <-req.errResponse:
		return nil, err
	case successResponse := <-req.successResponse:
		return &CreateFinalityProviderResult{
			FpInfo: successResponse.FpInfo,
		}, nil
	case <-app.quit:
		return nil, fmt.Errorf("finality-provider app is shutting down")
	}
}

// UnjailFinalityProvider sends a transaction to unjail a finality-provider
func (app *FinalityProviderApp) UnjailFinalityProvider(fpPk *bbntypes.BIP340PubKey) (string, error) {
	_, err := app.fps.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return "", fmt.Errorf("failed to get finality provider from db: %w", err)
	}

	// Send unjail transaction
	res, err := app.cc.UnjailFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return "", fmt.Errorf("failed to send unjail transaction: %w", err)
	}

	// Update finality-provider status in the local store
	// set it to INACTIVE for now and it will be updated to
	// ACTIVE if the fp has voting power
	err = app.fps.SetFpStatus(fpPk.MustToBTCPK(), proto.FinalityProviderStatus_INACTIVE)
	if err != nil {
		return "", fmt.Errorf("failed to update finality-provider status after unjailing: %w", err)
	}

	app.metrics.RecordFpStatus(fpPk.MarshalHex(), proto.FinalityProviderStatus_INACTIVE)

	app.logger.Info("successfully unjailed finality-provider",
		zap.String("btc_pk", fpPk.MarshalHex()),
		zap.String("txHash", res.TxHash),
	)

	return res.TxHash, nil
}

func (app *FinalityProviderApp) handleCreateFinalityProviderRequest(req *createFinalityProviderRequest) (*createFinalityProviderResponse, error) {
	// 1. check if the chain key exists
	kr, err := fpkr.NewChainKeyringControllerWithKeyring(app.kr, req.keyName, app.input)
	if err != nil {
		return nil, err
	}

	fpAddr, err := kr.Address(req.passPhrase)
	if err != nil {
		// the chain key does not exist, should create the chain key first
		return nil, fmt.Errorf("the keyname %s does not exist, add the key first: %w", req.keyName, err)
	}

	// 2. create proof-of-possession
	if req.eotsPk == nil {
		return nil, fmt.Errorf("eots pk cannot be nil")
	}
	pop, err := app.CreatePop(fpAddr, req.eotsPk, req.passPhrase)
	if err != nil {
		return nil, fmt.Errorf("failed to create proof-of-possession of the finality-provider: %w", err)
	}

	btcPk := req.eotsPk.MustToBTCPK()
	if err := app.fps.CreateFinalityProvider(fpAddr, btcPk, req.description, req.commission, req.keyName, req.chainID, pop.BtcSig); err != nil {
		return nil, fmt.Errorf("failed to save finality-provider: %w", err)
	}

	pkHex := req.eotsPk.MarshalHex()
	app.metrics.RecordFpStatus(pkHex, proto.FinalityProviderStatus_CREATED)

	app.logger.Info("successfully created a finality-provider",
		zap.String("eots_pk", pkHex),
		zap.String("addr", fpAddr.String()),
		zap.String("key_name", req.keyName),
	)

	storedFp, err := app.fps.GetFinalityProvider(btcPk)
	if err != nil {
		return nil, err
	}

	return &createFinalityProviderResponse{
		FpInfo: storedFp.ToFinalityProviderInfo(),
	}, nil
}

func (app *FinalityProviderApp) CreatePop(fpAddress sdk.AccAddress, fpPk *bbntypes.BIP340PubKey, passphrase string) (*bstypes.ProofOfPossessionBTC, error) {
	pop := &bstypes.ProofOfPossessionBTC{
		BtcSigType: bstypes.BTCSigType_BIP340, // by default, we use BIP-340 encoding for BTC signature
	}

	// generate pop.BtcSig = schnorr_sign(sk_BTC, hash(bbnAddress))
	// NOTE: *schnorr.Sign has to take the hash of the message.
	// So we have to hash the address before signing
	hash := tmhash.Sum(fpAddress.Bytes())

	sig, err := app.eotsManager.SignSchnorrSig(fpPk.MustMarshal(), hash, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get schnorr signature from the EOTS manager: %w", err)
	}

	pop.BtcSig = bbntypes.NewBIP340SignatureFromBTCSig(sig).MustMarshal()

	return pop, nil
}

// SignRawMsg loads the keyring private key and signs a message.
func (app *FinalityProviderApp) SignRawMsg(
	keyName, passPhrase, hdPath string,
	rawMsgToSign []byte,
) ([]byte, error) {
	_, chainSk, err := app.loadChainKeyring(keyName, passPhrase, hdPath)
	if err != nil {
		return nil, err
	}

	return chainSk.Sign(rawMsgToSign)
}

// loadChainKeyring checks the keyring by loading or creating a chain key.
func (app *FinalityProviderApp) loadChainKeyring(
	keyName, passPhrase, hdPath string,
) (*fpkr.ChainKeyringController, *secp256k1.PrivKey, error) {
	kr, err := fpkr.NewChainKeyringControllerWithKeyring(app.kr, keyName, app.input)
	if err != nil {
		return nil, nil, err
	}
	chainSk, err := kr.GetChainPrivKey(passPhrase)
	if err != nil {
		// the chain key does not exist, should create the chain key first
		keyInfo, err := kr.CreateChainKey(passPhrase, hdPath, "")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create chain key %s: %w", keyName, err)
		}
		chainSk = &secp256k1.PrivKey{Key: keyInfo.PrivateKey.Serialize()}
	}

	return kr, chainSk, nil
}

func (app *FinalityProviderApp) startFinalityProviderInstance(
	pk *bbntypes.BIP340PubKey,
	passphrase string,
) error {
	pkHex := pk.MarshalHex()
	if app.fpIns == nil {
		fpIns, err := NewFinalityProviderInstance(
			pk, app.config, app.fps, app.pubRandStore, app.cc, app.eotsManager,
			app.metrics, passphrase, app.criticalErrChan, app.logger,
		)
		if err != nil {
			return fmt.Errorf("failed to create finality provider instance %s: %w", pkHex, err)
		}

		app.fpIns = fpIns
	}

	return app.fpIns.Start()
}

func (app *FinalityProviderApp) IsFinalityProviderRunning(fpPk *bbntypes.BIP340PubKey) bool {
	if app.fpIns == nil {
		return false
	}

	if app.fpIns.GetBtcPkHex() != fpPk.MarshalHex() {
		return false
	}

	return app.fpIns.IsRunning()
}

func (app *FinalityProviderApp) removeFinalityProviderInstance() error {
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

func (app *FinalityProviderApp) setFinalityProviderJailed(fpi *FinalityProviderInstance) {
	fpi.MustSetStatus(proto.FinalityProviderStatus_JAILED)
	if err := app.removeFinalityProviderInstance(); err != nil {
		panic(fmt.Errorf("failed to terminate a jailed finality-provider %s: %w", fpi.GetBtcPkHex(), err))
	}
}

func (app *FinalityProviderApp) getLatestBlockWithRetry() (*types.BlockInfo, error) {
	var (
		latestBlock *types.BlockInfo
		err         error
	)

	if err := retry.Do(func() error {
		latestBlock, err = app.cc.QueryBestBlock()
		if err != nil {
			return err
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		app.logger.Debug(
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

// NOTE: this is not safe in production, so only used for testing purpose
func (app *FinalityProviderApp) getFpPrivKey(fpPk []byte) (*btcec.PrivateKey, error) {
	record, err := app.eotsManager.KeyRecord(fpPk, "")
	if err != nil {
		return nil, err
	}

	return record.PrivKey, nil
}
