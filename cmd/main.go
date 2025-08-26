package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"aliyun-dcdn-firewall-sync/internal/client"
	"aliyun-dcdn-firewall-sync/internal/config"
	"aliyun-dcdn-firewall-sync/internal/scheduler"
	"aliyun-dcdn-firewall-sync/pkg/models"
)

var (
	configFile = flag.String("config", "configs/config.yaml", "配置文件路径")
	onceMode   = flag.Bool("once", false, "执行一次后退出，不启动调度器")
	genConfig  = flag.Bool("gen-config", false, "生成示例配置文件")
	version    = flag.Bool("version", false, "显示版本信息")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Println("Aliyun DCDN Firewall Sync v1.0.0")
		return
	}

	if *genConfig {
		if err := generateSampleConfig(*configFile); err != nil {
			log.Fatal("生成示例配置文件失败:", err)
		}
		fmt.Printf("示例配置文件已生成: %s\n", *configFile)
		return
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		// 如果配置文件不存在，自动生成一个示例配置
		if os.IsNotExist(err) {
			fmt.Printf("配置文件不存在，正在生成示例配置文件: %s\n", *configFile)
			if genErr := generateSampleConfig(*configFile); genErr != nil {
				log.Fatal("生成示例配置文件失败:", genErr)
			}
			fmt.Printf("请编辑配置文件 %s 后重新运行程序\n", *configFile)
			return
		}
		log.Fatal("加载配置文件失败:", err)
	}

	// 创建客户端
	dcdnClient := client.NewDCDNClient(&cfg.DCDN)
	firewallClient := client.NewFirewallClient(&cfg.Firewall, &cfg.Sync)

	if *onceMode {
		// 执行一次同步
		fmt.Println("执行一次性同步...")
		if err := performSync(dcdnClient, firewallClient, cfg); err != nil {
			log.Fatal("同步失败:", err)
		}
		fmt.Println("同步完成")
		return
	}

	// 启动调度器
	fmt.Println("启动调度器...")
	scheduler := scheduler.NewScheduler(cfg)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("接收到停止信号，正在优雅关闭...")
		scheduler.Stop()
	}()

	// 启动调度器
	if err := scheduler.Start(); err != nil {
		log.Fatal("调度器启动失败:", err)
	}

	fmt.Println("程序已退出")
}

// performSync 执行一次同步操作
func performSync(dcdnClient *client.DCDNClient, firewallClient *client.FirewallClient, cfg *config.Config) error {
	// 获取DCDN源IP信息
	fmt.Println("查询DCDN L2节点IP信息...")
	sourceIPs, err := dcdnClient.GetL2IPList() // 新SDK不需要域名列表
	if err != nil {
		return fmt.Errorf("查询DCDN源IP失败: %w", err)
	}

	fmt.Printf("共获取到 %d 个源IP地址\n", len(sourceIPs))

	// 同步到防火墙地址组
	for _, addressGroup := range cfg.Sync.AddressGroups {
		fmt.Printf("同步到地址组: %s\n", addressGroup.GroupName)

		// 过滤IP地址
		filteredIPs := filterSourceIPs(sourceIPs, addressGroup.IncludePatterns, addressGroup.ExcludePatterns)

		fmt.Printf("过滤后的IP数量: %d\n", len(filteredIPs))

		// 同步到防火墙
		if err := firewallClient.SyncAddressBook(addressGroup.GroupName, filteredIPs); err != nil {
			return fmt.Errorf("同步地址组 %s 失败: %w", addressGroup.GroupName, err)
		}

		fmt.Printf("地址组 %s 同步完成\n", addressGroup.GroupName)
	}

	return nil
}

// filterSourceIPs 根据包含和排除模式过滤源IP信息
func filterSourceIPs(sourceIPs []*models.DCDNSourceIPInfo, includePatterns, excludePatterns []string) []*models.DCDNSourceIPInfo {
	if len(includePatterns) == 0 && len(excludePatterns) == 0 {
		return sourceIPs
	}

	var result []*models.DCDNSourceIPInfo
	for _, sourceIP := range sourceIPs {
		included := len(includePatterns) == 0 // 如果没有包含模式，默认包含所有
		excluded := false

		// 检查包含模式
		if len(includePatterns) > 0 {
			for _, pattern := range includePatterns {
				if matchPattern(sourceIP.IP, pattern) {
					included = true
					break
				}
			}
		}

		// 检查排除模式
		for _, pattern := range excludePatterns {
			if matchPattern(sourceIP.IP, pattern) {
				excluded = true
				break
			}
		}

		if included && !excluded {
			result = append(result, sourceIP)
		}
	}

	return result
}

// matchPattern 简单的模式匹配（支持通配符*）
func matchPattern(text, pattern string) bool {
	// 简单实现，仅支持前缀和后缀通配符
	if pattern == "*" {
		return true
	}

	if len(pattern) == 0 {
		return len(text) == 0
	}

	if pattern[0] == '*' {
		// 后缀匹配
		suffix := pattern[1:]
		return len(text) >= len(suffix) && text[len(text)-len(suffix):] == suffix
	}

	if pattern[len(pattern)-1] == '*' {
		// 前缀匹配
		prefix := pattern[:len(pattern)-1]
		return len(text) >= len(prefix) && text[:len(prefix)] == prefix
	}

	// 完全匹配
	return text == pattern
}

// generateSampleConfig 生成示例配置文件
func generateSampleConfig(filePath string) error {
	sampleConfig := `# Aliyun DCDN Firewall Sync Configuration

# DCDN配置 - 获取L2节点IP的凭证（建议使用只读权限用户）
dcdn:
  # 可选：配置文件中的AK/SK（不推荐，建议使用环境变量）
  # access_key_id: "DCDN_USER_ACCESS_KEY_ID"
  # access_key_secret: "DCDN_USER_ACCESS_KEY_SECRET"
  
  # 推荐：使用环境变量（为DCDN用户单独设置）
  # export ALIBABA_CLOUD_ACCESS_KEY_ID=dcdn_user_access_key_id
  # export ALIBABA_CLOUD_ACCESS_KEY_SECRET=dcdn_user_access_key_secret
  
  region: "ap-southeast-1"  # 新加坡区域

# 防火墙配置 - 更新防火墙地址簿的凭证（需要防火墙管理权限）
firewall:
  # 可选：配置文件中的AK/SK（不推荐，建议使用环境变量）
  # access_key_id: "FIREWALL_USER_ACCESS_KEY_ID"
  # access_key_secret: "FIREWALL_USER_ACCESS_KEY_SECRET"
  
  # 推荐：使用环境变量（为防火墙用户单独设置）
  # 可以通过前缀区分不同用户：
  # export DCDN_ALIBABA_CLOUD_ACCESS_KEY_ID=dcdn_user_key
  # export DCDN_ALIBABA_CLOUD_ACCESS_KEY_SECRET=dcdn_user_secret
  # export FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_ID=firewall_user_key
  # export FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_SECRET=firewall_user_secret
  
  region: "ap-southeast-1"  # 新加坡区域

scheduler:
  # 优先使用cron表达式（支持秒级精度）
  cron: "0 0 2 * * 0,3"   # 每周日和周三凌晨2点执行
  # 备用：传统间隔调度（当cron为空时使用）
  interval: "168h"        # 每周执行一次 (168小时)
  run_on_start: true      # 启动时是否立即执行一次
  timeout: "30m"          # 超时时间
  max_retries: 3          # 最大重试次数

sync:
  address_groups:
    # IPv4地址组
    - group_name: "dcdn-source-ips-v4"
      description: "DCDN源IPv4地址组"
      ip_type: "ipv4"      # 仅IPv4地址
      include_patterns:
        - "*"             # 包含所有IPv4
      exclude_patterns:   # 排除某些IP模式
        - "127.*"          # 本地回环
        - "192.168.*"      # 私有网络
        - "10.*"           # 私有网络
        - "172.16.*"       # 私有网络
    
    # IPv6地址组
    - group_name: "dcdn-source-ips-v6"
      description: "DCDN源IPv6地址组"
      ip_type: "ipv6"      # 仅IPv6地址
      include_patterns:
        - "*"             # 包含所有IPv6
      exclude_patterns:
        - "::1"           # IPv6本地回环
        - "fc00::*"       # 私有IPv6

logging:
  level: "info"           # debug, info, warn, error
  format: "text"          # text, json
  file_path: "logs/sync.log"
`

	// 创建目录
	if err := os.MkdirAll("configs", 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 写入配置文件
	if err := os.WriteFile(filePath, []byte(sampleConfig), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}
