package e2e_utils

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

type babylonNode struct {
	cmd        *exec.Cmd
	pidFile    string
	dataDir    string
	walletName string
}

func newBabylonNode(dataDir, walletName string, cmd *exec.Cmd) *babylonNode {
	return &babylonNode{
		dataDir:    dataDir,
		cmd:        cmd,
		walletName: walletName,
	}
}

func (n *babylonNode) start() error {
	if err := n.cmd.Start(); err != nil {
		return err
	}

	pid, err := os.Create(filepath.Join(n.dataDir,
		fmt.Sprintf("%s.pid", "config")))
	if err != nil {
		return err
	}

	n.pidFile = pid.Name()
	if _, err = fmt.Fprintf(pid, "%d\n", n.cmd.Process.Pid); err != nil {
		return err
	}

	if err := pid.Close(); err != nil {
		return err
	}

	return nil
}

func (n *babylonNode) stop() (err error) {
	if n.cmd == nil || n.cmd.Process == nil {
		// return if not properly initialized
		// or error starting the process
		return nil
	}

	defer func() {
		err = n.cmd.Wait()
	}()

	if runtime.GOOS == "windows" {
		return n.cmd.Process.Signal(os.Kill)
	}
	return n.cmd.Process.Signal(os.Interrupt)
}

func (n *babylonNode) cleanup() error {
	if n.pidFile != "" {
		if err := os.Remove(n.pidFile); err != nil {
			log.Printf("unable to remove file %s: %v", n.pidFile,
				err)
		}
	}

	dirs := []string{
		n.dataDir,
	}
	var err error
	for _, dir := range dirs {
		if err = os.RemoveAll(dir); err != nil {
			log.Printf("Cannot remove dir %s: %v", dir, err)
		}
	}
	return nil
}

func (n *babylonNode) shutdown() error {
	if err := n.stop(); err != nil {
		return err
	}
	if err := n.cleanup(); err != nil {
		return err
	}
	return nil
}

type BabylonNodeHandler struct {
	BabylonNode *babylonNode
}

func NewBabylonNodeHandler(t *testing.T, covenantQuorum int, covenantPks []*types.BIP340PubKey) *BabylonNodeHandler {
	testDir, err := BaseDir("zBabylonTest")
	require.NoError(t, err)
	defer func() {
		if err != nil {
			err := os.RemoveAll(testDir)
			require.NoError(t, err)
		}
	}()

	walletName := "node0"
	nodeDataDir := filepath.Join(testDir, walletName, "babylond")

	r := rand.New(rand.NewSource(time.Now().Unix()))
	slashingAddress, err := datagen.GenRandomBTCAddress(r, &chaincfg.SigNetParams)
	require.NoError(t, err)
	slashingPkScript, err := txscript.PayToAddrScript(slashingAddress)
	require.NoError(t, err)

	var covenantPksStr []string
	for _, pk := range covenantPks {
		covenantPksStr = append(covenantPksStr, pk.MarshalHex())
	}

	initTestnetCmd := exec.Command(
		"babylond",
		"testnet",
		"--v=1",
		fmt.Sprintf("--output-dir=%s", testDir),
		"--starting-ip-address=192.168.10.2",
		"--keyring-backend=test",
		"--chain-id=chain-test",
		"--additional-sender-account",
		fmt.Sprintf("--epoch-interval=%d", 5),
		fmt.Sprintf("--slashing-pk-script=%s", hex.EncodeToString(slashingPkScript)),
		fmt.Sprintf("--covenant-quorum=%d", covenantQuorum),
		fmt.Sprintf("--covenant-pks=%s", strings.Join(covenantPksStr, ",")),
	)

	var stderr bytes.Buffer
	initTestnetCmd.Stderr = &stderr

	err = initTestnetCmd.Run()
	if err != nil {
		fmt.Printf("init testnet failed: %s \n", stderr.String())
	}
	require.NoError(t, err)

	f, err := os.Create(filepath.Join(testDir, "babylon.log"))
	t.Logf("babylon log file: %s", f.Name())
	require.NoError(t, err)

	startCmd := exec.Command(
		"babylond",
		"start",
		fmt.Sprintf("--home=%s", nodeDataDir),
		"--log_level=trace",
		"--trace",
	)

	fmt.Println("Starting babylond with command: ", startCmd.String())

	startCmd.Stdout = f

	return &BabylonNodeHandler{
		BabylonNode: newBabylonNode(testDir, walletName, startCmd),
	}
}

func (w *BabylonNodeHandler) Start() error {
	if err := w.BabylonNode.start(); err != nil {
		// try to cleanup after start error, but return original error
		_ = w.BabylonNode.cleanup()
		return err
	}
	return nil
}

func (w *BabylonNodeHandler) Stop() error {
	if err := w.BabylonNode.shutdown(); err != nil {
		return err
	}

	return nil
}

func (w *BabylonNodeHandler) GetNodeDataDir() string {
	return w.BabylonNode.getNodeDataDir()
}

// getNodeDataDir returns the home path of the babylon node.
func (n *babylonNode) getNodeDataDir() string {
	dir := filepath.Join(n.dataDir, n.walletName, "babylond")
	return dir
}

// TxBankSend send transaction to a address from the node address.
func (n *babylonNode) TxBankSend(addr, coins string) error {
	flags := []string{
		"tx",
		"bank",
		"send",
		n.walletName,
		addr, coins,
		"--keyring-backend=test",
		fmt.Sprintf("--home=%s", n.getNodeDataDir()),
		"--log_level=debug",
		"--chain-id=chain-test",
		"-b=sync", "--yes", "--gas-prices=10ubbn",
	}

	cmd := exec.Command("babylond", flags...)
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

type balanceResponse struct {
	Balances []struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	} `json:"balances"`
	Pagination struct {
		Total string `json:"total"`
	} `json:"pagination"`
}

// CheckAddrBalance retrieves the balance of the specified address.
func (n *babylonNode) CheckAddrBalance(addr string) (int, error) {
	flags := []string{
		"query",
		"bank",
		"balances",
		addr,
		"--output=json",
		fmt.Sprintf("--home=%s", n.getNodeDataDir()),
		"--log_level=debug",
		"--chain-id=chain-test",
	}

	cmd := exec.Command("babylond", flags...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var resp balanceResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return 0, err
	}

	if len(resp.Balances) == 0 {
		return 0, fmt.Errorf("no balances found for address %s", addr)
	}

	balance, err := strconv.Atoi(resp.Balances[0].Amount)
	if err != nil {
		return 0, err
	}
	return balance, nil
}
