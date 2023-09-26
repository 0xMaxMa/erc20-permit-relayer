package main

import (
	"erc20-permit-relayer/common"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	geth_common "github.com/ethereum/go-ethereum/common"
)

// import (
// 	"erc20-permit-relayer/common"

// 	geth_common "github.com/ethereum/go-ethereum/common"
// )

// var config = common.Config{
// 	NetworkId:               11155111,
// 	RpcEndpoint:             "https://ethereum-sepolia.blockpi.network/v1/rpc/public",
// 	ProxyPort:               "8545",
// 	ERC20PermitTokenName:    "Digital10kToken",
// 	ERC20PermitTokenAddress: geth_geth_common.HexToAddress("0xFF2F0676e588bdCA786eBF25d55362d4488Fad64"),
// 	DeadlineMinimum:         90 * (24 * 60 * 60), // 90 days

// 	Signer: common.SignerConfig{
// 		Enable:           true,
// 		KeystoreFilePath: "./.keystore",
// 		// KeystoreFilePath: "/data/.keystore", // docker compose
// 		Password:       "unlock_password",
// 		GasPrice:       5000000000, // 5 gwei
// 		GasLimit:       3000000,
// 		SenderInterval: 60 * 1000, // 60 secs
// 		SenderBulkSize: 50,        // txs
// 	},

// 	Keeper: common.KeeperConfig{
// 		Enable:                 true,  // If enable require RpcEndpoint with archive
// 		InstanceId:             "dev", // require unique id for multiple keeper instance
// 		InitialSyncBlockNumber: 4353360,
// 		BlockBatchLimit:        20,
// 		SyncingInterval:        100,  // 100 ms
// 		LatestInterval:         1500, // 1.5 s
// 	},

// 	Db: common.DatabaseConnection{ // postgres database connection
// 		Host: "localhost",
// 		// Host:     "db", // docker compose
// 		Port:     "5432",
// 		User:     "postgres",
// 		Password: "password",
// 		Dbname:   "relayer_db",
// 	},

// 	LogDebug: true,
// }

func LoadConfig() (*common.Config, error) {
	// Check --config flag
	configPath := flag.String("config", "", "path to TOML configuration file, Example: ./config.toml")
	flag.Parse()

	if *configPath == "" {
		return nil, fmt.Errorf("missing required --config flag")
	}

	// Check if the file exists
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file %s does not exist", *configPath)
	}

	// Read the config file contents
	configData, err := os.ReadFile(*configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var configToml map[string]interface{}
	if _, err := toml.Decode(string(configData), &configToml); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// Apply default values
	config := common.Config{
		NetworkId:               configToml["network_id"].(int64),
		RpcEndpoint:             configToml["rpc_endpoint"].(string),
		ProxyPort:               configToml["proxy_port"].(string),
		ERC20PermitTokenName:    configToml["erc20_permit_token_name"].(string),
		ERC20PermitTokenAddress: geth_common.HexToAddress(configToml["erc20_permit_token_address"].(string)),
		DeadlineMinimum:         configToml["deadline_minimum"].(int64),

		Signer: common.SignerConfig{
			Enable:           configToml["signer"].(map[string]interface{})["enable"].(bool),
			KeystoreFilePath: configToml["signer"].(map[string]interface{})["keystore_file_path"].(string),
			Password:         configToml["signer"].(map[string]interface{})["password"].(string),
			GasPrice:         uint64(configToml["signer"].(map[string]interface{})["gas_price"].(int64)),
			GasLimit:         uint64(configToml["signer"].(map[string]interface{})["gas_limit"].(int64)),
			SenderInterval:   time.Duration(configToml["signer"].(map[string]interface{})["sender_interval"].(int64)),
			SenderBulkSize:   int(configToml["signer"].(map[string]interface{})["sender_bulk_size"].(int64)),
		},

		Keeper: common.KeeperConfig{
			Enable:                 configToml["keeper"].(map[string]interface{})["enable"].(bool),
			InstanceId:             configToml["keeper"].(map[string]interface{})["instance_id"].(string),
			InitialSyncBlockNumber: configToml["keeper"].(map[string]interface{})["initial_sync_block_number"].(int64),
			BlockBatchLimit:        configToml["keeper"].(map[string]interface{})["block_batch_limit"].(int64),
			SyncingInterval:        time.Duration(configToml["keeper"].(map[string]interface{})["syncing_interval"].(int64)),
			LatestInterval:         time.Duration(configToml["keeper"].(map[string]interface{})["latest_interval"].(int64)),
		},

		Db: common.DatabaseConnection{
			Host:     configToml["db"].(map[string]interface{})["host"].(string),
			Port:     configToml["db"].(map[string]interface{})["port"].(int64),
			User:     configToml["db"].(map[string]interface{})["user"].(string),
			Password: configToml["db"].(map[string]interface{})["password"].(string),
			Dbname:   configToml["db"].(map[string]interface{})["database"].(string),
		},

		LogDebug: configToml["log_debug"].(bool), // configToml["log_debug"].(bool),
	}

	return &config, nil
}
