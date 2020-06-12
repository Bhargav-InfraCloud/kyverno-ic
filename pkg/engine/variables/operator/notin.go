package operator

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/nirmata/kyverno/pkg/engine/context"
)

//NewNotInHandler returns handler to manage NotIn operations
func NewNotInHandler(log logr.Logger, ctx context.EvalInterface, subHandler VariableSubstitutionHandler) OperatorHandler {
	return NotInHandler{
		ctx:        ctx,
		subHandler: subHandler,
		log:        log,
	}
}

//NotInHandler provides implementation to handle NotIn Operator
type NotInHandler struct {
	ctx        context.EvalInterface
	subHandler VariableSubstitutionHandler
	log        logr.Logger
}

//Evaluate evaluates expression with NotIn Operator
func (neh NotInHandler) Evaluate(key, value interface{}) bool {
	var err error
	//TODO: decouple variables from evaluation
	// substitute the variables
	if key, err = neh.subHandler(neh.log, neh.ctx, key); err != nil {
		// Failed to resolve the variable
		neh.log.Error(err, "Failed to resolve variable", "variable", key)
		return false
	}
	if value, err = neh.subHandler(neh.log, neh.ctx, value); err != nil {
		// Failed to resolve the variable
		neh.log.Error(err, "Failed to resolve variable", "variable", value)
		return false
	}
	// key and value need to be of same type
	switch typedKey := key.(type) {
	case bool:
		return neh.validateValuewithBoolPattern(typedKey, value)
	case int:
		return neh.validateValuewithIntPattern(int64(typedKey), value)
	case int64:
		return neh.validateValuewithIntPattern(typedKey, value)
	case float64:
		return neh.validateValuewithFloatPattern(typedKey, value)
	case string:
		return neh.validateValuewithStringPattern(typedKey, value)
	case map[string]interface{}:
		return neh.validateValueWithMapPattern(typedKey, value)
	case []interface{}:
		return neh.validateValueWithSlicePattern(typedKey, value)
	default:
		neh.log.Info("Unsupported type", "value", typedKey, "type", fmt.Sprintf("%T", typedKey))
		return false
	}
}

func (neh NotInHandler) validateValueWithSlicePattern(key []interface{}, value interface{}) bool {
	if val, ok := value.([]interface{}); ok {
		return !reflect.DeepEqual(key, val)
	}
	neh.log.Info("Expected type []interface{}", "value", value, "type", fmt.Sprintf("%T", value))
	return false
}

func (neh NotInHandler) validateValueWithMapPattern(key map[string]interface{}, value interface{}) bool {
	if val, ok := value.(map[string]interface{}); ok {
		return !reflect.DeepEqual(key, val)
	}
	neh.log.Info("Expected type map[string]interface{}", "value", value, "type", fmt.Sprintf("%T", value))
	return false
}

func (neh NotInHandler) validateValuewithStringPattern(key string, value interface{}) bool {
	if val, ok := value.(string); ok {
		return key != val
	}
	neh.log.Info("Expected type string", "value", value, "type", fmt.Sprintf("%T", value))
	return false
}

func (neh NotInHandler) validateValuewithFloatPattern(key float64, value interface{}) bool {
	switch typedValue := value.(type) {
	case int:
		// check that float has not fraction
		if key == math.Trunc(key) {
			return int(key) != typedValue
		}
		neh.log.Info("Expected type float, found int", "typedValue", typedValue)
	case int64:
		// check that float has not fraction
		if key == math.Trunc(key) {
			return int64(key) != typedValue
		}
		neh.log.Info("Expected type float, found int", "typedValue", typedValue)
	case float64:
		return typedValue != key
	case string:
		// extract float from string
		float64Num, err := strconv.ParseFloat(typedValue, 64)
		if err != nil {
			neh.log.Error(err, "Failed to parse float64 from string")
			return false
		}
		return float64Num != key
	default:
		neh.log.Info("Expected type float", "value", value, "type", fmt.Sprintf("%T", value))
		return false
	}
	return false
}

func (neh NotInHandler) validateValuewithBoolPattern(key bool, value interface{}) bool {
	typedValue, ok := value.(bool)
	if !ok {
		neh.log.Info("Expected type bool", "value", value, "type", fmt.Sprintf("%T", value))
		return false
	}
	return key != typedValue
}

func (neh NotInHandler) validateValuewithIntPattern(key int64, value interface{}) bool {
	switch typedValue := value.(type) {
	case int:
		return int64(typedValue) != key
	case int64:
		return typedValue != key
	case float64:
		// check that float has no fraction
		if typedValue == math.Trunc(typedValue) {
			return int64(typedValue) != key
		}
		neh.log.Info("Expected type int, found float", "value", typedValue, "type", fmt.Sprintf("%T", typedValue))
		return false
	case string:
		// extract in64 from string
		int64Num, err := strconv.ParseInt(typedValue, 10, 64)
		if err != nil {
			neh.log.Error(err, "Failed to parse int64 from string")
			return false
		}
		return int64Num != key
	default:
		neh.log.Info("Expected type int", "value", value, "type", fmt.Sprintf("%T", value))
		return false
	}
}
