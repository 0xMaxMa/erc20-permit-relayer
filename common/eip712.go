package common

import (
	"fmt"

	geth_common "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func VerifySignature(domain Domain, values PermitType, signature []byte) (geth_common.Address, error) {
	// EIP-2098; pull the v from the top bit of s and clear it
	if len(signature) != 65 {
		return Address0x0, fmt.Errorf("invalid signature length: %d", len(signature))
	}

	if signature[64] != 27 && signature[64] != 28 {
		return Address0x0, fmt.Errorf("invalid recovery id: %d", signature[64])
	}
	signature[64] -= 27

	// Make permit type values
	var typedData = &apitypes.TypedData{
		Domain: apitypes.TypedDataDomain{
			Name:              domain.Name,
			Version:           domain.Version,
			ChainId:           math.NewHexOrDecimal256(domain.ChainId),
			VerifyingContract: domain.VerifyingContract.Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"owner":    values.Owner.Hex(),
			"receiver": values.Receiver.Hex(),
			"value":    values.Value,
			"nonce":    values.Nonce,
			"deadline": values.Deadline,
		},
		Types: apitypes.Types{
			"EIP712Domain": {
				{
					Name: "name",
					Type: "string",
				},
				{
					Name: "version",
					Type: "string",
				},
				{
					Name: "chainId",
					Type: "uint256",
				},
				{
					Name: "verifyingContract",
					Type: "address",
				},
			},
			"Permit": {
				{
					Name: "owner",
					Type: "address",
				},
				{
					Name: "receiver",
					Type: "address",
				},
				{
					Name: "value",
					Type: "uint256",
				},
				{
					Name: "nonce",
					Type: "uint256",
				},
				{
					Name: "deadline",
					Type: "uint256",
				},
			},
		},
	}

	// Encodes domain type data
	encodeTypedData, err := encodeDomainTypeData(typedData)
	if err != nil {
		return Address0x0, err
	}

	// Hash encoded
	messageHash := crypto.Keccak256([]byte(encodeTypedData))

	// Recover the public key of signer address from the signature
	signerPublicKey, err := crypto.Ecrecover(messageHash, signature)
	if err != nil {
		return Address0x0, err
	}

	recoveredPublicKey, err := crypto.UnmarshalPubkey(signerPublicKey)
	if err != nil {
		return Address0x0, err
	}

	// Get the ethereum public address of the signer
	signerAddress := crypto.PubkeyToAddress(*recoveredPublicKey)

	return signerAddress, nil
}

func encodeDomainTypeData(typedData *apitypes.TypedData) ([]byte, error) {
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, err
	}

	typedDataHash, err := typedData.HashStruct("Permit", typedData.Message)
	if err != nil {
		return nil, err
	}

	// Encodes the hash that will be signed for the given EIP712 data
	rawData := []byte(fmt.Sprintf("\x19\x01%s%s", string(domainSeparator), string(typedDataHash)))
	return rawData, nil
}
