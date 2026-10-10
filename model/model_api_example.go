package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm/clause"
)

// ModelAPIExample stores display-only code independently of model metadata and routing.
type ModelAPIExample struct {
	Id           int    `json:"id"`
	ModelName    string `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_api_example,priority:1"`
	EndpointType string `json:"endpoint_type" gorm:"size:32;not null;uniqueIndex:uk_model_api_example,priority:2"`
	Language     string `json:"language" gorm:"size:16;not null;uniqueIndex:uk_model_api_example,priority:3"`
	Code         string `json:"code" gorm:"type:text;not null"`
}

func GetModelAPIExamples(modelName string) (map[string]map[string]string, error) {
	var rows []ModelAPIExample
	if err := DB.Where("model_name = ?", modelName).Find(&rows).Error; err != nil {
		return nil, err
	}
	examples := make(map[string]map[string]string)
	for _, row := range rows {
		if examples[row.EndpointType] == nil {
			examples[row.EndpointType] = make(map[string]string)
		}
		examples[row.EndpointType][row.Language] = row.Code
	}
	return examples, nil
}

func UpdateModelAPIExample(example ModelAPIExample) error {
	example.ModelName = strings.TrimSpace(example.ModelName)
	example.EndpointType = strings.TrimSpace(example.EndpointType)
	if example.ModelName == "" || len(example.ModelName) > 128 {
		return fmt.Errorf("model name must contain between 1 and 128 bytes")
	}
	if example.EndpointType == "" || len(example.EndpointType) > 32 {
		return fmt.Errorf("endpoint type must contain between 1 and 32 bytes")
	}
	switch example.Language {
	case "curl", "python", "typescript", "javascript":
	default:
		return fmt.Errorf("unsupported code sample language")
	}
	// MySQL TEXT holds at most 65535 bytes; leave room below that across dialects.
	if len(example.Code) > 60*1024 {
		return fmt.Errorf("code sample must not exceed 60 KiB")
	}
	example.Id = 0
	if strings.TrimSpace(example.Code) == "" {
		return DB.Where("model_name = ? AND endpoint_type = ? AND language = ?",
			example.ModelName, example.EndpointType, example.Language).Delete(&ModelAPIExample{}).Error
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "model_name"}, {Name: "endpoint_type"}, {Name: "language"}},
		DoUpdates: clause.AssignmentColumns([]string{"code"}),
	}).Create(&example).Error
}
