package common

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	geth_common "github.com/ethereum/go-ethereum/common"
)

func LoadConfig() (*Config, error) {
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
	config := Config{
		NetworkId:               configToml["network_id"].(int64),
		RpcEndpoint:             configToml["rpc_endpoint"].(string),
		ProxyPort:               configToml["proxy_port"].(string),
		ERC20PermitTokenName:    configToml["erc20_permit_token_name"].(string),
		ERC20PermitTokenAddress: geth_common.HexToAddress(configToml["erc20_permit_token_address"].(string)),
		DeadlineMinimum:         configToml["deadline_minimum"].(int64),

		Signer: SignerConfig{
			Enable:           configToml["signer"].(map[string]interface{})["enable"].(bool),
			KeystoreFilePath: configToml["signer"].(map[string]interface{})["keystore_file_path"].(string),
			Password:         configToml["signer"].(map[string]interface{})["password"].(string),
			GasPrice:         uint64(configToml["signer"].(map[string]interface{})["gas_price"].(int64)),
			GasLimit:         uint64(configToml["signer"].(map[string]interface{})["gas_limit"].(int64)),
			SenderInterval:   time.Duration(configToml["signer"].(map[string]interface{})["sender_interval"].(int64)),
			SenderBulkSize:   int(configToml["signer"].(map[string]interface{})["sender_bulk_size"].(int64)),
		},

		Keeper: KeeperConfig{
			Enable:                 configToml["keeper"].(map[string]interface{})["enable"].(bool),
			InstanceId:             configToml["keeper"].(map[string]interface{})["instance_id"].(string),
			InitialSyncBlockNumber: configToml["keeper"].(map[string]interface{})["initial_sync_block_number"].(int64),
			BlockBatchLimit:        configToml["keeper"].(map[string]interface{})["block_batch_limit"].(int64),
			SyncingInterval:        time.Duration(configToml["keeper"].(map[string]interface{})["syncing_interval"].(int64)),
			LatestInterval:         time.Duration(configToml["keeper"].(map[string]interface{})["latest_interval"].(int64)),
		},

		Db: DatabaseConnection{
			Host:     configToml["db"].(map[string]interface{})["host"].(string),
			Port:     configToml["db"].(map[string]interface{})["port"].(int64),
			User:     configToml["db"].(map[string]interface{})["user"].(string),
			Password: configToml["db"].(map[string]interface{})["password"].(string),
			Dbname:   configToml["db"].(map[string]interface{})["database"].(string),
		},

		LogDebug: configToml["log_debug"].(bool),
	}

	return &config, nil
}
