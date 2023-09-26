package common

import (
	"math/big"
	"time"

	geth_common "github.com/ethereum/go-ethereum/common"
)

// Constants
var (
	BigInt0        = big.NewInt(0)
	BigInt1        = big.NewInt(1)
	BigFloat0      = big.NewFloat(0)
	BigFloatBase18 = big.NewFloat(1e18)
	BigFloatBase9  = big.NewFloat(1e9)
	Address0x0     = geth_common.Address{0x0}
)

type DatabaseConnection struct {
	Host     string
	Port     int64
	User     string
	Password string
	Dbname   string
}

type SignerConfig struct {
	Enable           bool
	KeystoreFilePath string
	Password         string
	GasPrice         uint64
	GasLimit         uint64
	SenderInterval   time.Duration
	SenderBulkSize   int
}

type KeeperConfig struct {
	Enable                 bool
	InstanceId             string
	InitialSyncBlockNumber int64
	BlockBatchLimit        int64
	SyncingInterval        time.Duration
	LatestInterval         time.Duration
}

type Config struct {
	NetworkId               int64
	RpcEndpoint             string
	ProxyPort               string
	ERC20PermitTokenName    string
	ERC20PermitTokenAddress geth_common.Address
	DeadlineMinimum         int64
	Signer                  SignerConfig
	Keeper                  KeeperConfig
	Db                      DatabaseConnection
	LogDebug                bool
}

type Domain struct {
	Name              string
	Version           string
	ChainId           int64
	VerifyingContract geth_common.Address
}

type PermitType struct {
	Owner    geth_common.Address
	Receiver geth_common.Address
	Value    *big.Int
	Nonce    *big.Int
	Deadline *big.Int
}

var ERC20PermitTokenABI = `
[
   {
      "constant":false,
      "inputs":[
         {
            "name":"owner",
            "type":"address"
         },
         {
            "name":"receiver",
            "type":"address"
         },
         {
            "name":"value",
            "type":"uint256"
         },
         {
            "name":"deadline",
            "type":"uint256"
         },
         {
            "name":"v",
            "type":"uint8"
         },
         {
            "name":"r",
            "type":"bytes32"
         },
         {
            "name":"s",
            "type":"bytes32"
         }
      ],
      "name":"transferWithPermit",
      "type":"function"
   }
]`
