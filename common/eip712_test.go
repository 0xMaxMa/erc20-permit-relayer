package common

import (
	"encoding/hex"
	"math/big"
	"testing"

	geth_common "github.com/ethereum/go-ethereum/common"
)

func TestVerifySignature(t *testing.T) {
	ownerAddress := geth_common.HexToAddress("0xddDDd3bf0B0d7df20B0376b51b8c9e8F5968Ae4B")
	domain := Domain{
		Name:              "Test",
		Version:           "1.0",
		ChainId:           1,
		VerifyingContract: geth_common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}
	values := PermitType{
		Owner:    ownerAddress,
		Receiver: geth_common.HexToAddress("0x0987654321098765432109876543210987654321"),
		Value:    big.NewInt(1000000000),
		Nonce:    big.NewInt(1),
		Deadline: big.NewInt(1695600000),
	}
	hexStr := "9e035741b0f22daedd05d084902bc07cb2e7eebeaa9b7877c16ae1c72271c367147106ab98d131d8bf69c8ecdca9c4e72985def28347745df5b7f41ea9745e6d1b"
	signature, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Errorf("Decode signature returned error: %v", err)
	}

	signerAddress, err := VerifySignature(domain, values, signature)
	if err != nil {
		t.Errorf("VerifySignature returned error: %v", err)
	}
	if signerAddress != ownerAddress {
		t.Errorf("VerifySignature returned wrong address: expected %v, got %v", ownerAddress, signerAddress)
	}
}
