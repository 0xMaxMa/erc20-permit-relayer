package main

import (
	"erc20-permit-relayer/common"

	geth_common "github.com/ethereum/go-ethereum/common"
)

var config = common.Config{
	NetworkId:               11155111,
	RpcEndpoint:             "https://ethereum-sepolia.blockpi.network/v1/rpc/public",
	ProxyPort:               "8545",
	ERC20PermitTokenName:    "Digital10kToken",
	ERC20PermitTokenAddress: geth_common.HexToAddress("0xFF2F0676e588bdCA786eBF25d55362d4488Fad64"),
	DeadlineMinimum:         90 * (24 * 60 * 60), // 90 days

	Signer: common.SignerConfig{
		Enable:           true,
		KeystoreFilePath: "./.keystore",
		// KeystoreFilePath: "/data/.keystore", // docker compose
		Password:       "unlock_password",
		GasPrice:       5000000000, // 5 gwei
		GasLimit:       3000000,
		SenderInterval: 60 * 1000, // 60 secs
		SenderBulkSize: 50,        // txs
	},

	Keeper: common.KeeperConfig{
		Enable:                 true,  // If enable require RpcEndpoint with archive
		InstanceId:             "dev", // require unique id for multiple keeper instance
		InitialSyncBlockNumber: 4353360,
		BlockBatchLimit:        20,
		SyncingInterval:        100,  // 100 ms
		LatestInterval:         1500, // 1.5 s
	},

	Db: common.DatabaseConnection{ // postgres database connection
		Host: "localhost",
		// Host:     "db", // docker compose
		Port:     "5432",
		User:     "postgres",
		Password: "password",
		Dbname:   "relayer_db",
	},

	LogDebug: true,
}
