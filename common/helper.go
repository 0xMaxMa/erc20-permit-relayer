package common

import (
	"encoding/json"
	"math/big"
)

func ParseEther(wei *big.Int) *big.Float {
	return new(big.Float).Quo(new(big.Float).SetInt(wei), BigFloatBase18)
}

func MakeJsonResponseResult(id float64, result string) ([]byte, error) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return jsonResponse, nil
}

func MakeJsonResponseError(id float64, code float64, msg string) ([]byte, error) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": msg,
		},
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return jsonResponse, nil
}
