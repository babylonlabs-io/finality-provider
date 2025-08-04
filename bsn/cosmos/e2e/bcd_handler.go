//go:build e2e_bcd

package e2etest_bcd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	common "github.com/babylonlabs-io/finality-provider/itest"
)

const (
	bcdRpcPort    int = 3990
	bcdP2pPort    int = 3991
	bcdChainID        = "bcd-test"
	bcdConsumerID     = "07-tendermint-0"

	// Relayer constants
	relayerAPIPort int = 5183
	babylonKey         = "babylon-key"
	consumerKey        = "bcd-key"
	pathName           = "bcd"
)

type BcdNodeHandler struct {
	cmd             *exec.Cmd
	relayerCmd      *exec.Cmd
	pidFile         string
	relayerPidFile  string
	dataDir         string
	relayerHomeDir  string
	contractAddress string

	babylonChainID string
	babylonNodeRPC string
	babylonHome    string
}

type RelayerConfig struct {
	Global RelayerGlobalConfig           `yaml:"global"`
	Chains map[string]RelayerChainConfig `yaml:"chains"`
	Paths  map[string]RelayerPathConfig  `yaml:"paths"`
}

type RelayerGlobalConfig struct {
	APIListenAddr  string `yaml:"api-listen-addr"`
	MaxRetries     int    `yaml:"max-retries"`
	Timeout        string `yaml:"timeout"`
	Memo           string `yaml:"memo"`
	LightCacheSize int    `yaml:"light-cache-size"`
}

type RelayerChainConfig struct {
	Type  string                  `yaml:"type"`
	Value RelayerChainValueConfig `yaml:"value"`
}

type RelayerChainValueConfig struct {
	Key            string   `yaml:"key"`
	ChainID        string   `yaml:"chain-id"`
	RPCAddr        string   `yaml:"rpc-addr"`
	AccountPrefix  string   `yaml:"account-prefix"`
	KeyringBackend string   `yaml:"keyring-backend"`
	GasAdjustment  float64  `yaml:"gas-adjustment"`
	GasPrices      string   `yaml:"gas-prices"`
	MinGasAmount   int      `yaml:"min-gas-amount"`
	Debug          bool     `yaml:"debug"`
	Timeout        string   `yaml:"timeout"`
	OutputFormat   string   `yaml:"output-format"`
	SignMode       string   `yaml:"sign-mode"`
	ExtraCodecs    []string `yaml:"extra-codecs"`
}

type RelayerPathConfig struct {
	Src RelayerEndpointConfig `yaml:"src"`
	Dst RelayerEndpointConfig `yaml:"dst"`
}

type RelayerEndpointConfig struct {
	ChainID string `yaml:"chain-id"`
}

func NewBcdNodeHandler(t *testing.T) *BcdNodeHandler {
	testDir, err := common.BaseDir("ZBcdTest")
	require.NoError(t, err)
	defer func() {
		if err != nil {
			err := os.RemoveAll(testDir)
			require.NoError(t, err)
		}
	}()

	relayerDir, err := common.BaseDir("ZRelayerTest")
	require.NoError(t, err)

	setupBcd(t, testDir)
	cmd := bcdStartCmd(t, testDir)
	t.Log("Starting bcd with command:", cmd.String())
	t.Log("Test directory:", testDir)
	t.Log("Relayer directory:", relayerDir)

	return &BcdNodeHandler{
		cmd:            cmd,
		pidFile:        "",
		relayerPidFile: "",
		dataDir:        testDir,
		relayerHomeDir: relayerDir,
		// These should be set by the caller based on their setup
		babylonChainID: "chain-test",
		babylonNodeRPC: "http://localhost:26657", // Default, should be configured
		babylonHome:    "",                       // Should be set by caller
	}
}

func (w *BcdNodeHandler) SetBabylonConfig(chainID, nodeRPC, homeDir string) {
	w.babylonChainID = chainID
	w.babylonNodeRPC = nodeRPC
	w.babylonHome = homeDir
}

func (w *BcdNodeHandler) SetContractAddress(address string) {
	w.contractAddress = address
}

func (w *BcdNodeHandler) Start() error {
	if err := w.start(); err != nil {
		// try to cleanup after start error, but return original error
		_ = w.cleanup()
		return err
	}
	return nil
}

func (w *BcdNodeHandler) StartRelayer(t *testing.T) error {
	// Query contract address if not set
	if w.contractAddress == "" {
		contractAddr, err := w.queryContractAddress()
		if err != nil {
			return fmt.Errorf("failed to query contract address: %w", err)
		}
		w.contractAddress = contractAddr
	}

	// Setup relayer
	if err := w.setupRelayer(t); err != nil {
		return fmt.Errorf("failed to setup relayer: %w", err)
	}

	if err := w.createClients(t); err != nil {
		return fmt.Errorf("failed to create IBC clients: %w", err)
	}

	// Wait for clients to be established
	time.Sleep(10 * time.Second)

	// Step 2: Create IBC connection
	if err := w.createConnection(t); err != nil {
		return fmt.Errorf("failed to create IBC connection: %w", err)
	}

	// Wait for connection to be established
	time.Sleep(5 * time.Second)

	// Create IBC channels
	if err := w.createTransferChannel(t); err != nil {
		return fmt.Errorf("failed to create IBC channels: %w", err)
	}

	// Start relayer
	if err := w.startRelayer(t); err != nil {
		return fmt.Errorf("failed to start relayer: %w", err)
	}

	return nil
}

func (w *BcdNodeHandler) Stop(t *testing.T) {
	// Stop relayer first
	if w.relayerCmd != nil && w.relayerCmd.Process != nil {
		err := w.stopRelayer()
		if err != nil {
			log.Printf("error stopping relayer process: %v", err)
		}
	}

	// Stop BCD node
	err := w.stop()
	if err != nil {
		log.Printf("error stopping bcd process: %v", err)
	}

	err = w.cleanup()
	require.NoError(t, err)
}

func (w *BcdNodeHandler) queryContractAddress() (string, error) {
	cmd := exec.Command("bcd", "query", "wasm", "list-contract-by-code", "1", "--home", w.dataDir, "--output", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to query contract address: %w, output: %s", err, string(output))
	}

	// Parse the JSON output to extract the contract address
	var result struct {
		Contracts []string `json:"contracts"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse contract query result: %w", err)
	}

	if len(result.Contracts) == 0 {
		return "", fmt.Errorf("no contracts found")
	}

	return result.Contracts[0], nil
}

func (w *BcdNodeHandler) setupRelayer(t *testing.T) error {
	// Initialize relayer config
	if err := w.initRelayerConfig(); err != nil {
		return fmt.Errorf("failed to init relayer config: %w", err)
	}

	// Create relayer configuration file
	if err := w.createRelayerConfig(t); err != nil {
		return fmt.Errorf("failed to create relayer config: %w", err)
	}

	// Restore keys
	if err := w.restoreRelayerKeys(t); err != nil {
		return fmt.Errorf("failed to restore relayer keys: %w", err)
	}

	return nil
}

func (w *BcdNodeHandler) initRelayerConfig() error {
	cmd := exec.Command("relayer", "--home", w.relayerHomeDir, "config", "init")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to init relayer config: %w, output: %s", err, string(output))
	}
	return nil
}

func (w *BcdNodeHandler) createRelayerConfig(t *testing.T) error {
	configContent := fmt.Sprintf(`global:
    api-listen-addr: :%d
    max-retries: 20
    timeout: 30s
    memo: ""
    light-cache-size: 10
chains:
    babylon:
        type: cosmos
        value:
            key: %s
            chain-id: %s
            rpc-addr: %s
            account-prefix: bbn
            keyring-backend: test
            gas-adjustment: 1.5
            gas-prices: 0.002ubbn
            min-gas-amount: 1
            debug: true
            timeout: 30s
            output-format: json
            sign-mode: direct
            extra-codecs: []
    bcd:
        type: cosmos
        value:
            key: %s
            chain-id: %s
            rpc-addr: http://localhost:%d
            account-prefix: bbnc
            keyring-backend: test
            gas-adjustment: 1.5
            gas-prices: 0.002ustake
            min-gas-amount: 1
            debug: true
            timeout: 30s
            output-format: json
            sign-mode: direct
            extra-codecs: []     
paths:
    %s:
        src:
            chain-id: %s
        dst:
            chain-id: %s
`, relayerAPIPort, babylonKey, w.babylonChainID, w.babylonNodeRPC,
		consumerKey, bcdChainID, bcdRpcPort,
		pathName, w.babylonChainID, bcdChainID)

	configPath := filepath.Join(w.relayerHomeDir, "config", "config.yaml")

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write relayer config: %w", err)
	}

	t.Logf("Created relayer config at: %s\n", configPath)

	return nil
}

func (w *BcdNodeHandler) restoreRelayerKeys(t *testing.T) error {
	// Restore consumer key
	consumerKeyPath := filepath.Join(w.dataDir, "key_seed.json")
	if _, err := os.Stat(consumerKeyPath); err == nil {
		consumerMemo, err := w.extractMnemonic(consumerKeyPath, "mnemonic")
		if err != nil {
			return fmt.Errorf("failed to extract consumer mnemonic: %w", err)
		}

		if err := w.restoreKey("bcd", consumerKey, consumerMemo); err != nil {
			return fmt.Errorf("failed to restore consumer key: %w", err)
		}
		t.Log("Restored consumer key successfully")
	} else {
		return fmt.Errorf("consumer key seed file not found: %s", consumerKeyPath)
	}

	// Restore Babylon key
	if w.babylonHome != "" {
		babylonKeyPath := filepath.Join(w.babylonHome, "key_seed.json")
		if _, err := os.Stat(babylonKeyPath); err == nil {
			babylonMemo, err := w.extractMnemonic(babylonKeyPath, "secret")
			if err != nil {
				return fmt.Errorf("failed to extract babylon mnemonic: %w", err)
			}

			if err := w.restoreKey("babylon", babylonKey, babylonMemo); err != nil {
				return fmt.Errorf("failed to restore babylon key: %w", err)
			}
			t.Log("Restored babylon key successfully")
		} else {
			return fmt.Errorf("babylon key seed file not found: %s", babylonKeyPath)
		}
	}

	return nil
}

func (w *BcdNodeHandler) extractMnemonic(keyPath, field string) (string, error) {
	content, err := os.ReadFile(keyPath)
	if err != nil {
		return "", err
	}

	var keyData map[string]interface{}
	if err := json.Unmarshal(content, &keyData); err != nil {
		return "", err
	}

	mnemonic, ok := keyData[field].(string)
	if !ok {
		return "", fmt.Errorf("field %s not found or not a string", field)
	}

	return mnemonic, nil
}

func (w *BcdNodeHandler) restoreKey(chainName, keyName, mnemonic string) error {
	cmd := exec.Command("relayer", "--home", w.relayerHomeDir, "keys", "restore", chainName, keyName, mnemonic)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore key: %w, output: %s", err, string(output))
	}
	return nil
}

func (w *BcdNodeHandler) createTransferChannel(t *testing.T) error {
	cmd := exec.Command("relayer", "--home", w.relayerHomeDir, "query", "connections", pathName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to query connections: %w, output: %s", err, string(output))
	}
	t.Logf("Available connections: %s", string(output))

	t.Log("Creating IBC channel for IBC transfer")
	if err := w.createChannel(t, "transfer", "transfer", "unordered", "ics20-1"); err != nil {
		return fmt.Errorf("failed to create transfer channel: %w", err)
	}
	t.Log("Created IBC transfer channel successfully!")

	// Wait for channels to be established
	time.Sleep(10 * time.Second)

	return nil
}

func (w *BcdNodeHandler) createZoneConciergeChannel(t *testing.T) error {
	contractPort := fmt.Sprintf("wasm.%s", w.contractAddress)
	t.Log("Creating IBC channel for zoneconcierge")
	if err := w.createChannel(t, "zoneconcierge", contractPort, "ordered", "zoneconcierge-1"); err != nil {
		return fmt.Errorf("failed to create zoneconcierge channel: %w", err)
	}
	t.Log("Created zoneconcierge IBC channel successfully!")

	return nil
}

func (w *BcdNodeHandler) createChannel(t *testing.T, srcPort, dstPort, order, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "relayer", "--home", w.relayerHomeDir, "tx", "channel", pathName,
		"--src-port", srcPort, "--dst-port", dstPort,
		"--order", order, "--version", version,
		"--timeout", "59s", "--max-retries", "5",
		"--debug")

	t.Logf("Running command: %s\n", cmd.String())

	output, err := cmd.CombinedOutput()

	// Check if operation was actually successful even if command timed out
	if err != nil {
		outputStr := string(output)

		// Look for success indicators in the output
		successIndicators := []string{
			"Found termination condition for connection handshake",
			"Found termination condition for channel handshake",
			"Successful transaction",
			"connection_open_confirm",
			"channel_open_confirm",
			"Clients created",
			"Connection created",
			"Channel created",
		}

		isSuccessful := false
		for _, indicator := range successIndicators {
			if strings.Contains(outputStr, indicator) {
				isSuccessful = true
				break
			}
		}

		if isSuccessful {
			t.Logf("Operation completed successfully (despite timeout): %s\n", outputStr)

			return nil
		}

		return fmt.Errorf("failed to create channel: %w, output: %s", err, outputStr)
	}

	t.Logf("Channel creation output: %s\n", string(output))

	return nil
}

func (w *BcdNodeHandler) createClients(t *testing.T) error {
	t.Log("Creating IBC clients...")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "relayer", "--home", w.relayerHomeDir, "tx", "clients", pathName, "--debug")
	output, err := cmd.CombinedOutput()

	// Check for successful client creation even if command timed out
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "Clients created") ||
			strings.Contains(outputStr, "Successful transaction") ||
			strings.Contains(outputStr, "client_created") {
			t.Logf("Clients created successfully (despite timeout): %s\n", outputStr)
			return nil
		}
		return fmt.Errorf("failed to create clients: %w, output: %s", err, outputStr)
	}

	t.Logf("Clients created: %s", string(output))

	return nil
}

func (w *BcdNodeHandler) createConnection(t *testing.T) error {
	t.Log("Creating IBC connection...")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "relayer", "--home", w.relayerHomeDir, "tx", "connection", pathName, "--debug")
	output, err := cmd.CombinedOutput()

	// Check for successful connection creation even if command timed out
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "Found termination condition for connection handshake") ||
			strings.Contains(outputStr, "Successful transaction") ||
			strings.Contains(outputStr, "connection_open_confirm") {
			t.Logf("Connection created successfully (despite timeout): %s\n", outputStr)
			return nil
		}
		return fmt.Errorf("failed to create connection: %w, output: %s", err, outputStr)
	}

	t.Logf("Connection created: %s\n", string(output))

	return nil
}

func (w *BcdNodeHandler) startRelayer(t *testing.T) error {
	t.Log("Starting the IBC relayer")

	args := []string{
		"--home", w.relayerHomeDir,
		"start", pathName,
		"--debug-addr", "",
		"--flush-interval", "10s",
	}

	// Create log file for relayer
	logFile, err := os.Create(filepath.Join(w.relayerHomeDir, "relayer.log"))
	if err != nil {
		return fmt.Errorf("failed to create relayer log file: %w", err)
	}

	// Create multi-writer for both file and console
	mw := io.MultiWriter(os.Stdout, logFile)

	w.relayerCmd = exec.Command("relayer", args...)
	w.relayerCmd.Stdout = mw
	w.relayerCmd.Stderr = mw

	if err := w.relayerCmd.Start(); err != nil {
		return fmt.Errorf("failed to start relayer: %w", err)
	}

	// Create PID file for relayer
	relayerPid, err := os.Create(filepath.Join(w.relayerHomeDir, "relayer.pid"))
	if err != nil {
		return fmt.Errorf("failed to create relayer PID file: %w", err)
	}

	w.relayerPidFile = relayerPid.Name()
	if _, err = fmt.Fprintf(relayerPid, "%d\n", w.relayerCmd.Process.Pid); err != nil {
		return fmt.Errorf("failed to write relayer PID: %w", err)
	}

	if err := relayerPid.Close(); err != nil {
		return fmt.Errorf("failed to close relayer PID file: %w", err)
	}

	t.Logf("Started relayer with PID: %d\n", w.relayerCmd.Process.Pid)

	return nil
}

func (w *BcdNodeHandler) stopRelayer() error {
	if w.relayerCmd == nil || w.relayerCmd.Process == nil {
		return nil
	}

	defer func() {
		err := w.relayerCmd.Wait()
		fmt.Printf("error waiting for relayer command: %v\n", err)
	}()

	if runtime.GOOS == "windows" {
		return w.relayerCmd.Process.Signal(os.Kill)
	}
	return w.relayerCmd.Process.Signal(os.Interrupt)
}

func (w *BcdNodeHandler) GetRpcUrl() string {
	return fmt.Sprintf("http://localhost:%d", bcdRpcPort)
}

func (w *BcdNodeHandler) GetHomeDir() string {
	return w.dataDir
}

func (w *BcdNodeHandler) GetRelayerHomeDir() string {
	return w.relayerHomeDir
}

func (w *BcdNodeHandler) GetContractAddress() string {
	return w.contractAddress
}

func (w *BcdNodeHandler) start() error {
	if err := w.cmd.Start(); err != nil {
		return err
	}

	pid, err := os.Create(filepath.Join(w.dataDir, fmt.Sprintf("%s.pid", "bcd")))
	if err != nil {
		return err
	}

	w.pidFile = pid.Name()
	if _, err = fmt.Fprintf(pid, "%d\n", w.cmd.Process.Pid); err != nil {
		return err
	}

	if err := pid.Close(); err != nil {
		return err
	}

	return nil
}

func (w *BcdNodeHandler) stop() (err error) {
	if w.cmd == nil || w.cmd.Process == nil {
		return nil
	}

	defer func() {
		err = w.cmd.Wait()
	}()

	if runtime.GOOS == "windows" {
		return w.cmd.Process.Signal(os.Kill)
	}
	return w.cmd.Process.Signal(os.Interrupt)
}

func (w *BcdNodeHandler) cleanup() error {
	if w.pidFile != "" {
		if err := os.Remove(w.pidFile); err != nil {
			log.Printf("unable to remove file %s: %v", w.pidFile, err)
		}
	}

	if w.relayerPidFile != "" {
		if err := os.Remove(w.relayerPidFile); err != nil {
			log.Printf("unable to remove relayer PID file %s: %v", w.relayerPidFile, err)
		}
	}

	dirs := []string{
		w.dataDir,
		w.relayerHomeDir,
	}
	var err error
	for _, dir := range dirs {
		if err = os.RemoveAll(dir); err != nil {
			log.Printf("Cannot remove dir %s: %v", dir, err)
		}
	}
	return nil
}

// Keep all the existing functions (bcdInit, bcdUpdateGenesisFile, etc.)
// ... (all your existing setup functions remain the same)

func bcdInit(homeDir string) error {
	_, err := common.RunCommand("bcd", "init", "--home", homeDir, "--chain-id", bcdChainID, common.WasmMoniker)
	return err
}

func bcdUpdateGenesisFile(t *testing.T, homeDir string) error {
	genesisPath := filepath.Join(homeDir, "config", "genesis.json")
	t.Log("Home directory path:", homeDir)

	// Update "stake" placeholder (keep your existing sed)
	sedCmd1 := fmt.Sprintf("sed -i. 's/\"stake\"/\"%s\"/' %s", common.WasmStake, genesisPath)
	t.Log("Executing command:", sedCmd1)
	_, err := common.RunCommand("sh", "-c", sedCmd1)
	if err != nil {
		return fmt.Errorf("failed to update stake in genesis.json: %w", err)
	}

	jqCmd := fmt.Sprintf("jq '.app_state.gov.params.voting_period = \"30s\" | .app_state.gov.params.max_deposit_period = \"10s\" | .app_state.gov.params.expedited_voting_period = \"15s\"' %s > %s.tmp && mv %s.tmp %s",
		genesisPath, genesisPath, genesisPath, genesisPath)
	t.Log("Executing jq command:", jqCmd)
	_, err = common.RunCommand("sh", "-c", jqCmd)
	if err != nil {
		return fmt.Errorf("failed to update governance periods: %w", err)
	}

	// Your existing verification code...
	content, err := os.ReadFile(genesisPath)
	if err != nil {
		return fmt.Errorf("failed to read updated genesis.json: %w", err)
	}

	t.Log("Updated genesis.json")
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.Contains(line, "voting_period") || strings.Contains(line, "max_deposit_period") {
			t.Log(line)
		}
	}

	return nil
}

func bcdKeysAdd(homeDir string) error {
	keySeedPath := filepath.Join(homeDir, "key_seed.json")

	// Create the key and capture JSON output
	cmd := exec.Command("bcd", "keys", "add", "validator", "--home", homeDir, "--keyring-backend=test", "--output", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add key: %w, output: %s", err, string(output))
	}

	// Write the JSON output directly to key_seed.json
	if err := os.WriteFile(keySeedPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write key_seed.json: %w", err)
	}

	return nil
}

func bcdAddValidatorGenesisAccount(homeDir string) error {
	_, err := common.RunCommand("bcd", "genesis", "add-genesis-account", "validator", fmt.Sprintf("1000000000000%s,1000000000000%s", common.WasmStake, common.WasmFee), "--home", homeDir, "--keyring-backend=test")
	return err
}

func bcdVersion(t *testing.T) error {
	cmd := exec.Command("bcd", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	t.Logf("bcd version:\n%s\n", string(output))

	return nil
}

func bcdGentxValidator(t *testing.T, homeDir string) error {
	cmd := exec.Command("bcd", "genesis", "gentx", "validator",
		fmt.Sprintf("250000000%s", common.WasmStake),
		"--chain-id="+bcdChainID,
		"--amount="+fmt.Sprintf("250000000%s", common.WasmStake),
		"--home", homeDir,
		"--keyring-backend=test")

	t.Logf("Running gentx command: %s\n", cmd.String())
	output, err := cmd.CombinedOutput()
	t.Logf("Gentx output: %s\n", string(output))

	if err != nil {
		return fmt.Errorf("gentx failed: %w", err)
	}
	return nil
}

func bcdCollectGentxs(homeDir string) error {
	_, err := common.RunCommand("bcd", "genesis", "collect-gentxs", "--home", homeDir)
	return err
}

func setupBcd(t *testing.T, testDir string) {
	err := bcdInit(testDir)
	require.NoError(t, err)

	err = bcdUpdateGenesisFile(t, testDir)
	require.NoError(t, err)

	err = bcdKeysAdd(testDir)
	require.NoError(t, err)

	err = bcdAddValidatorGenesisAccount(testDir)
	require.NoError(t, err)

	err = bcdGentxValidator(t, testDir)
	require.NoError(t, err)

	err = bcdCollectGentxs(testDir)
	require.NoError(t, err)

	err = bcdVersion(t)
	require.NoError(t, err)
}

func bcdStartCmd(t *testing.T, testDir string) *exec.Cmd {
	args := []string{
		"start",
		"--home", testDir,
		"--rpc.laddr", fmt.Sprintf("tcp://0.0.0.0:%d", bcdRpcPort),
		"--p2p.laddr", fmt.Sprintf("tcp://0.0.0.0:%d", bcdP2pPort),
		"--log_level=debug",
	}

	f, err := os.Create(filepath.Join(testDir, "bcd.log"))
	require.NoError(t, err)

	// Create a multi-writer to write to both the log file and the console
	mw := io.MultiWriter(os.Stdout, f)

	cmd := exec.Command("bcd", args...)
	cmd.Stdout = mw
	cmd.Stderr = mw

	return cmd
}
