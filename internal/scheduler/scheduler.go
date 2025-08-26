package scheduler

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"aliyun-dcdn-firewall-sync/internal/client"
	"aliyun-dcdn-firewall-sync/internal/config"
	"aliyun-dcdn-firewall-sync/pkg/models"

	"github.com/robfig/cron/v3"
)

// Scheduler 定时调度器
type Scheduler struct {
	config         *config.Config
	dcdnClient     *client.DCDNClient
	firewallClient *client.FirewallClient
	stopCh         chan struct{}
	ctx            context.Context
	cancelFunc     context.CancelFunc
	cron           *cron.Cron
}

// NewScheduler 创建新的调度器
func NewScheduler(cfg *config.Config) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		config:         cfg,
		dcdnClient:     client.NewDCDNClient(&cfg.DCDN),
		firewallClient: client.NewFirewallClient(&cfg.Firewall, &cfg.Sync),
		stopCh:         make(chan struct{}),
		ctx:            ctx,
		cancelFunc:     cancel,
	}
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	// 如果启用了立即执行，先执行一次
	if s.config.Scheduler.RunOnStart {
		log.Println("执行初始同步任务...")
		if err := s.executeSyncTask(); err != nil {
			log.Printf("初始同步任务失败: %v", err)
		}
	}

	// 优先使用cron表达式
	if s.config.Scheduler.Cron != "" {
		return s.startWithCron()
	}

	// 否则使用传统的间隔调度
	return s.startWithInterval()
}

// startWithCron 使用cron表达式启动调度器
func (s *Scheduler) startWithCron() error {
	log.Printf("启动cron调度器，cron表达式: %s", s.config.Scheduler.Cron)

	// 创建cron调度器
	s.cron = cron.New(cron.WithSeconds()) // 支持秒级的cron表达式

	// 添加任务
	_, err := s.cron.AddFunc(s.config.Scheduler.Cron, func() {
		log.Println("开始执行定时同步任务...")
		if err := s.executeSyncTask(); err != nil {
			log.Printf("同步任务执行失败: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("添加cron任务失败: %v", err)
	}

	// 启动cron调度器
	s.cron.Start()
	log.Printf("cron调度器已启动")

	// 等待停止信号
	select {
	case <-s.ctx.Done():
		log.Println("调度器收到停止信号")
	case <-s.stopCh:
		log.Println("调度器已停止")
	}

	return nil
}

// startWithInterval 使用传统间隔启动调度器
func (s *Scheduler) startWithInterval() error {
	log.Printf("启动间隔调度器，执行间隔：%s", s.config.Scheduler.Interval)

	// 解析执行间隔
	interval, err := time.ParseDuration(s.config.Scheduler.Interval)
	if err != nil {
		return fmt.Errorf("解析执行间隔失败: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("调度器已启动，下次执行时间：%s", time.Now().Add(interval).Format("2006-01-02 15:04:05"))

	for {
		select {
		case <-ticker.C:
			log.Printf("开始执行定时同步任务...")
			if err := s.executeSyncTask(); err != nil {
				log.Printf("同步任务执行失败: %v", err)
			}
			log.Printf("下次执行时间：%s", time.Now().Add(interval).Format("2006-01-02 15:04:05"))

		case <-s.ctx.Done():
			log.Println("调度器收到停止信号")
			return nil

		case <-s.stopCh:
			log.Println("调度器已停止")
			return nil
		}
	}
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	log.Println("正在停止调度器...")
	s.cancelFunc()
	if s.cron != nil {
		s.cron.Stop()
		log.Println("cron调度器已停止")
	}
	close(s.stopCh)
}

// executeSyncTask 执行同步任务
func (s *Scheduler) executeSyncTask() error {
	startTime := time.Now()
	log.Printf("=== 开始执行同步任务 [%s] ===", startTime.Format("2006-01-02 15:04:05"))

	// 创建同步任务记录
	task := &models.SyncTask{
		TaskId:    fmt.Sprintf("sync_%d", startTime.Unix()),
		Status:    "running",
		StartTime: startTime,
		SourceIPs: []string{},
		AddedIPs:  []string{},
	}

	defer func() {
		task.EndTime = time.Now()
		duration := task.EndTime.Sub(task.StartTime)

		if task.Status == "running" {
			task.Status = "completed"
		}

		log.Printf("=== 同步任务完成 [%s] 耗时: %s 状态: %s ===",
			task.EndTime.Format("2006-01-02 15:04:05"),
			duration,
			task.Status)

		if task.Status == "completed" {
			log.Printf("成功同步 %d 个源IP地址", len(task.AddedIPs))
		} else {
			log.Printf("同步任务失败: %s", task.ErrorMsg)
		}
	}()

	// 1. 查询DCDN L2节点IP信息
	log.Println("步骤1: 查询DCDN L2节点IP信息...")
	sourceIPs, err := s.dcdnClient.GetL2IPList()
	if err != nil {
		task.Status = "failed"
		task.ErrorMsg = fmt.Sprintf("查询DCDN L2节点IP信息失败: %v", err)
		return fmt.Errorf("%s", task.ErrorMsg)
	}

	log.Printf("查询到 %d 个L2节点IP地址", len(sourceIPs))

	// 记录源IP列表
	for _, ip := range sourceIPs {
		task.SourceIPs = append(task.SourceIPs, ip.IP)
	}

	// 2. 处理CIDR格式的IP地址，过滤IPv4地址
	log.Println("步骤2: 处理CIDR格式IP地址并过滤IPv4地址...")
	ipv4IPs := s.filterIPv4Addresses(sourceIPs)
	log.Printf("过滤结果: IPv4地址 %d 个", len(ipv4IPs))

	// 3. 同步到防火墙地址薄
	log.Println("步骤3: 同步到云防火墙地址薄...")
	for _, syncGroup := range s.config.Sync.AddressGroups {
		log.Printf("开始同步地址薄: %s", syncGroup.GroupName)

		// 使用IPv4地址
		targetIPs := ipv4IPs

		// 过滤源IP（如果有过滤规则）
		filteredIPs := s.filterSourceIPs(targetIPs, syncGroup)
		log.Printf("地址薄 %s: 过滤后剩余 %d 个IP地址", syncGroup.GroupName, len(filteredIPs))

		// 执行同步
		err = s.firewallClient.SyncAddressBook(syncGroup.GroupName, filteredIPs)
		if err != nil {
			log.Printf("同步地址薄 %s 失败: %v", syncGroup.GroupName, err)
			// 记录错误但继续处理其他地址薄
			if task.ErrorMsg == "" {
				task.ErrorMsg = fmt.Sprintf("同步地址薄 %s 失败: %v", syncGroup.GroupName, err)
			}
			continue
		}

		// 记录成功添加的IP
		for _, ip := range filteredIPs {
			task.AddedIPs = append(task.AddedIPs, ip.IP)
		}

		log.Printf("地址薄 %s 同步完成", syncGroup.GroupName)
	}

	// 4. 清理和统计
	log.Println("步骤4: 清理重复IP和生成统计...")
	task.AddedIPs = s.removeDuplicateIPs(task.AddedIPs)

	if task.ErrorMsg != "" {
		task.Status = "completed_with_errors"
	}

	return nil
}

// filterIPv4Addresses 过滤出IPv4地址
func (s *Scheduler) filterIPv4Addresses(sourceIPs []*models.DCDNSourceIPInfo) []*models.DCDNSourceIPInfo {
	var ipv4IPs []*models.DCDNSourceIPInfo

	for _, ipInfo := range sourceIPs {
		// 处理CIDR格式的IP地址
		if strings.Contains(ipInfo.IP, "/") {
			// 这是CIDR格式，提取网络地址
			_, ipNet, err := net.ParseCIDR(ipInfo.IP)
			if err != nil {
				log.Printf("警告: 无法解析CIDR格式的IP地址: %s, 错误: %v", ipInfo.IP, err)
				continue
			}

			// 只处理IPv4地址
			if ipNet.IP.To4() != nil {
				ipv4IPs = append(ipv4IPs, &models.DCDNSourceIPInfo{
					IP:          ipInfo.IP, // 保持原始CIDR格式
					Location:    ipInfo.Location,
					ISP:         ipInfo.ISP,
					Status:      ipInfo.Status,
					LastUpdated: ipInfo.LastUpdated,
				})
			}
		} else {
			// 单个IP地址
			if s.isIPv4(ipInfo.IP) {
				ipv4IPs = append(ipv4IPs, ipInfo)
			}
		}
	}

	return ipv4IPs
}

// filterSourceIPs 根据地址薄配置过滤源IP
func (s *Scheduler) filterSourceIPs(sourceIPs []*models.DCDNSourceIPInfo, group config.AddressGroup) []*models.DCDNSourceIPInfo {
	if len(group.IncludePatterns) == 0 && len(group.ExcludePatterns) == 0 {
		return sourceIPs
	}

	var filtered []*models.DCDNSourceIPInfo

	for _, ip := range sourceIPs {
		// 检查排除模式
		excluded := false
		for _, pattern := range group.ExcludePatterns {
			if s.matchIPPattern(ip.IP, pattern) {
				excluded = true
				break
			}
		}

		if excluded {
			continue
		}

		// 检查包含模式（如果有的话）
		if len(group.IncludePatterns) > 0 {
			included := false
			for _, pattern := range group.IncludePatterns {
				if s.matchIPPattern(ip.IP, pattern) {
					included = true
					break
				}
			}

			if !included {
				continue
			}
		}

		filtered = append(filtered, ip)
	}

	return filtered
}

// matchIPPattern 简单的IP模式匹配（支持CIDR和通配符）
func (s *Scheduler) matchIPPattern(ip, pattern string) bool {
	// 简化实现：支持精确匹配和简单通配符
	if pattern == "*" || pattern == ip {
		return true
	}

	// 可以在这里添加更复杂的匹配逻辑，如CIDR匹配
	// 当前仅支持简单的前缀匹配
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(ip) >= len(prefix) && ip[:len(prefix)] == prefix
	}

	return false
}

// removeDuplicateIPs 移除重复的IP地址
func (s *Scheduler) removeDuplicateIPs(ips []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, ip := range ips {
		if !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}

	return result
}

// RunOnce 立即执行一次同步任务
func (s *Scheduler) RunOnce() error {
	log.Println("手动执行同步任务...")
	return s.executeSyncTask()
}

// GetStatus 获取调度器状态
func (s *Scheduler) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"running":     s.ctx.Err() == nil,
		"next_run":    "基于配置的间隔时间",
		"config":      s.config.Scheduler,
		"last_update": time.Now().Format("2006-01-02 15:04:05"),
	}
}

// isIPv4 判断是否为IPv4地址
func (s *Scheduler) isIPv4(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.To4() != nil
}
