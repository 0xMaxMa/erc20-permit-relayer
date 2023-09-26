package common

import (
	"math/big"
	"testing"
)

func TestParseEther(t *testing.T) {
	wei := big.NewInt(1000000000000000000) // 1 ether in wei
	expected := big.NewFloat(1.0)
	actual := ParseEther(wei)
	if actual.Cmp(expected) != 0 {
		t.Errorf("ParseEther returned wrong value: expected %v, got %v", expected, actual)
	}
}

func TestMakeJsonResponseResult(t *testing.T) {
	id := 123.0
	result := "test result"
	expected := `{"id":123,"jsonrpc":"2.0","result":"test result"}`

	actual, err := MakeJsonResponseResult(id, result)
	if err != nil {
		t.Errorf("MakeJsonResponseResult returned error: %v", err)
	}
	if string(actual) != expected {
		t.Errorf("MakeJsonResponseResult returned wrong JSON: expected %v, got %v", expected, string(actual))
	}
}

func TestMakeJsonResponseError(t *testing.T) {
	id := 123.0
	code := -100.0
	msg := "test error"
	expected := `{"error":{"code":-100,"message":"test error"},"id":123,"jsonrpc":"2.0"}`

	actual, err := MakeJsonResponseError(id, code, msg)
	if err != nil {
		t.Errorf("MakeJsonResponseError returned error: %v", err)
	}
	if string(actual) != expected {
		t.Errorf("MakeJsonResponseError returned wrong JSON: expected %v, got %v", expected, string(actual))
	}
}
