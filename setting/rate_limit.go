package setting

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitCountSpec = "0"
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitSuccessCountSpec = "1000"
var ModelRequestRateLimitGroup = map[string][2]string{}
var ModelRequestRateLimitMutex sync.RWMutex

var AwsBedrockRateLimitEnabled = false
var AwsBedrockRateLimitCount = 0
var AwsBedrockRateLimitCountSpec = "0"
var AwsBedrockRateLimitQueueTimeoutSeconds = 30
var AwsBedrockRateLimitQueueMaxSize = 0

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	nextModelRequestRateLimitGroup, err := parseModelRequestRateLimitGroup(jsonStr)
	if err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()

	ModelRequestRateLimitGroup = nextModelRequestRateLimitGroup
	return nil
}

func parseModelRequestRateLimitGroup(jsonStr string) (map[string][2]string, error) {
	var raw map[string][]interface{}
	if err := common.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, err
	}
	nextModelRequestRateLimitGroup := make(map[string][2]string)
	for group, limits := range raw {
		if len(limits) != 2 {
			return nil, fmt.Errorf("group %s must have exactly two rate limit values", group)
		}
		totalSpec, err := normalizeRateLimitSpecValue(limits[0])
		if err != nil {
			return nil, fmt.Errorf("group %s total rate limit %w", group, err)
		}
		successSpec, err := normalizeRateLimitSpecValue(limits[1])
		if err != nil {
			return nil, fmt.Errorf("group %s success rate limit %w", group, err)
		}
		if err := common.ValidateRateLimitSpec("total rate limit", totalSpec, true); err != nil {
			return nil, fmt.Errorf("group %s %w", group, err)
		}
		if err := common.ValidateRateLimitSpec("success rate limit", successSpec, false); err != nil {
			return nil, fmt.Errorf("group %s %w", group, err)
		}
		nextModelRequestRateLimitGroup[group] = [2]string{totalSpec, successSpec}
	}
	return nextModelRequestRateLimitGroup, nil
}

func GetGroupRateLimit(group string) (totalCountSpec, successCountSpec string, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return "", "", false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return "", "", false
	}
	return limits[0], limits[1], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	_, err := parseModelRequestRateLimitGroup(jsonStr)
	return err
}

func CheckPositiveRateLimitValue(name string, raw string) error {
	return common.ValidateRateLimitSpec(name, raw, false)
}

func CheckOptionalRateLimitValue(name string, raw string) error {
	return common.ValidateRateLimitSpec(name, raw, true)
}

func CheckPositiveRateLimitIntegerValue(name string, raw string) error {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	if value < 1 || value > math.MaxInt32 {
		return fmt.Errorf("%s must be between 1 and 2147483647", name)
	}
	return nil
}

func CheckOptionalRateLimitIntegerValue(name string, raw string) error {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s must be a non-negative integer", name)
	}
	if value < 0 || value > math.MaxInt32 {
		return fmt.Errorf("%s must be between 0 and 2147483647", name)
	}
	return nil
}

func normalizeRateLimitSpecValue(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "", fmt.Errorf("must not be empty")
		}
		return strings.TrimSpace(v), nil
	case float64:
		if math.Trunc(v) != v {
			return "", fmt.Errorf("must be an integer or N(mean,std=stddev)")
		}
		if v < 0 || v > math.MaxInt32 {
			return "", fmt.Errorf("must be between 0 and 2147483647")
		}
		return strconv.Itoa(int(v)), nil
	case int:
		return strconv.Itoa(v), nil
	default:
		return "", fmt.Errorf("must be an integer or N(mean,std=stddev)")
	}
}
