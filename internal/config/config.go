package config

import (
	"fmt"
)

// Config 存储图床工具的所有配置信息
type Config struct {
	User   string // 图床用户 (例如 delpub)
	URL    string // 最终的文件公共下载 URL (例如 https://img.bsay.de)
	Token  string // B2 Token
	Bucket string // B2 Bucket 名称
}

// NewConfig 构造配置结构，并检查关键字段是否设置
func NewConfig(user, url, token, bucket string) (*Config, error) {

	cfg := &Config{
		User:   user,
		URL:    url,
		Token:  token,
		Bucket: bucket,
	}

	// 检查 Token
	if cfg.Token == "" {
		return nil, fmt.Errorf("错误: 图床Token未设置. 请确保在 b2upload.toml 文件根部设置 token 字段")
	}

	// 检查 Bucket
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("错误: 图床Bucket未设置. 请确保在 b2upload.toml 文件根部设置 bucket 字段")
	}

	// 检查 User 和 URL (来自配置标签或 base_url 回退)
	if cfg.User == "" {
		return nil, fmt.Errorf("错误: 缺少配置标签的用户(username)信息")
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("错误: 缺少配置标签的URL信息。请检查 b2upload.toml 中标签下的 url 字段或全局 base_url 字段")
	}

	return cfg, nil
}
