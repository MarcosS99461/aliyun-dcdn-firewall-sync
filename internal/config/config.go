package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config 应用配置
type Config struct {
	DCDN      DCDNConfig      `yaml:"dcdn"`
	Firewall  FirewallConfig  `yaml:"firewall"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Sync      SyncConfig      `yaml:"sync"`
	Logging   LogConfig       `yaml:"logging"`
}

// AliyunConfig 阿里云基础配置
type AliyunConfig struct {
	AccessKeyId     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	Region          string `yaml:"region"`
}

// DCDNConfig DCDN配置
type DCDNConfig struct {
	AliyunConfig `yaml:",inline"` // 内嵌阿里云配置
	// 移除Domains字段，新SDK直接获取全部L2节点IP，无需指定域名
}

// FirewallConfig 防火墙配置
type FirewallConfig struct {
	AliyunConfig `yaml:",inline"` // 内嵌阿里云配置
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	Cron       string `yaml:"cron"`         // cron表达式，优先级高于Interval
	Interval   string `yaml:"interval"`     // 执行间隔，如 "168h"（当cron为空时使用）
	RunOnStart bool   `yaml:"run_on_start"` // 启动时是否立即执行
	Timeout    string `yaml:"timeout"`      // 超时时间
	MaxRetries int    `yaml:"max_retries"`  // 最大重试次数
}

// SyncConfig 同步配置
type SyncConfig struct {
	AddressGroups []AddressGroup `yaml:"address_groups"`
}

// AddressGroup 地址组配置
type AddressGroup struct {
	GroupName       string   `yaml:"group_name"`
	Description     string   `yaml:"description"`
	IPType          string   `yaml:"ip_type"`          // 支持的IP类型: "ipv4", "ipv6", "both" (默认)
	IncludePatterns []string `yaml:"include_patterns"`
	ExcludePatterns []string `yaml:"exclude_patterns"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level    string `yaml:"level"`     // debug, info, warn, error
	Format   string `yaml:"format"`    // json, text
	FilePath string `yaml:"file_path"` // 日志文件路径
}

// LoadConfig 从文件加载配置
func LoadConfig(filePath string) (*Config, error) {
	// 如果没有指定文件路径，使用默认路径
	if filePath == "" {
		filePath = "configs/config.yaml"
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	setDefaults(&config)

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &config, nil
}

// setDefaults 设置默认配置值
func setDefaults(config *Config) {
	if config.Scheduler.Interval == "" {
		config.Scheduler.Interval = "168h" // 每周一次
	}
	if config.Scheduler.Timeout == "" {
		config.Scheduler.Timeout = "30m"
	}
	if config.Scheduler.MaxRetries == 0 {
		config.Scheduler.MaxRetries = 3
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "text"
	}
	
	// 为DCDN设置默认区域
	if config.DCDN.Region == "" {
		config.DCDN.Region = "ap-southeast-1" // 新加坡区域
	}
	
	// 为防火墙设置默认区域
	if config.Firewall.Region == "" {
		config.Firewall.Region = "ap-southeast-1" // 新加坡区域
	}
}

// validateConfig 验证配置
func validateConfig(config *Config) error {
	// 不再强制要求AK/SK，支持更安全的凭证管理方式
	// 如果配置文件中没有提供，SDK将自动使用以下顺序查找凭证：
	// 1. 环境变量 (ALIBABA_CLOUD_ACCESS_KEY_ID, ALIBABA_CLOUD_ACCESS_KEY_SECRET)
	// 2. 配置文件 (~/.alibabacloud/credentials)
	// 3. 实例RAM角色
	// 4. ECS实例元数据服务 (IMDS)
	
	// 移除DCDN域名验证，新SDK无需指定域名
	if len(config.Sync.AddressGroups) == 0 {
		return fmt.Errorf("防火墙地址组列表不能为空")
	}
	return nil
}

// GetEnvOrDefault 获取环境变量值，如果不存在则返回默认值
func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
