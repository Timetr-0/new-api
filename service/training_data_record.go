package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	trainingDataRecordContextKey = "training_data_record_state"
	trainingDataRecordBaseDir    = "/data/training-capture"
	trainingDataRecordQueueSize  = 4096
	trainingDataRecordBatchSize  = 256
)

var (
	trainingDataRecordQueue     chan trainingDataRecordEntry
	trainingDataRecordStartOnce sync.Once
)

type trainingDataRecordState struct {
	requestBody  []byte
	responseBody []byte
	streamEvents []json.RawMessage
}

type trainingDataRecordEntry struct {
	capturedAt time.Time

	Request      json.RawMessage   `json:"request"`
	Response     json.RawMessage   `json:"response,omitempty"`
	StreamEvents []json.RawMessage `json:"stream_events,omitempty"`
}

func SetTrainingDataRecordRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody []byte) {
	if !shouldCaptureTrainingData(c, info, requestBody) {
		return
	}
	state := getOrCreateTrainingDataRecordState(c)
	if state == nil {
		return
	}
	state.requestBody = append(state.requestBody[:0], requestBody...)
}

func SetTrainingDataRecordResponse(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte) {
	if !shouldCaptureTrainingData(c, info, responseBody) {
		return
	}
	state := getTrainingDataRecordState(c)
	if state == nil {
		return
	}
	state.responseBody = append(state.responseBody[:0], responseBody...)
}

func AppendTrainingDataRecordStreamEvent(c *gin.Context, info *relaycommon.RelayInfo, eventBody []byte) {
	if !shouldCaptureTrainingData(c, info, eventBody) {
		return
	}
	state := getTrainingDataRecordState(c)
	if state == nil {
		return
	}
	event := json.RawMessage(append([]byte(nil), eventBody...))
	state.streamEvents = append(state.streamEvents, event)
}

func EnqueueTrainingDataRecord(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil || !common.TrainingDataRecordEnabled {
		return
	}
	state := getTrainingDataRecordState(c)
	if state == nil || len(state.requestBody) == 0 {
		return
	}
	if len(state.responseBody) == 0 && len(state.streamEvents) == 0 {
		return
	}
	if !shouldCaptureTrainingData(c, info, state.requestBody) {
		return
	}

	entry := buildTrainingDataRecordEntry(state)
	startTrainingDataRecordWorker()
	select {
	case trainingDataRecordQueue <- entry:
	default:
		logger.LogWarn(c, "training data record queue is full, dropping capture entry")
	}
}

func getOrCreateTrainingDataRecordState(c *gin.Context) *trainingDataRecordState {
	if c == nil {
		return nil
	}
	if state := getTrainingDataRecordState(c); state != nil {
		return state
	}
	state := &trainingDataRecordState{}
	c.Set(trainingDataRecordContextKey, state)
	return state
}

func getTrainingDataRecordState(c *gin.Context) *trainingDataRecordState {
	if c == nil {
		return nil
	}
	value, ok := c.Get(trainingDataRecordContextKey)
	if !ok {
		return nil
	}
	state, ok := value.(*trainingDataRecordState)
	if !ok {
		return nil
	}
	return state
}

func shouldCaptureTrainingData(c *gin.Context, info *relaycommon.RelayInfo, body []byte) bool {
	if c == nil || info == nil || !common.TrainingDataRecordEnabled || info.IsChannelTest || len(body) == 0 {
		return false
	}
	if !gjson.ValidBytes(body) {
		return false
	}
	if info.GetFinalRequestRelayFormat() == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatClaude {
		return true
	}
	if isClaudeModelName(info.OriginModelName) || isClaudeModelName(info.UpstreamModelName) {
		return true
	}
	return isClaudeModelName(gjson.GetBytes(body, "model").String())
}

func isClaudeModelName(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "claude-") || strings.Contains(model, ".claude-")
}

func buildTrainingDataRecordEntry(state *trainingDataRecordState) trainingDataRecordEntry {
	now := time.Now()

	entry := trainingDataRecordEntry{
		capturedAt: now,
		Request:    json.RawMessage(append([]byte(nil), state.requestBody...)),
	}
	if len(state.responseBody) > 0 {
		entry.Response = json.RawMessage(append([]byte(nil), state.responseBody...))
	}
	if len(state.streamEvents) > 0 {
		entry.StreamEvents = append([]json.RawMessage(nil), state.streamEvents...)
	}
	return entry
}

func startTrainingDataRecordWorker() {
	trainingDataRecordStartOnce.Do(func() {
		trainingDataRecordQueue = make(chan trainingDataRecordEntry, trainingDataRecordQueueSize)
		go trainingDataRecordWorker()
	})
}

func trainingDataRecordWorker() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	batch := make([]trainingDataRecordEntry, 0, trainingDataRecordBatchSize)
	for {
		select {
		case entry := <-trainingDataRecordQueue:
			batch = append(batch, entry)
			if len(batch) >= trainingDataRecordBatchSize {
				writeTrainingDataRecordBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				writeTrainingDataRecordBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func writeTrainingDataRecordBatch(batch []trainingDataRecordEntry) {
	entriesByPath := make(map[string][]trainingDataRecordEntry)
	for _, entry := range batch {
		path := trainingDataRecordPath(entry.capturedAt)
		entriesByPath[path] = append(entriesByPath[path], entry)
	}
	for path, entries := range entriesByPath {
		if err := appendTrainingDataRecordEntries(path, entries); err != nil {
			common.SysError("append training data record failed: " + err.Error())
		}
	}
}

func trainingDataRecordPath(t time.Time) string {
	local := t.Local()
	windowStart := local.Hour() / 6 * 6
	windowEnd := windowStart + 5
	fileName := fmt.Sprintf("claude-%02d-%02d.jsonl", windowStart, windowEnd)
	return filepath.Join(trainingDataRecordBaseDir, local.Format("2006-01-02"), fileName)
}

func appendTrainingDataRecordEntries(path string, entries []trainingDataRecordEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	buffered := bufio.NewWriter(file)
	for _, entry := range entries {
		line, err := common.Marshal(entry)
		if err != nil {
			return err
		}
		if _, err := buffered.Write(line); err != nil {
			return err
		}
		if err := buffered.WriteByte('\n'); err != nil {
			return err
		}
	}
	return buffered.Flush()
}
