package e2etest

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/stretchr/testify/require"
)

type BabylonNode struct {
	cmd        *exec.Cmd
	pidFile    string
	DataDir    string
	WalletName string
}

func newBabylonNode(dataDir, walletName string, cmd *exec.Cmd) *BabylonNode {
	return &BabylonNode{
		DataDir:    dataDir,
		cmd:        cmd,
		WalletName: walletName,
	}
}

func (n *BabylonNode) start() error {
	if err := n.cmd.Start(); err != nil {
		return err
	}

	pid, err := os.Create(filepath.Join(n.DataDir,
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

func (n *BabylonNode) stop() (err error) {
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

func (n *BabylonNode) cleanup() error {
	if n.pidFile != "" {
		if err := os.Remove(n.pidFile); err != nil {
			log.Printf("unable to remove file %s: %v", n.pidFile,
				err)
		}
	}

	dirs := []string{
		n.DataDir,
	}
	var err error
	for _, dir := range dirs {
		if err = os.RemoveAll(dir); err != nil {
			log.Printf("Cannot remove dir %s: %v", dir, err)
		}
	}
	return nil
}

func (n *BabylonNode) shutdown() error {
	if err := n.stop(); err != nil {
		return err
	}
	if err := n.cleanup(); err != nil {
		return err
	}
	return nil
}

type BabylonNodeHandler struct {
	BabylonNode *BabylonNode
}

func NewBabylonNodeHandler(t *testing.T, covenantQuorum int, covenantPks []*types.BIP340PubKey) *BabylonNodeHandler {
	testDir, err := baseDir("zBabylonTest")
	require.NoError(t, err)
	defer func() {
		if err != nil {
			err := os.RemoveAll(testDir)
			require.NoError(t, err)
		}
	}()

	walletName := "node0"
	nodeDataDir := filepath.Join(testDir, walletName, "babylond")

	slashingAddr := "SZtRT4BySL3o4efdGLh3k7Kny8GAnsBrSW"

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
		fmt.Sprintf("--slashing-address=%s", slashingAddr),
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
	require.NoError(t, err)
	t.Logf("babylon log file: %s", f.Name())

	startCmd := exec.Command(
		"babylond",
		"start",
		fmt.Sprintf("--home=%s", nodeDataDir),
		"--log_level=debug",
	)

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
	return w.BabylonNode.GetNodeDataDir()
}

// GetNodeDataDir returns the home path of the babylon node.
func (n *BabylonNode) GetNodeDataDir() string {
	dir := filepath.Join(n.DataDir, n.WalletName, "babylond")
	return dir
}

// TxBankSend send transaction to a address from the node address.
func (n *BabylonNode) TxBankSend(addr, coins string) error {
	flags := []string{
		"tx",
		"bank",
		"send",
		n.WalletName,
		addr, coins,
		"--keyring-backend=test",
		fmt.Sprintf("--home=%s", n.GetNodeDataDir()),
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
