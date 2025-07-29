package e2etest_bcd

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	common "github.com/babylonlabs-io/finality-provider/itest"
)

const (
	bcdRpcPort    int = 3990
	bcdP2pPort    int = 3991
	bcdChainID        = "bcd-test"
	bcdConsumerID     = "09-localhost" // TODO: mock a real consumer ID
)

type BcdNodeHandler struct {
	cmd     *exec.Cmd
	pidFile string
	dataDir string
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

	setupBcd(t, testDir)
	cmd := bcdStartCmd(t, testDir)
	fmt.Println("Starting bcd with command:", cmd.String())
	return &BcdNodeHandler{
		cmd:     cmd,
		pidFile: "", // empty for now, will be set after start
		dataDir: testDir,
	}
}

func (w *BcdNodeHandler) Start() error {
	if err := w.start(); err != nil {
		// try to cleanup after start error, but return original error
		_ = w.cleanup()
		return err
	}
	return nil
}

func (w *BcdNodeHandler) Stop(t *testing.T) {
	err := w.stop()
	// require.NoError(t, err)
	// TODO: investigate why stopping the bcd process is failing
	if err != nil {
		log.Printf("error stopping bcd process: %v", err)
	}

	err = w.cleanup()
	require.NoError(t, err)
}

func (w *BcdNodeHandler) GetRpcUrl() string {
	return fmt.Sprintf("http://localhost:%d", bcdRpcPort)
}

func (w *BcdNodeHandler) GetHomeDir() string {
	return w.dataDir
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
		// return if not properly initialized
		// or error starting the process
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

	dirs := []string{
		w.dataDir,
	}
	var err error
	for _, dir := range dirs {
		if err = os.RemoveAll(dir); err != nil {
			log.Printf("Cannot remove dir %s: %v", dir, err)
		}
	}
	return nil
}

func bcdInit(homeDir string) error {
	_, err := common.RunCommand("bcd", "init", "--home", homeDir, "--chain-id", bcdChainID, common.WasmMoniker)
	return err
}

func bcdUpdateGenesisFile(homeDir string) error {
	genesisPath := filepath.Join(homeDir, "config", "genesis.json")
	fmt.Println("Home directory path:", homeDir)

	// Update "stake" placeholder
	sedCmd1 := fmt.Sprintf("sed -i. 's/\"stake\"/\"%s\"/' %s", common.WasmStake, genesisPath)
	fmt.Println("Executing command:", sedCmd1)
	_, err := common.RunCommand("sh", "-c", sedCmd1)
	if err != nil {
		return fmt.Errorf("failed to update stake in genesis.json: %w", err)
	}

	// Add: Set voting period to 30 seconds for E2E tests
	//sedCmd2 := fmt.Sprintf("sed -i. 's/\"voting_period\": \".*\"/\"voting_period\": \"60s\"/' %s", genesisPath)
	//_, err = common.RunCommand("sh", "-c", sedCmd2)
	//if err != nil {
	//	return fmt.Errorf("failed to update voting period in genesis.json: %w", err)
	//}

	// Read and print the updated genesis.json to verify changes
	content, err := os.ReadFile(genesisPath)
	if err != nil {
		return fmt.Errorf("failed to read updated genesis.json: %w", err)
	}

	fmt.Println("Updated genesis.json content:")
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.Contains(line, "babylon_contract_address") || strings.Contains(line, "btc_staking_contract_address") {
			fmt.Println(line)
		}
	}

	return nil
}

func bcdKeysAdd(homeDir string) error {
	_, err := common.RunCommand("bcd", "keys", "add", "validator", "--home", homeDir, "--keyring-backend=test")
	return err
}

func bcdAddValidatorGenesisAccount(homeDir string) error {
	_, err := common.RunCommand("bcd", "genesis", "add-genesis-account", "validator", fmt.Sprintf("1000000000000%s,1000000000000%s", common.WasmStake, common.WasmFee), "--home", homeDir, "--keyring-backend=test")
	return err
}

func bcdVersion() error {
	cmd := exec.Command("bcd", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	fmt.Printf("bcd version:\n%s\n", string(output))
	return nil
}

func bcdGentxValidator(homeDir string) error {
	_, err := common.RunCommand("bcd", "genesis", "gentx", "validator", fmt.Sprintf("250000000%s", common.WasmStake), "--chain-id="+bcdChainID, "--amount="+fmt.Sprintf("250000000%s", common.WasmStake), "--home", homeDir, "--keyring-backend=test")
	return err
}

func bcdCollectGentxs(homeDir string) error {
	_, err := common.RunCommand("bcd", "genesis", "collect-gentxs", "--home", homeDir)
	return err
}

func setupBcd(t *testing.T, testDir string) {
	err := bcdInit(testDir)
	require.NoError(t, err)

	err = bcdUpdateGenesisFile(testDir)
	require.NoError(t, err)

	err = bcdKeysAdd(testDir)
	require.NoError(t, err)

	err = bcdAddValidatorGenesisAccount(testDir)
	require.NoError(t, err)

	err = bcdGentxValidator(testDir)
	require.NoError(t, err)

	err = bcdCollectGentxs(testDir)
	require.NoError(t, err)

	err = bcdVersion()
	require.NoError(t, err)
}

func bcdStartCmd(t *testing.T, testDir string) *exec.Cmd {
	args := []string{
		"start",
		"--home", testDir,
		"--rpc.laddr", fmt.Sprintf("tcp://0.0.0.0:%d", bcdRpcPort),
		"--p2p.laddr", fmt.Sprintf("tcp://0.0.0.0:%d", bcdP2pPort),
		// "--log_level=trace",
		// "--trace",
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
