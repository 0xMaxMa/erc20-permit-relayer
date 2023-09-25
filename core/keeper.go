package core

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"time"

	"erc20-permit-relayer/common"
	"erc20-permit-relayer/store"

	geth_common "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/inconshreveable/log15"
)

type Keeper struct {
	config   *common.Config
	log      log15.Logger
	txStore  store.TxStore
	client   *ethclient.Client
	wg       *sync.WaitGroup
	isClosed bool
}

func NewKeeper(config *common.Config, log *log15.Logger, txStore *store.TxStore, client *ethclient.Client, wg *sync.WaitGroup) *Keeper {
	return &Keeper{
		config:   config,
		log:      *log,
		txStore:  *txStore,
		client:   client,
		wg:       wg,
		isClosed: false,
	}
}

func (k *Keeper) Sync(syncBlockNumber int64) {
	k.wg.Add(1)
	defer k.wg.Done()

	// Prepare defult config
	err := k.txStore.PrepareKeeperConfig(k.config.Keeper.InitialSyncBlockNumber)
	if err != nil {
		k.log.Error("PrepareKeeperConfig fail", "msg", err)
		return
	}

	ctx := context.Background()
	blockNumber := big.NewInt(syncBlockNumber - 1) // rewind 1 block
	isSyncing := true

	for i := uint64(1); !k.isClosed; i++ {
		// Process txs
		isSyncing, blockNumber = k.processTransactions(ctx, blockNumber)

		// Update block_number every 10 rounds
		if i%10 == 0 {
			err := k.txStore.UpdateKeeperBlockNumber(blockNumber.Int64())
			if err != nil {
				k.log.Error("Cannot update keeper block number", "msg", err)
			}
		}

		// Sleep
		if isSyncing {
			time.Sleep(k.config.Keeper.SyncingInterval * time.Millisecond)
		} else {
			time.Sleep(k.config.Keeper.LatestInterval * time.Millisecond)
		}
	}
}

func (k *Keeper) Close() {
	k.isClosed = true
}

func (k *Keeper) processTransactions(ctx context.Context, blockNumber *big.Int) (bool, *big.Int) {
	start := mclock.Now()

	// Get Latest block
	latestBlock, err := k.client.BlockByNumber(ctx, nil)
	if err != nil {
		k.log.Error("Failed to retrieve latest block", "msg", err)
		return true, blockNumber
	}

	// Get Current sync block
	blockCount := k.config.Keeper.BlockBatchLimit
	isSyncing := latestBlock.Number().Int64()-blockNumber.Int64() > blockCount
	if !isSyncing {
		blockCount = latestBlock.Number().Int64() - blockNumber.Int64()
	}
	if blockCount < 1 {
		return false, blockNumber
	}

	var (
		block *types.Block
		txs   int = 0
	)
	for i := int64(0); i < blockCount; i++ {
		// Get next block
		nextBlock := new(big.Int).Add(blockNumber, big.NewInt(i+1))
		block, err = k.client.BlockByNumber(ctx, nextBlock)
		if err != nil {
			k.log.Error("Failed to retrieve block", "number", nextBlock.String(), "msg", err)
			return true, blockNumber
		}

		// Process txs in block
		for _, tx := range block.Transactions() {
			// Skip non ERC20PermitTokenAddress
			if tx.To() == nil || !bytes.Equal(tx.To().Bytes(), k.config.ERC20PermitTokenAddress.Bytes()) {
				continue
			}

			// Get tx receipt
			txHash := tx.Hash()
			receipt, err := k.client.TransactionReceipt(context.Background(), txHash)
			if err != nil || receipt == nil {
				k.log.Error("Transaction receipt fail", "hash", tx.Hash(), "block", block.Number(), "msg", err)
				return false, blockNumber
			}

			if receipt.Status == 1 {
				// Transaction succeeded
				// clear from tx_pending, move to tx_submited
				ok, _tx, err := k.txStore.UpdateTxPendingToSubmited(txHash.Hex())
				if !ok && err != nil {
					k.log.Error("Cannot update tx pending to submited", "hash", tx.Hash(), "msg", err)
					return false, blockNumber
				}

				if ok {
					duration := time.Since(_tx.Timestamp)
					k.log.Info("🔗 Finalized transaction", "  hash", tx.Hash(), "finalized", geth_common.PrettyDuration(duration))
				}
			} else if receipt.Status == 0 {
				// Transaction failed or was reverted
				// clear from tx_pending, move to tx_fail to enqueue for retry again
				ok, _, err := k.txStore.UpdateTxPendingToFail(txHash.Hex())
				if !ok && err != nil {
					k.log.Error("Cannot update tx pending to fail", "hash", tx.Hash(), "msg", err)
					return false, blockNumber
				}

				k.log.Info("😱 Transaction receipt fail", "  hash", tx.Hash(), "msg", "tx failed or reverted, enqueue to retry again")
			}
		}
		txs += len(block.Transactions())
	}

	// Log
	if isSyncing {
		k.log.Info("Syncing transactions", "number", block.Number().String()+"/"+latestBlock.Number().String(), "blocks", blockCount, "txs", txs, "elapsed", geth_common.PrettyDuration(mclock.Now().Sub(start)))
	} else {
		k.log.Info("Process transactions", "number", block.Number().String(), "blocks", blockCount, "txs", txs, "elapsed", geth_common.PrettyDuration(mclock.Now().Sub(start)))
	}

	return isSyncing, block.Number()
}
