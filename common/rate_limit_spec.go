package common

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RateLimitSpec struct {
	Raw     string
	Fixed   int
	Mean    float64
	StdDev  float64
	Dynamic bool
}

type dynamicRateLimitSample struct {
	minute int64
	value  int
}

var dynamicRateLimitSamples = struct {
	sync.Mutex
	entries map[string]dynamicRateLimitSample
}{
	entries: map[string]dynamicRateLimitSample{},
}

func ValidateRateLimitSpec(name string, raw string, allowZero bool) error {
	_, err := ParseRateLimitSpec(raw, allowZero)
	if err != nil {
		return fmt.Errorf("%s %w", name, err)
	}
	return nil
}

func ParseRateLimitSpec(raw string, allowZero bool) (RateLimitSpec, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return RateLimitSpec{}, fmt.Errorf("must not be empty")
	}
	if value, err := strconv.Atoi(trimmed); err == nil {
		if value < 0 || (!allowZero && value == 0) || value > math.MaxInt32 {
			if allowZero {
				return RateLimitSpec{}, fmt.Errorf("must be between 0 and 2147483647")
			}
			return RateLimitSpec{}, fmt.Errorf("must be between 1 and 2147483647")
		}
		return RateLimitSpec{Raw: trimmed, Fixed: value}, nil
	}

	if !strings.HasPrefix(strings.ToUpper(trimmed), "N(") || !strings.HasSuffix(trimmed, ")") {
		return RateLimitSpec{}, fmt.Errorf("must be a positive integer or N(mean,std=stddev)")
	}

	body := strings.TrimSpace(trimmed[2 : len(trimmed)-1])
	if body == "" {
		return RateLimitSpec{}, fmt.Errorf("normal distribution spec is empty")
	}

	var mean float64
	var stdDev float64
	hasMean := false
	hasStdDev := false
	for index, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := ""
		valueText := part
		if equalIndex := strings.Index(part, "="); equalIndex >= 0 {
			key = strings.ToLower(strings.TrimSpace(part[:equalIndex]))
			valueText = strings.TrimSpace(part[equalIndex+1:])
		}
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil {
			return RateLimitSpec{}, fmt.Errorf("has invalid normal distribution value %q", valueText)
		}
		switch {
		case key == "mean" || key == "mu":
			mean = value
			hasMean = true
		case key == "std" || key == "stddev" || key == "sigma":
			stdDev = value
			hasStdDev = true
		case key == "" && index == 0:
			mean = value
			hasMean = true
		case key == "" && index == 1:
			stdDev = value
			hasStdDev = true
		default:
			return RateLimitSpec{}, fmt.Errorf("has unsupported normal distribution field %q", key)
		}
	}
	if !hasMean {
		return RateLimitSpec{}, fmt.Errorf("normal distribution mean is required")
	}
	if !hasStdDev {
		return RateLimitSpec{}, fmt.Errorf("normal distribution std is required")
	}
	if mean < 1 || mean > math.MaxInt32 {
		return RateLimitSpec{}, fmt.Errorf("normal distribution mean must be between 1 and 2147483647")
	}
	if stdDev < 0 || stdDev > math.MaxInt32 {
		return RateLimitSpec{}, fmt.Errorf("normal distribution std must be between 0 and 2147483647")
	}

	return RateLimitSpec{
		Raw:     trimmed,
		Mean:    mean,
		StdDev:  stdDev,
		Dynamic: true,
	}, nil
}

func ResolveRateLimitSpec(scope string, raw string, allowZero bool) (int, error) {
	spec, err := ParseRateLimitSpec(raw, allowZero)
	if err != nil {
		return 0, err
	}
	if !spec.Dynamic {
		return spec.Fixed, nil
	}

	nowMinute := time.Now().Unix() / 60
	cacheKey := scope + "\x00" + spec.Raw

	dynamicRateLimitSamples.Lock()
	defer dynamicRateLimitSamples.Unlock()

	if sample, ok := dynamicRateLimitSamples.entries[cacheKey]; ok && sample.minute == nowMinute {
		return sample.value, nil
	}

	value := sampleNormalRateLimit(scope, spec, nowMinute)
	dynamicRateLimitSamples.entries[cacheKey] = dynamicRateLimitSample{
		minute: nowMinute,
		value:  value,
	}
	for key, sample := range dynamicRateLimitSamples.entries {
		if sample.minute < nowMinute-10 {
			delete(dynamicRateLimitSamples.entries, key)
		}
	}
	return value, nil
}

func sampleNormalRateLimit(scope string, spec RateLimitSpec, minute int64) int {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(scope))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(spec.Raw))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(strconv.FormatInt(minute, 10)))
	rng := rand.New(rand.NewSource(int64(hasher.Sum64())))
	value := int(math.Round(rng.NormFloat64()*spec.StdDev + spec.Mean))
	lowerBound, upperBound := normalRateLimitBounds(spec.Mean)
	if value < lowerBound {
		return lowerBound
	}
	if value > upperBound {
		return upperBound
	}
	return value
}

func normalRateLimitBounds(mean float64) (int, int) {
	lowerBound := int(math.Round(mean * 0.4))
	if lowerBound < 1 {
		lowerBound = 1
	}

	upperBound := math.MaxInt32
	if upperValue := mean * 1.6; upperValue < math.MaxInt32 {
		upperBound = int(math.Round(upperValue))
	}
	if upperBound < lowerBound {
		upperBound = lowerBound
	}
	return lowerBound, upperBound
}

func RateLimitSpecBaseValue(raw string, fallback int) int {
	spec, err := ParseRateLimitSpec(raw, true)
	if err != nil {
		return fallback
	}
	if spec.Dynamic {
		return int(math.Round(spec.Mean))
	}
	return spec.Fixed
}

func GetRateLimitSpecEnvOrDefault(env string, defaultValue string, allowZero bool) string {
	value := GetEnvOrDefaultString(env, defaultValue)
	if err := ValidateRateLimitSpec(env, value, allowZero); err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %s", env, err.Error(), defaultValue))
		return defaultValue
	}
	return value
}
