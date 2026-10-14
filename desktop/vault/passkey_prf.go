package vault

import (
	"bytes"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/passkey"
)

func decodePRFInput(values *browserprotocol.PasskeyPRFValues) (passkey.PRFInput, error) {
	first, err := decodeBase64URL(values.First)
	if err != nil || len(first) > passkey.MaxPRFInputLength {
		return passkey.PRFInput{}, passkey.ErrInvalidRequest
	}
	input := passkey.PRFInput{First: first}
	if values.Second != nil {
		second, err := decodeBase64URL(*values.Second)
		if err != nil || len(second) > passkey.MaxPRFInputLength {
			return passkey.PRFInput{}, passkey.ErrInvalidRequest
		}
		input.Second, input.HasSecond = second, true
	}
	return input, nil
}

func evaluatePRFValues(values *browserprotocol.PasskeyPRFValues, privateKeyDER []byte) (*browserprotocol.PasskeyPRFValues, error) {
	input, err := decodePRFInput(values)
	if err != nil {
		return nil, err
	}
	output, err := passkey.EvaluatePRFForPasskey(privateKeyDER, input)
	if err != nil {
		return nil, err
	}
	results := &browserprotocol.PasskeyPRFValues{First: encodeBase64URL(output.First)}
	if input.HasSecond {
		second := encodeBase64URL(output.Second)
		results.Second = &second
	}
	return results, nil
}

// evaluatePasskeyCreationPRF runs before the new passkey is saved, so an invalid input saves nothing.
func evaluatePasskeyCreationPRF(request *browserprotocol.PasskeyPRFRequest, privateKeyDER []byte) (*browserprotocol.PasskeyPRFResult, error) {
	if request == nil {
		return nil, nil
	}
	if len(request.EvalByCredential) > 0 {
		return nil, passkey.ErrInvalidRequest
	}
	result := &browserprotocol.PasskeyPRFResult{IsEnabled: true}
	if request.Eval != nil {
		results, err := evaluatePRFValues(request.Eval, privateKeyDER)
		if err != nil {
			return nil, err
		}
		result.Results = results
	}
	return result, nil
}

// selectAssertionPRFInput applies the spec's evalByCredential checks against allowCredentials.
func selectAssertionPRFInput(request *browserprotocol.PasskeyPRFRequest, allAllowedID [][]byte, credentialID []byte) (*browserprotocol.PasskeyPRFValues, error) {
	if len(request.EvalByCredential) > 0 && len(allAllowedID) == 0 {
		return nil, passkey.ErrInvalidRequest
	}
	if len(request.EvalByCredential) > maxPasskeyCredentialCount {
		return nil, passkey.ErrInvalidRequest
	}
	var selected *browserprotocol.PasskeyPRFValues
	for encodedID, values := range request.EvalByCredential {
		decodedID, err := decodeBase64URL(encodedID)
		if err != nil || len(decodedID) == 0 || !containsCredentialID(allAllowedID, decodedID) {
			return nil, passkey.ErrInvalidRequest
		}
		if bytes.Equal(decodedID, credentialID) {
			selected = &values
		}
	}
	if selected == nil {
		selected = request.Eval
	}
	return selected, nil
}

func evaluatePasskeyAssertionPRF(request *browserprotocol.PasskeyPRFRequest, allAllowedID [][]byte, storedPasskey item.Passkey) (*browserprotocol.PasskeyPRFResult, error) {
	if request == nil {
		return nil, nil
	}
	selected, err := selectAssertionPRFInput(request, allAllowedID, storedPasskey.CredentialID)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return &browserprotocol.PasskeyPRFResult{}, nil
	}
	results, err := evaluatePRFValues(selected, storedPasskey.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &browserprotocol.PasskeyPRFResult{Results: results}, nil
}
