package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"erc20-permit-relayer/common"
	"erc20-permit-relayer/core"
	"erc20-permit-relayer/store"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/inconshreveable/log15"
)

var (
	log            log15.Logger
	processRequest core.ProcessRequest
	signer         core.Signer
	keeper         core.Keeper
	txStore        store.TxStore
	wg             sync.WaitGroup
)

func handleRPCRequest(w http.ResponseWriter, r *http.Request) {
	var requestBody map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		log.Error("Failed to process request", "msg", "failed to parse request body")
		return
	}

	// Process
	response, err := processRequest.Process(requestBody)
	if err != nil {
		id, ok := requestBody["id"].(float64)
		if !ok {
			id = 1
		}

		log.Error("Failed to process request", "msg", err)
		response, _ = common.MakeJsonResponseError(id, -1, err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

func main() {
	// Startup
	log = log15.New()
	log.Info("🧙 ERC20 Permit Relayer RPC", "  🔑", "⛓️")
	log.Info("Proxy listening", "port", config.ProxyPort)
	log.Info("Connect rpc endpoint", "endpoint", config.RpcEndpoint)
	log.Info("Connect database", "postgres", config.Db.User+"@"+config.Db.Host+":"+config.Db.Port, "db", config.Db.Dbname)

	// Database
	txStore = *store.NewTxStore(&config, &log)
	err := txStore.Connect()
	if err != nil {
		log.Error("Failed to connect database", "error", err)
		return
	}
	defer txStore.Close()

	// Connect ethclient
	client, err := ethclient.Dial(config.RpcEndpoint)
	if err != nil {
		log.Error("Failed to connect rpc endpoint", "msg", err)
		return
	}

	// Signer
	signer = *core.NewSigner(&config, &log, &txStore, client, &wg)

	// Start Transaction Sender
	if config.Signer.Enable {
		go signer.Sender()
		defer signer.Close()
	} else {
		log.Info("Transaction Sender", "enable", false)
	}

	// Process request
	processRequest = *core.NewProcessRequest(&config, &log, &txStore, &signer)

	// Proxy http
	http.HandleFunc("/", handleRPCRequest)

	// New thread for http.ListenAndServe
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = http.ListenAndServe(":"+config.ProxyPort, nil)
		if err != nil {
			log.Error("Failed to start server", "error", err)
			return
		}
	}()

	// Keeper
	keeper = *core.NewKeeper(&config, &log, &txStore, client, &wg)

	// Start Transaction Keeper sync
	if config.Keeper.Enable {
		// Load config
		syncBlockNumber, err := txStore.GetKeeperBlockNumber()
		if err != nil {
			log.Error("Failed to connect database", "error", err)
			return
		}

		log.Info("Transaction Keeper", "intance", config.Keeper.InstanceId)
		log.Info("Start sync block number", "number", syncBlockNumber)

		go keeper.Sync(syncBlockNumber)
		defer keeper.Close()
	} else {
		log.Info("Transaction Keeper", "enable", false)
	}

	wg.Wait()
}
