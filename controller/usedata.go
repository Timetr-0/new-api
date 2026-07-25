package controller

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

var userUsageExportFieldLabels = map[string]string{
	"user_id":    "user_id",
	"username":   "username",
	"model_name": "model_name",
	"use_group":  "use_group",
	"requests":   "requests",
	"quota":      "quota",
	"cost_usd":   "cost_usd",
	"tokens":     "tokens",
}

func parseFlowQuotaTimeRange(c *gin.Context) (int64, int64, bool) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return 0, 0, false
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return 0, 0, false
	}
	if endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return 0, 0, false
	}
	return startTimestamp, endTimestamp, true
}

func parseUserUsageExportFields(fields string) []string {
	defaultFields := []string{"user_id", "username", "model_name", "cost_usd", "tokens"}
	if strings.TrimSpace(fields) == "" {
		return defaultFields
	}
	seen := make(map[string]bool)
	selected := make([]string, 0)
	for _, field := range strings.Split(fields, ",") {
		field = strings.TrimSpace(field)
		if _, ok := userUsageExportFieldLabels[field]; ok && !seen[field] {
			selected = append(selected, field)
			seen[field] = true
		}
	}
	if len(selected) == 0 {
		return defaultFields
	}
	return selected
}

func writeUserUsageExportValue(record []string, field string, data *model.UserUsageExportData) []string {
	switch field {
	case "user_id":
		return append(record, strconv.Itoa(data.UserID))
	case "username":
		return append(record, data.Username)
	case "model_name":
		return append(record, data.ModelName)
	case "use_group":
		return append(record, data.UseGroup)
	case "requests":
		return append(record, strconv.FormatInt(data.Count, 10))
	case "quota":
		return append(record, strconv.FormatInt(data.Quota, 10))
	case "cost_usd":
		return append(record, fmt.Sprintf("%.2f", float64(data.Quota)/common.QuotaPerUnit))
	case "tokens":
		return append(record, strconv.FormatInt(data.TokenUsed, 10))
	default:
		return record
	}
}

func ExportUserUsageData(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if endTimestamp <= startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return
	}
	fields := parseUserUsageExportFields(c.Query("fields"))
	includeUseGroup := false
	for _, field := range fields {
		if field == "use_group" {
			includeUseGroup = true
			break
		}
	}
	usageData, err := model.GetUserUsageExportData(startTimestamp, endTimestamp, includeUseGroup)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	filename := fmt.Sprintf("user_usage_%s.csv", time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF"))

	writer := csv.NewWriter(c.Writer)
	header := make([]string, 0, len(fields))
	for _, field := range fields {
		header = append(header, userUsageExportFieldLabels[field])
	}
	if err := writer.Write(header); err != nil {
		return
	}
	for _, data := range usageData {
		record := make([]string, 0, len(fields))
		for _, field := range fields {
			record = writeUserUsageExportValue(record, field, data)
		}
		if err := writer.Write(record); err != nil {
			return
		}
	}
	writer.Flush()
}

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetQuotaDatesByUser(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	dates, err := model.GetQuotaDataGroupByUser(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetAllFlowQuotaDates(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	username := c.Query("username")
	dates, err := model.GetFlowQuotaData(startTimestamp, endTimestamp, username, 0, c.GetInt("role"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserFlowQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetFlowQuotaData(startTimestamp, endTimestamp, "", userId, common.RoleCommonUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}
