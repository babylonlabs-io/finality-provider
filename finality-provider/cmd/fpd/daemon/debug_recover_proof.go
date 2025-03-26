package daemon

import (
	"fmt"
	"path/filepath"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/clientcontroller"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"

	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
)

func CommandDebugRecoverProof() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "debug-recover-rand-proof [fp-eots-pk-hex]",
		Aliases: []string{"drrp"},
		Short:   "Recover the public randomness' merkle proof for a finality provider",
		Long:    "Recover the public randomness' merkle proof for a finality provider. Currently only Babylon consumer chain is supported.",
		Example: `fpd recover-rand-proof --home /home/user/.fpd [fp-eots-pk-hex]`,
		Args:    cobra.ExactArgs(1),
		RunE:    fpcmd.RunEWithClientCtx(runCommandDebugRecoverProof),
	}
	cmd.Flags().Uint64("start-height", 1, "The block height from which the proof is recovered from (optional)")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	cmd.Flags().String(flags.FlagChainID, "", "The consumer identifier")

	return cmd
}

func runCommandDebugRecoverProof(ctx client.Context, cmd *cobra.Command, args []string) error {
	chainID, err := cmd.Flags().GetString(flags.FlagChainID)
	if err != nil {
		return fmt.Errorf("failed to read chain id flag: %w", err)
	}

	if chainID == "" {
		return fmt.Errorf("please specify chain-id")
	}

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}
	startHeight, err := cmd.Flags().GetUint64("start-height")
	if err != nil {
		return err
	}

	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	db, err := cfg.DatabaseConfig.GetDBBackend()
	defer func() {
		err := db.Close()
		if err != nil {
			panic(fmt.Errorf("failed to close db: %w", err))
		}
	}()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate public randomness store: %w", err)
	}

	// fpStore, err := store.NewFinalityProviderStore(db)
	// if err != nil {
	// 	return fmt.Errorf("failed to initiate finality provider store: %w", err)
	// }

	// em, err := eotsclient.NewEOTSManagerGRpcClient(cfg.EOTSManagerAddress, cfg.HMACKey)
	// if err != nil {
	// 	return fmt.Errorf("failed to create EOTS manager client: %w", err)
	// }

	bbncfg := fpcfg.BBNConfigToBabylonConfig(cfg.BabylonConfig)
	bc, err := bbnclient.New(
		&bbncfg,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create Babylon rpc client: %w", err)
	}
	bcc := clientcontroller.NewBabylonController(bc, cfg.BabylonConfig, &cfg.BTCNetParams, logger)

	eotsPk := fpPk.MustToBTCPK()
	commitList, err := bcc.QueryPublicRandCommitList(eotsPk, startHeight)
	if err != nil {
		return fmt.Errorf("failed to query randomness commit list for fp %s: %w", fpPk.MarshalHex(), err)
	}

	for _, commit := range commitList {

		logger.Info("commit on babylon",
			// zap.Uint64("NumPubRand", commit.NumPubRand),
			zap.Uint64("StartHeight", commit.StartHeight),
			zap.Binary("Commitment", commit.Commitment),
		)

		randProof, err := pubRandStore.GetPubRandProof([]byte(chainID), fpPk.MustMarshal(), commit.StartHeight)
		if err != nil {
			return fmt.Errorf("failed to GetPubRandProof rand list for fp %s: %w", fpPk.MarshalHex(), err)
		}

		logger.Info("randProof in db",
			zap.Binary("randProof", randProof),
		)
		// 	// to bypass gosec check of overflow risk
		// 	if commit.NumPubRand > uint64(math.MaxUint32) {
		// 		return fmt.Errorf("NumPubRand %d exceeds maximum uint32 value", commit.NumPubRand)
		// 	}

		// 	pubRandList, err := em.CreateRandomnessPairList(fpPk.MustMarshal(), []byte(chainID), commit.StartHeight, uint32(commit.NumPubRand))
		// 	if err != nil {
		// 		return fmt.Errorf("failed to get randomness from height %d to height %d: %w", commit.StartHeight, commit.EndHeight(), err)
		// 	}

		// 	// generate commitment and proof for each public randomness
		// 	commitRoot, proofList := types.GetPubRandCommitAndProofs(pubRandList)
		// 	if !bytes.Equal(commitRoot, commit.Commitment) {
		// 		return fmt.Errorf("the commit root on Babylon does not match the local one, expected: %x, got: %x", commit.Commitment, commitRoot)
		// 	}

		// 	// store them to database
		// 	if err := pubRandStore.AddPubRandProofList([]byte(chainID), fpPk.MustMarshal(), commit.StartHeight, commit.NumPubRand, proofList); err != nil {
		// 		return fmt.Errorf("failed to save public randomness to DB: %w", err)
		// 	}
	}

	// fpStore.
	latestCommitMap, err := bcc.QueryLastCommittedPublicRand(eotsPk, 3)
	if err != nil {
		return fmt.Errorf("failed to query randomness pub rand list for fp %s: %w", fpPk.MarshalHex(), err)
	}

	logger.Warn(
		"start iterate latestCommitMap",
		zap.Any("latestCommitMap", latestCommitMap),
	)
	logger.Warn("start iterate latestCommitMap")
	logger.Warn("start iterate latestCommitMap")
	logger.Warn("start iterate latestCommitMap")

	for startHeight, pubRand := range latestCommitMap {
		if startHeight > 595892 {
			continue
		}
		logger.Info(
			"height",
			zap.Uint64("startHeight", startHeight),
		)

		logger.Info("latestCommitMap in babylon",
			zap.Uint64("startHeight", startHeight),
			zap.Binary("Commitment", pubRand.Commitment),
			zap.Uint64("NumPubRand", pubRand.NumPubRand),
			zap.Uint64("EpochNum", pubRand.EpochNum),
		)

		randProof, err := pubRandStore.GetPubRandProof([]byte(chainID), fpPk.MustMarshal(), startHeight)
		if err != nil {
			return fmt.Errorf("failed to GetPubRandProof rand list for fp %s: %w", fpPk.MarshalHex(), err)
		}

		logger.Info("randProof in db",
			zap.Binary("randProof", randProof),
		)

		proofList, err := pubRandStore.GetPubRandProofList([]byte(chainID), fpPk.MustMarshal(), startHeight, 2)
		if err != nil {
			return fmt.Errorf("failed to GetPubRandProofList for fp %s: %w", fpPk.MarshalHex(), err)
		}

		for i, proof := range proofList {
			blockHeightVoted := startHeight + uint64(i)

			logger.Info("init prooflist range",
				zap.Uint64("blockHeightVoted", blockHeightVoted),
			)

			cmtProof := cmtcrypto.Proof{}
			if err := cmtProof.Unmarshal(proof); err != nil {
				return fmt.Errorf("failed to Unmarshal for fp %s: %w", fpPk.MarshalHex(), err)
			}

			blockInfo, err := bcc.QueryBlock(blockHeightVoted)
			if err != nil {
				return fmt.Errorf("failed to QueryBlock for fp %s: %w", fpPk.MarshalHex(), err)
			}
			// pubRand *btcec.FieldVal,

			pubRandSchnorr := bbntypes.SchnorrPubRand(pubRand.Commitment[:])
			err = finalitytypes.VerifyFinalitySig(&finalitytypes.MsgAddFinalitySig{
				FpBtcPk:      fpPk,
				BlockHeight:  blockHeightVoted,
				PubRand:      &pubRandSchnorr,
				FinalitySig:  nil,
				BlockAppHash: blockInfo.Hash,
				Proof:        &cmtProof,
			}, &finalitytypes.PubRandCommit{
				StartHeight: startHeight,
				NumPubRand:  pubRand.NumPubRand,
				Commitment:  pubRand.Commitment,
				EpochNum:    pubRand.EpochNum,
			})

			logger.Info("trying to verify finality sig",
				zap.Uint64("blockHeightVoted", blockHeightVoted),
				zap.Binary("blockHash", blockInfo.Hash),
				zap.Error(err),
			)
		}
	}

	return nil
}
