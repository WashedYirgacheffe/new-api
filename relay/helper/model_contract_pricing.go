package helper

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func ModelOperationParameters(request interface{}) (map[string]interface{}, error) {
	if request == nil {
		return map[string]interface{}{}, nil
	}
	payload, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	parameters := map[string]interface{}{}
	if err := common.Unmarshal(payload, &parameters); err != nil {
		return nil, err
	}
	return parameters, nil
}

func modelOperationMappedParameter(value interface{}, mappedName string) (interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if matched, ok := typed[mappedName]; ok {
			return matched, true
		}
		for _, nested := range typed {
			if matched, ok := modelOperationMappedParameter(nested, mappedName); ok {
				return matched, true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if matched, ok := modelOperationMappedParameter(nested, mappedName); ok {
				return matched, true
			}
		}
	}
	return nil, false
}

func modelOperationContractParameters(contract *model.ModelOperationEffectiveContract, parameters map[string]interface{}) map[string]interface{} {
	if contract == nil || len(contract.RequestContract.FieldMap) == 0 {
		return parameters
	}
	resolved := make(map[string]interface{}, len(parameters)+len(contract.RequestContract.FieldMap))
	for key, value := range parameters {
		resolved[key] = value
	}
	for field, mappedName := range contract.RequestContract.FieldMap {
		if _, exists := resolved[field]; exists {
			continue
		}
		if value, ok := modelOperationMappedParameter(parameters, mappedName); ok {
			resolved[field] = value
		}
	}
	return resolved
}

func ApplyModelOperationContractPricing(info *relaycommon.RelayInfo, operation string, parameters map[string]interface{}) (*model.ModelOperationEffectiveContract, error) {
	if info == nil {
		return nil, errors.New("relay info is required")
	}
	_, _, _, contract, err := model.GetEnabledModelOperationContract(info.OriginModelName, operation)
	if errors.Is(err, model.ErrModelOperationBindingNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ratios, err := model.CalculateModelOperationParameterRatios(contract, modelOperationContractParameters(contract, parameters))
	if err != nil {
		return nil, err
	}
	for key, ratio := range ratios {
		info.PriceData.AddOtherRatio(key, ratio)
	}
	return contract, nil
}

func ApplyModelOperationRatiosToPreConsume(info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("relay info is required")
	}
	if len(info.PriceData.OtherRatios()) == 0 {
		return nil
	}
	adjusted := info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.QuotaToPreConsume))
	quota, clamp := common.QuotaFromFloatChecked(adjusted)
	if quota < 0 {
		return fmt.Errorf("model operation contract produced negative pre-consume quota")
	}
	info.PriceData.QuotaToPreConsume = quota
	if clamp != nil {
		info.QuotaClamp = clamp
	}
	return nil
}
