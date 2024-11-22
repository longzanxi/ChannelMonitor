package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Channel struct {
	ID        int
	Type      int
	Name      string
	BaseURL   string
	Key       string
	Status    int
	Models    string
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

type Ability struct {
	Group     string `gorm:"column:group"`
	Model     string `gorm:"column:model"`
	ChannelID int64  `gorm:"column:channel_id"`
	Enabled   bool   `gorm:"column:enabled"`
	Priority  int64  `gorm:"column:priority"`
	Weight    uint64 `gorm:"column:weight"`
}

type ChannelConfig struct {
	Priority int64
	Weight   uint64
}

var (
	db     *gorm.DB
	config *Config
)

// 获取渠道默认配置
func getChannelDefaultConfig(channelID int) (ChannelConfig, error) {
	var ability Ability
	result := db.Raw(`
		SELECT priority, weight 
		FROM abilities 
		WHERE channel_id = ? 
		LIMIT 1`, channelID).Scan(&ability)

	if result.Error != nil {
		return ChannelConfig{0, 1}, fmt.Errorf("获取渠道默认配置失败：%v", result.Error)
	}

	// 如果没有找到记录，返回默认值
	if result.RowsAffected == 0 {
		return ChannelConfig{0, 1}, nil
	}

	return ChannelConfig{ability.Priority, ability.Weight}, nil
}

// 删除渠道的所有模型能力
func deleteChannelAbilities(channelID int) error {
	result := db.Exec("DELETE FROM abilities WHERE channel_id = ?", channelID)
	if result.Error != nil {
		return fmt.Errorf("删除渠道能力失败：%v", result.Error)
	}
	log.Printf("已删除渠道 ID:%d 的所有模型能力\n", channelID)
	return nil
}

// 添加渠道的模型能力
func addChannelAbilities(channelID int, models []string, cfg ChannelConfig) error {
	for _, model := range models {
		ability := Ability{
			Group:     "default",
			Model:     model,
			ChannelID: int64(channelID),
			Enabled:   true,
			Priority:  cfg.Priority,
			Weight:    cfg.Weight,
		}

		result := db.Exec(`
			INSERT INTO abilities (`+"`group`"+`, model, channel_id, enabled, priority, weight) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			ability.Group, ability.Model, ability.ChannelID, ability.Enabled,
			ability.Priority, ability.Weight)
		if result.Error != nil {
			return fmt.Errorf("添加模型能力失败：%v", result.Error)
		}
	}
	log.Printf("已为渠道 ID:%d 添加 %d 个模型能力使用配置：priority=%d, weight=%d\n",
		channelID, len(models), cfg.Priority, cfg.Weight)
	return nil
}

// 其他辅助函数保持不变
func contains(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func containsString(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func fetchChannels() ([]Channel, error) {
	query := `
		SELECT id, type, name, base_url, ` + "`key`" + `, status, models 
		FROM channels 
		WHERE deleted_at IS NULL 
		AND type IN (1, 40, 999)`

	rows, err := db.Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询渠道失败：%v", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Type, &c.Name, &c.BaseURL, &c.Key, &c.Status, &c.Models); err != nil {
			return nil, fmt.Errorf("扫描渠道数据失败：%v", err)
		}

		// 检查是否在排除列表中
		if contains(config.ExcludeChannel, c.ID) {
			log.Printf("渠道 %s(ID:%d) 在排除列表中，跳过\n", c.Name, c.ID)
			continue
		}

		// 处理特殊类型渠道的 BaseURL
		if c.Type == 40 || c.Type == 999 {
			c.BaseURL = "https://api.siliconflow.cn"
			channels = append(channels, c)
			continue
		}

		// 处理类型 1 的渠道
		if c.Type == 1 && c.BaseURL != "" {
			channels = append(channels, c)
		} else if c.Type == 1 {
			log.Printf("渠道 %s(ID:%d) 的 base_url 为空，跳过\n", c.Name, c.ID)
		}
	}

	if len(channels) == 0 {
		log.Println("警告：没有找到任何符合条件的渠道")
		return channels, nil
	}

	log.Printf("获取到 %d 个有效渠道\n", len(channels))
	for _, c := range channels {
		log.Printf("- %s (ID:%d, Type:%d)\n", c.Name, c.ID, c.Type)
	}

	return channels, nil
}

// 更新渠道模型和能力
func updateChannelModels(channel Channel, models []string, cfg ChannelConfig) error {
	// 1. 更新 channels 表中的 models 字段
	modelsStr := strings.Join(models, ",")
	if result := db.Exec("UPDATE channels SET models = ? WHERE id = ?", modelsStr, channel.ID); result.Error != nil {
		return fmt.Errorf("更新渠道模型失败：%v", result.Error)
	}

	// 2. 删除旧的能力记录
	if err := deleteChannelAbilities(channel.ID); err != nil {
		return err
	}

	// 3. 添加新的能力记录
	return addChannelAbilities(channel.ID, models, cfg)
}

// 测试渠道模型
func testChannelModels(channel Channel) ([]string, error) {
	var availableModels []string
	modelList := []string{}

	if config.ForceModels {
		log.Println("强制使用自定义模型列表")
		modelList = config.Models
	} else {
		req, err := http.NewRequest("GET", channel.BaseURL+"/v1/models", nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败：%v", err)
		}
		req.Header.Set("Authorization", "Bearer "+channel.Key)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			log.Printf("获取模型列表失败：%v", err)

			// 尝试使用渠道原有的模型列表
			if channel.Models != "" {
				log.Printf("使用渠道原有的模型列表: %s", channel.Models)
				modelList = strings.Split(channel.Models, ",")
			} else {
				log.Println("渠道原有模型列表为空，使用配置文件中的默认模型列表")
				modelList = config.Models
			}
		} else {
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)

			var response struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}

			if err := json.Unmarshal(body, &response); err != nil {
				log.Printf("解析模型列表失败：%v，使用渠道原有的模型列表", err)
				if channel.Models != "" {
					modelList = strings.Split(channel.Models, ",")
				} else {
					modelList = config.Models
				}
			} else {
				for _, model := range response.Data {
					if containsString(config.ExcludeModel, model.ID) {
						log.Printf("模型 %s 在排除列表中，跳过\n", model.ID)
						continue
					}
					modelList = append(modelList, model.ID)
				}
			}
		}
	}

	for _, model := range modelList {
		url := channel.BaseURL
		if !strings.Contains(channel.BaseURL, "/v1/chat/completions") {
			if !strings.HasSuffix(channel.BaseURL, "/chat") {
				if !strings.HasSuffix(channel.BaseURL, "/v1") {
					url += "/v1"
				}
				url += "/chat"
			}
			url += "/completions"
		}

		reqBody := map[string]interface{}{
			"model": model,
			"messages": []map[string]string{
				{"role": "user", "content": "Hello! Reply in short"},
			},
		}
		jsonData, _ := json.Marshal(reqBody)

		log.Printf("测试渠道 %s(ID:%d) 的模型 %s\n", channel.Name, channel.ID, model)

		req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
		if err != nil {
			log.Println("创建请求失败：", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+channel.Key)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("\033[31m请求失败：%v\033[0m\n", err)
			continue
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusOK {
			availableModels = append(availableModels, model)
			log.Printf("\033[32m渠道 %s(ID:%d) 的模型 %s 测试成功\033[0m\n", channel.Name, channel.ID, model)
		} else {
			log.Printf("\033[31m渠道 %s(ID:%d) 的模型 %s 测试失败，状态码：%d，响应：%s\033[0m\n",
				channel.Name, channel.ID, model, resp.StatusCode, string(body))
		}
	}

	return availableModels, nil
}

func main() {
	var err error
	config, err = loadConfig()
	if err != nil {
		log.Fatal("加载配置失败：", err)
	}

	duration, err := time.ParseDuration(config.TimePeriod)
	if err != nil {
		log.Fatal("解析时间周期失败：", err)
	}

	db, err = NewDB(*config)
	if err != nil {
		log.Fatal("数据库连接失败：", err)
	}

	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		log.Println("开始检测...")
		channels, err := fetchChannels()
		if err != nil {
			log.Printf("\033[31m获取渠道失败：%v\033[0m\n", err)
			continue
		}

		for _, channel := range channels {
			log.Printf("开始测试渠道 %s(ID:%d) 的模型\n", channel.Name, channel.ID)

			// 获取渠道配置
			cfg, err := getChannelDefaultConfig(channel.ID)
			if err != nil {
				log.Printf("获取渠道配置失败：%v，使用默认配置", err)
				cfg = ChannelConfig{0, 1}
			}
			log.Printf("渠道 %s(ID:%d) 使用配置：priority=%d, weight=%d\n",
				channel.Name, channel.ID, cfg.Priority, cfg.Weight)

			// 测试模型
			models, err := testChannelModels(channel)
			if err != nil {
				log.Printf("\033[31m渠道 %s(ID:%d) 测试模型失败：%v\033[0m\n",
					channel.Name, channel.ID, err)
				continue
			}

			// 更新数据库
			if err := updateChannelModels(channel, models, cfg); err != nil {
				log.Printf("\033[31m更新渠道 %s(ID:%d) 的模型失败：%v\033[0m\n",
					channel.Name, channel.ID, err)
			} else {
				log.Printf("渠道 %s(ID:%d) 可用模型：%v\n", channel.Name, channel.ID, models)
			}
		}

		<-ticker.C
	}
}
