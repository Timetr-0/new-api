package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetModelAPIExamples(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" || len(modelName) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model name"})
		return
	}
	examples, err := model.GetModelAPIExamples(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, examples)
}

func UpdateModelAPIExample(c *gin.Context) {
	var example model.ModelAPIExample
	if err := c.ShouldBindJSON(&example); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid code sample"})
		return
	}
	if err := model.UpdateModelAPIExample(example); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
