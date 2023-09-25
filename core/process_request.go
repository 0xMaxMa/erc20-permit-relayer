package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"erc20-permit-relayer/common"
	"erc20-permit-relayer/store"

	geth_common "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/inconshreveable/log15"
)

type ProcessRequest struct {
	config  *common.Config
	log     log15.Logger
	txStore store.TxStore
	signer  Signer
	mutex   sync.Mutex
}

func NewProcessRequest(config *common.Config, log *log15.Logger, txStore *store.TxStore, signer *Signer) *ProcessRequest {
	return &ProcessRequest{
		config:  config,
		log:     *log,
		txStore: *txStore,
		signer:  *signer,
	}
}

func (p *ProcessRequest) Process(requestBody map[string]interface{}) ([]byte, error) {
	method, ok := requestBody["method"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid request format: method not found")
	}

	// Check to process request eth_call
	if method == "eth_call" {
		params, ok := requestBody["params"].([]interface{})
		if !ok || len(params) == 0 {
			return nil, fmt.Errorf("invalid eth_call params format")
		}

		data, ok := params[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid eth_call params format")
		}

		to, ok := data["to"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid eth_call params format")
		}

		// Check ERC20PermitTokenAddress
		if bytes.Equal(p.config.ERC20PermitTokenAddress.Bytes(), geth_common.HexToAddress(to).Bytes()) {
			calldata := strings.ToLower(data["data"].(string))

			if len(calldata) == 74 && calldata[:10] == "0x70a08231" { // ERC20.balanceOf(address)
				account := "0x" + calldata[34:]
				return p.queryERC20BalanceOf(requestBody, account)
			} else if len(calldata) == 74 && calldata[:10] == "0x7ecebe00" { // ERC20Permit.nonces(address)
				account := "0x" + calldata[34:]
				return p.queryERC20PermitNonce(requestBody, account)
			}
		}
	} else if method == "delegate_permit" {
		params, ok := requestBody["params"].([]interface{})
		if !ok || len(params) == 0 {
			return nil, fmt.Errorf("invalid delegate_permit params format")
		}

		data, ok := params[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid delegate_permit params format")
		}

		// Parse parameters
		values, signature, err := p.parseDelegatePermitParams(data)
		if err != nil {
			return nil, fmt.Errorf("invalid delegate_permit params: %v", err)
		}

		// Verify permit signature
		if err = p.verifyPermit(values, signature); err != nil {
			return nil, fmt.Errorf("invalid verify permit with signature: %v", err)
		}

		// Verify balance, nonce, deadline
		if err = p.verifyData(values); err != nil {
			return nil, fmt.Errorf("invalid verify data: %v", err)
		}

		if p.config.LogDebug {
			p.log.Debug("Incoming delegate_permit", "owner", data["owner"], "receiver", data["receiver"], "value", data["value"])
		}

		// Added tx to tx_pending
		txHash, err := p.signer.AddPendingTransaction(values, signature)
		if err != nil {
			return nil, fmt.Errorf("failed to add pending transaction: %v", err)
		}

		return common.MakeJsonResponseResult(requestBody["id"].(float64), txHash.Hex())
	}

	// Others case
	return p.forwardRequest(requestBody)
}

func (p *ProcessRequest) parseDelegatePermitParams(data map[string]interface{}) (common.PermitType, []byte, error) {
	if _, ok := data["owner"].(string); !ok {
		return common.PermitType{}, nil, fmt.Errorf("invalid owner")
	}
	if _, ok := data["receiver"].(string); !ok {
		return common.PermitType{}, nil, fmt.Errorf("invalid receiver")
	}
	if _, ok := data["value"].(string); !ok {
		return common.PermitType{}, nil, fmt.Errorf("invalid value")
	}
	if _, ok := data["signature"].(string); !ok {
		return common.PermitType{}, nil, fmt.Errorf("invalid signature")
	}

	ownerAddress := geth_common.HexToAddress(data["owner"].(string))
	receiverAddress := geth_common.HexToAddress(data["receiver"].(string))
	value, _ := new(big.Int).SetString(data["value"].(string), 10)

	nonce := new(big.Int)
	if _, ok := data["nonce"].(string); ok {
		nonce.SetString(data["nonce"].(string), 10)
	} else if _, ok := data["nonce"].(float64); ok {
		nonce.SetInt64(int64(data["nonce"].(float64)))
	} else {
		return common.PermitType{}, nil, fmt.Errorf("invalid nonce")
	}

	deadline := new(big.Int)
	if _, ok := data["deadline"].(string); ok {
		deadline.SetString(data["deadline"].(string), 10)
	} else if _, ok := data["deadline"].(float64); ok {
		deadline.SetInt64(int64(data["deadline"].(float64)))
	} else {
		return common.PermitType{}, nil, fmt.Errorf("invalid deadline")
	}

	values := common.PermitType{
		Owner:    ownerAddress,
		Receiver: receiverAddress,
		Value:    value,
		Nonce:    nonce,
		Deadline: deadline,
	}

	signature, err := hexutil.Decode(data["signature"].(string))
	if err != nil {
		return common.PermitType{}, nil, err
	}

	return values, signature, nil
}

func (p *ProcessRequest) verifyPermit(values common.PermitType, signature []byte) error {
	domain := common.Domain{
		Name:              p.config.ERC20PermitTokenName,
		Version:           "1",
		ChainId:           p.config.NetworkId,
		VerifyingContract: p.config.ERC20PermitTokenAddress,
	}

	signerAddress, err := common.VerifySignature(domain, values, signature)
	if err != nil {
		return err
	}

	// Check signer must equal owner
	if !bytes.Equal(signerAddress.Bytes(), values.Owner.Bytes()) {
		return fmt.Errorf("recovered signer mismatch, signer: %v owner: %v", signerAddress.Hex(), values.Owner.Hex())
	}

	return nil
}

func (p *ProcessRequest) verifyData(values common.PermitType) error {
	// Ensure only one access
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Check nonce
	nonce, err := p.wrapQueryERC20PermitNonce(values.Owner.Hex())
	if err != nil {
		return err
	}
	if nonce.Cmp(values.Nonce) != 0 { // Require must equal next nonce
		return fmt.Errorf("invalid nonce")
	}

	// Check balance
	balance, err := p.wrapQueryERC20BalanceOf(values.Owner.Hex())
	if err != nil {
		return err
	}
	if balance.Cmp(values.Value) < 0 {
		return fmt.Errorf("insifficient balance")
	}

	// Check deadline
	differenceInSeconds := values.Deadline.Int64() - time.Now().Unix()
	if differenceInSeconds < p.config.DeadlineMinimum {
		return fmt.Errorf("minimum deadline is %d days", p.config.DeadlineMinimum/(24*60*60))
	}

	return nil
}

func (p *ProcessRequest) wrapQueryERC20BalanceOf(account string) (*big.Int, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_call",
		"params": []interface{}{
			map[string]interface{}{
				"data": "0x70a08231000000000000000000000000" + account[2:],
				"to":   p.config.ERC20PermitTokenAddress.Hex(),
			},
			"latest",
		},
		"id": 1,
	}

	response, err := p.queryERC20BalanceOf(payload, account)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON string into the struct
	var data map[string]interface{}
	err = json.Unmarshal(response, &data)
	if err != nil {
		return nil, err
	}

	balance, _ := new(big.Int).SetString(data["result"].(string)[2:], 16) // remove 0x
	return balance, nil
}

func (p *ProcessRequest) wrapQueryERC20PermitNonce(account string) (*big.Int, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_call",
		"params": []interface{}{
			map[string]interface{}{
				"data": "0x7ecebe00000000000000000000000000" + account[2:],
				"to":   p.config.ERC20PermitTokenAddress.Hex(),
			},
			"latest",
		},
		"id": 1,
	}

	response, err := p.queryERC20PermitNonce(payload, account)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON string into the struct
	var data map[string]interface{}
	err = json.Unmarshal(response, &data)
	if err != nil {
		return nil, err
	}

	nonce, _ := new(big.Int).SetString(data["result"].(string)[2:], 16) // remove 0x
	return nonce, nil
}

func (p *ProcessRequest) queryERC20BalanceOf(requestBody map[string]interface{}, account string) ([]byte, error) {
	start := mclock.Now()

	// Get balanceOf from direct rpc
	response, err := p.forwardRequest(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Define a struct to hold the relevant fields
	var data map[string]interface{}

	// Unmarshal the JSON string into the struct
	err = json.Unmarshal(response, &data)
	if err != nil {
		return nil, err
	}

	// Check nil result
	if data["result"] == nil {
		return nil, fmt.Errorf("failed to read response data result")
	}

	balance := new(big.Int)
	balance.SetString(data["result"].(string)[2:], 16) // remove 0x

	// Get pending balance from txStore
	pending_balance, err := p.txStore.GetPendingBalance(account)
	if err != nil {
		return nil, err
	}

	// Update unrealize balance
	unrealize_balance := new(big.Int).Add(balance, pending_balance)

	if p.config.LogDebug {
		p.log.Debug("Query ERC20.balanceOf", "account", account, "realize", common.ParseEther(balance), "pending", common.ParseEther(pending_balance), "unrealize", common.ParseEther(unrealize_balance), "elapsed", geth_common.PrettyDuration(mclock.Now().Sub(start)))
	}

	// Check negative value to default 0
	if unrealize_balance.Sign() < 0 {
		unrealize_balance = geth_common.Big0
	}

	// Update result with uint256 (64 hexadecimal characters)
	hexStr := unrealize_balance.Text(16)
	data["result"] = fmt.Sprintf("0x%s", strings.Repeat("0", 64-len(hexStr))+hexStr)

	// Marshal the updated data back to JSON
	updatedJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return updatedJSON, nil
}

func (p *ProcessRequest) queryERC20PermitNonce(requestBody map[string]interface{}, account string) ([]byte, error) {
	start := mclock.Now()

	// Get balanceOf from direct rpc
	response, err := p.forwardRequest(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Define a struct to hold the relevant fields
	var data map[string]interface{}

	// Unmarshal the JSON string into the struct
	err = json.Unmarshal(response, &data)
	if err != nil {
		return nil, err
	}

	// Check nil result
	if data["result"] == nil {
		return nil, fmt.Errorf("failed to read response data result")
	}

	nonce := new(big.Int)
	nonce.SetString(data["result"].(string)[2:], 16) // remove 0x

	// Get pending balance from txStore
	pending_txs, err := p.txStore.GetPendingTxs(account)
	if err != nil {
		return nil, err
	}

	// Update latest_nonce
	latest_nonce := new(big.Int).Add(nonce, big.NewInt(pending_txs))

	if p.config.LogDebug {
		p.log.Debug("Query ERC20Permit.nonce", "account", account, "nonce", latest_nonce.String(), "pending_txs", pending_txs, "elapsed", geth_common.PrettyDuration(mclock.Now().Sub(start)))
	}

	// Update result with uint256 (64 hexadecimal characters)
	hexStr := latest_nonce.Text(16)
	data["result"] = fmt.Sprintf("0x%s", strings.Repeat("0", 64-len(hexStr))+hexStr)

	// Marshal the updated data back to JSON
	updatedJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return updatedJSON, nil
}

func (p *ProcessRequest) forwardRequest(requestBody map[string]interface{}) ([]byte, error) {
	reqJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(p.config.RpcEndpoint, "application/json", bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to endpoint: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBody, nil
}
