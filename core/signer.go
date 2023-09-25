package core

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"erc20-permit-relayer/common"
	"erc20-permit-relayer/store"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	geth_common "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/inconshreveable/log15"
)

type Signer struct {
	config              *common.Config
	log                 log15.Logger
	txStore             store.TxStore
	client              *ethclient.Client
	account             *keystore.Key
	erc20PermitTokenABI abi.ABI
	wg                  *sync.WaitGroup
	isClosed            bool
	mutex               sync.Mutex
}

func NewSigner(config *common.Config, log *log15.Logger, txStore *store.TxStore, client *ethclient.Client, wg *sync.WaitGroup) *Signer {
	var account *keystore.Key
	if config.Signer.Enable {
		// Load the keystore file
		keystoreJSON, err := os.ReadFile(config.Signer.KeystoreFilePath)
		if err != nil {
			(*log).Error("Failed to read keystore file", "msg", err)
		}

		// Unlock the account
		account, err = keystore.DecryptKey(keystoreJSON, config.Signer.Password)
		if err != nil {
			(*log).Error("Failed to unlock the account", "msg", err)
		} else {
			(*log).Info("Unlock account", "address", account.Address)
		}
	}

	// ABI
	erc20PermitTokenABI, err := abi.JSON(strings.NewReader(common.ERC20PermitTokenABI))
	if err != nil {
		(*log).Info("Failed to parse json abi", "msg", err)
	}

	return &Signer{
		config:              config,
		log:                 *log,
		txStore:             *txStore,
		client:              client,
		account:             account,
		erc20PermitTokenABI: erc20PermitTokenABI,
		wg:                  wg,
		isClosed:            false,
	}
}

func (s *Signer) Sender() {
	s.wg.Add(1)
	defer s.wg.Done()

	// Prepare defult config
	err := s.txStore.PrepareSignerConfig(strings.ToLower(s.account.Address.Hex()))
	if err != nil {
		s.log.Error("PrepareSignerConfig fail", "msg", err)
		return
	}

	ctx := context.Background()

	// Wait for startup ready
	time.Sleep(3000 * time.Millisecond)

	for !s.isClosed {
		// Bulk send transactions
		total, err := s.sendTransactions(ctx)
		if err != nil {
			s.log.Error("Failed to sendTransactions", "msg", err)
		}

		if total == 0 {
			// Fast sleep
			time.Sleep(3000 * time.Millisecond)
		} else {
			// Sleep
			time.Sleep(s.config.Signer.SenderInterval * time.Millisecond)
		}
	}
}

func (s *Signer) Close() {
	s.isClosed = true
}

func (s *Signer) sendTransactions(ctx context.Context) (int, error) {
	// Ensure only one access
	s.mutex.Lock()
	defer s.mutex.Unlock()

	start := mclock.Now()

	// Get pending txs
	txs, err := s.txStore.GetAllTxPending(s.config.Signer.SenderBulkSize)
	if err != nil {
		return 0, err
	}

	sendCount := 0
	for _, tx := range txs {
		var signedTx *types.Transaction

		// Decode []byte to Transaction
		decoder := gob.NewDecoder(bytes.NewBuffer(tx.TxSigned))
		err := decoder.Decode(&signedTx)
		if err != nil {
			return 0, err
		}

		// Send the transaction
		err = s.client.SendTransaction(ctx, signedTx)
		if err != nil {
			if err.Error() == "already known" {
				// tx exist in mempool
				s.log.Info("Skip transaction exist in mempool", "hash", signedTx.Hash())
				continue
			}
			return 0, err
		}

		s.log.Info("🔑 Submitted transaction", "  hash", signedTx.Hash())
		sendCount++
	}

	// Log
	if sendCount > 1 {
		s.log.Info("📦 Sent batch of transactions", "  count", sendCount, "elapsed", geth_common.PrettyDuration(mclock.Now().Sub(start)))
	}

	return len(txs), nil
}

func (s *Signer) AddPendingTransaction(values common.PermitType, signature []byte) (geth_common.Hash, error) {
	// Ensure only one access
	s.mutex.Lock()
	defer s.mutex.Unlock()

	ctx := context.Background()

	// Get next nonce from pending txs
	txNonce, err := s.client.PendingNonceAt(ctx, s.account.Address)
	if err != nil {
		return geth_common.Hash{}, err
	}
	// Get next nonce from signer_config
	localTxNonce, err := s.txStore.GetSignerTxNonce(strings.ToLower(s.account.Address.Hex()))
	if err != nil {
		return geth_common.Hash{}, err
	}
	// Use highest nonce
	if localTxNonce > txNonce {
		txNonce = localTxNonce
	}

	// Split signature
	var _r [32]byte
	var _s [32]byte
	var _v uint8
	copy(_r[:], signature[:32])
	copy(_s[:], signature[32:64])
	_v = uint8(signature[64] + 27)

	// ABI encode function call
	data, err := s.erc20PermitTokenABI.Pack("transferWithPermit", values.Owner, values.Receiver, values.Value, values.Deadline, _v, _r, _s)
	if err != nil {
		return geth_common.Hash{}, err
	}

	// Make Tx
	tx := types.NewTransaction(txNonce, geth_common.HexToAddress(s.config.ERC20PermitTokenAddress.Hex()), nil, s.config.Signer.GasLimit, big.NewInt(int64(s.config.Signer.GasPrice)), data)

	// Sign the transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(s.config.NetworkId)), s.account.PrivateKey)
	if err != nil {
		return geth_common.Hash{}, err
	}

	// Encode the signedTx to []byte
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err = encoder.Encode(signedTx)
	if err != nil {
		return geth_common.Hash{}, err
	}

	// Get the transaction hash before sending
	txHash := signedTx.Hash()

	// Check tx_pedning exist
	_tx, _ := s.txStore.GetTxPending(txHash.Hex())
	if _tx.TxHash == txHash.Hex() {
		return geth_common.Hash{}, fmt.Errorf("transaction already exist")
	}

	// Insert pending tx
	err = s.txStore.AddTxPending(txHash.Hex(), values.Owner.Hex(), values.Receiver.Hex(), values.Value, values.Nonce, buffer.Bytes(), txNonce)
	if err != nil {
		return geth_common.Hash{}, err
	}

	// Update next nonce
	err = s.txStore.UpdateSignerTxNonce(strings.ToLower(s.account.Address.Hex()), txNonce+1)
	if err != nil {
		return geth_common.Hash{}, err
	}

	return txHash, nil
}
