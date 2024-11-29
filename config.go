package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
    ExcludeChannel []int      `json:"exclude_channel"`
    ExcludeModel   []string   `json:"exclude_model"`
    Models         []string   `json:"models"`
    ForceModels    bool       `json:"force_models"`
    TimePeriod     string     `json:"time_period"`
    DbType         string     `json:"db_type"`
    DbDsn          string     `json:"db_dsn"`
}

func loadConfig() (*Config, error) {
    file, err := os.ReadFile("config.json")
    if err != nil {
        return nil, fmt.Errorf("读取配置文件失败: %v", err)
    }

    var config Config
    if err := json.Unmarshal(file, &config); err != nil {
        return nil, fmt.Errorf("解析配置文件失败: %v", err)
    }

    return &config, nil
}
