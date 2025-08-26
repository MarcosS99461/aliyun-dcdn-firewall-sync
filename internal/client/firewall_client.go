package client

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"aliyun-dcdn-firewall-sync/internal/config"
	"aliyun-dcdn-firewall-sync/pkg/models"

	cloudfw20171207 "github.com/alibabacloud-go/cloudfw-20171207/v8/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	credential "github.com/aliyun/credentials-go/credentials"
)

// FirewallClient 阿里云云防火墙客户端
type FirewallClient struct {
	config *config.FirewallConfig
	sync   *config.SyncConfig
	client *cloudfw20171207.Client
}

// NewFirewallClient 创建新的云防火墙客户端
func NewFirewallClient(cfg *config.FirewallConfig, sync *config.SyncConfig) *FirewallClient {
	// 初始化安全凭证
	cred, err := initializeCredential(&cfg.AliyunConfig)
	if err != nil {
		panic(fmt.Sprintf("初始化防火墙客户端凭证失败: %v", err))
	}

	config := &openapi.Config{
		Credential: cred,
		RegionId:   tea.String(cfg.Region),
	}

	// 根据区域设置对应的endpoint
	if cfg.Region == "ap-southeast-1" {
		config.Endpoint = tea.String("cloudfw.ap-southeast-1.aliyuncs.com")
	} else if cfg.Region == "cn-hangzhou" {
		config.Endpoint = tea.String("cloudfw.aliyuncs.com")
	} else {
		config.Endpoint = tea.String(fmt.Sprintf("cloudfw.%s.aliyuncs.com", cfg.Region))
	}

	client, err := cloudfw20171207.NewClient(config)
	if err != nil {
		panic(fmt.Sprintf("创建防火墙客户端失败: %v", err))
	}

	return &FirewallClient{
		config: cfg,
		sync:   sync,
		client: client,
	}
}

// initializeCredential 初始化安全凭证
func initializeCredential(cfg *config.AliyunConfig) (credential.Credential, error) {
	// 1. 优先使用配置文件中的AK/SK
	if cfg.AccessKeyId != "" && cfg.AccessKeySecret != "" {
		return credential.NewCredential(&credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(cfg.AccessKeyId),
			AccessKeySecret: tea.String(cfg.AccessKeySecret),
		})
	}

	// 2. 尝试使用防火墙专用环境变量
	firewallAccessKeyId := os.Getenv("FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_ID")
	firewallAccessKeySecret := os.Getenv("FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_SECRET")

	if firewallAccessKeyId != "" && firewallAccessKeySecret != "" {
		return credential.NewCredential(&credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(firewallAccessKeyId),
			AccessKeySecret: tea.String(firewallAccessKeySecret),
		})
	}

	// 3. 回退到标准环境变量
	standardAccessKeyId := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	standardAccessKeySecret := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")

	if standardAccessKeyId != "" && standardAccessKeySecret != "" {
		return credential.NewCredential(&credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(standardAccessKeyId),
			AccessKeySecret: tea.String(standardAccessKeySecret),
		})
	}

	// 4. 使用默认凭证链
	return credential.NewCredential(nil)
}

// GetAddressBookByName 根据名称获取单个地址薄的详细信息
func (c *FirewallClient) GetAddressBookByName(groupName string) (*models.FirewallAddressBook, error) {
	fmt.Printf("DEBUG: 开始根据名称获取地址薄详情: %s\n", groupName)

	// 使用精确的名称查询
	request := &cloudfw20171207.DescribeAddressBookRequest{
		PageSize:    tea.String("50"),
		CurrentPage: tea.String("1"),
		Lang:        tea.String("zh"),
		GroupType:   tea.String("ip"),
	}

	runtime := &util.RuntimeOptions{}

	response, err := c.client.DescribeAddressBookWithOptions(request, runtime)
	if err != nil {
		return nil, fmt.Errorf("调用DescribeAddressBook API失败: %v", err)
	}

	if response.Body == nil {
		return nil, fmt.Errorf("API响应体为空")
	}

	if len(response.Body.Acls) == 0 {
		return nil, nil // 地址薄不存在
	}

	// 查找匹配的地址簿
	var targetAcl *cloudfw20171207.DescribeAddressBookResponseBodyAcls
	for _, acl := range response.Body.Acls {
		if acl != nil && tea.StringValue(acl.GroupName) == groupName {
			targetAcl = acl
			break
		}
	}

	if targetAcl == nil {
		return nil, nil // 未找到精确匹配的地址薄
	}

	// 转换为内部数据结构
	var entries []models.FirewallAddressEntry
	for _, addr := range targetAcl.AddressList {
		if addr == nil {
			continue
		}
		entry := models.FirewallAddressEntry{
			IP:          tea.StringValue(addr),
			Description: "",
			Tags:        []string{},
			CreatedTime: time.Now(),
			UpdatedTime: time.Now(),
		}
		entries = append(entries, entry)
	}

	return &models.FirewallAddressBook{
		GroupId:     tea.StringValue(targetAcl.GroupUuid),
		GroupName:   tea.StringValue(targetAcl.GroupName),
		Description: tea.StringValue(targetAcl.Description),
		UpdateTime:  time.Now(),
		Entries:     entries,
	}, nil
}

// SyncAddressBook 同步地址薄
func (c *FirewallClient) SyncAddressBook(groupName string, sourceIPs []*models.DCDNSourceIPInfo) error {
	// 1. 准备新的IP地址集合（过滤无效IP并进行格式化）
	var newIPs []string

	fmt.Printf("DEBUG: 开始处理地址薄: %s\n", groupName)

	// 处理IP地址
	for _, ip := range sourceIPs {
		if !c.isValidIP(ip.IP) {
			fmt.Printf("DEBUG: 跳过无效IP: %s\n", ip.IP)
			continue
		}

		ipStr := ip.IP

		// 解析IP和CIDR
		var ipNet *net.IPNet
		var parsedIP net.IP
		var err error

		if strings.Contains(ipStr, "/") {
			// CIDR格式
			parsedIP, ipNet, err = net.ParseCIDR(ipStr)
			if err != nil {
				fmt.Printf("DEBUG: 无法解析CIDR: %s, 错误: %v\n", ipStr, err)
				continue
			}
			// 确保是IPv4地址
			if parsedIP.To4() == nil {
				continue
			}
			ipStr = ipNet.String()
		} else {
			// 单个IP地址
			parsedIP = net.ParseIP(ipStr)
			if parsedIP == nil {
				fmt.Printf("DEBUG: 无法解析IP: %s\n", ipStr)
				continue
			}
			// 确保是IPv4地址
			if parsedIP.To4() == nil {
				continue
			}
			ipStr = parsedIP.String()
		}

		fmt.Printf("DEBUG: 添加IPv4地址: %s\n", ipStr)
		newIPs = append(newIPs, ipStr)
	}

	fmt.Printf("DEBUG: 过滤后的IP数量: %d\n", len(newIPs))

	// 2. 获取地址薄信息
	targetBook, err := c.GetAddressBookByName(groupName)
	if err != nil {
		return fmt.Errorf("获取地址薄信息失败: %v", err)
	}

	runtime := &util.RuntimeOptions{}

	if targetBook == nil {
		// 获取地址薄配置
		var addressGroup *config.AddressGroup
		for _, group := range c.sync.AddressGroups {
			if group.GroupName == groupName {
				addressGroup = &group
				break
			}
		}
		if addressGroup == nil {
			return fmt.Errorf("未找到地址薄 %s 的配置信息", groupName)
		}

		// 如果地址薄不存在，创建新的
		request := &cloudfw20171207.AddAddressBookRequest{
			Description:   tea.String(addressGroup.Description),
			GroupName:     tea.String(groupName),
			AddressList:   tea.String(strings.Join(newIPs, ",")),
			AutoAddTagEcs: tea.String("false"),
			TagRelation:   tea.String("and"),
			GroupType:     tea.String("ip"),
			Lang:          tea.String("zh"),
		}

		_, err = c.client.AddAddressBookWithOptions(request, runtime)
		if err != nil {
			return fmt.Errorf("创建地址薄失败: %v", err)
		}
		fmt.Printf("成功创建地址薄 %s，IP数量: %d\n", groupName, len(newIPs))
	} else {
		// 获取地址薄配置
		var addressGroup *config.AddressGroup
		for _, group := range c.sync.AddressGroups {
			if group.GroupName == groupName {
				addressGroup = &group
				break
			}
		}
		if addressGroup == nil {
			return fmt.Errorf("未找到地址薄 %s 的配置信息", groupName)
		}

		// 如果地址薄已存在，直接覆盖IP列表
		modifyRequest := &cloudfw20171207.ModifyAddressBookRequest{
			GroupUuid:   tea.String(targetBook.GroupId),
			GroupName:   tea.String(groupName),
			Description: tea.String(addressGroup.Description),
			AddressList: tea.String(strings.Join(newIPs, ",")),
		}
		_, err = c.client.ModifyAddressBookWithOptions(modifyRequest, runtime)
		if err != nil {
			return fmt.Errorf("更新地址薄失败: %v", err)
		}
		fmt.Printf("成功更新地址薄 %s，IP数量: %d\n", groupName, len(newIPs))
	}

	return nil
}

// isValidIP 验证是否为有效IP地址或CIDR
func (c *FirewallClient) isValidIP(ip string) bool {
	// 检查是否为CIDR格式
	if strings.Contains(ip, "/") {
		_, _, err := net.ParseCIDR(ip)
		return err == nil
	}

	// 检查单个IP地址
	return net.ParseIP(ip) != nil
}

// calculateIPDifferences 计算IP地址集合差异
func (c *FirewallClient) calculateIPDifferences(existing []string, new []string) (toAdd []string, toRemove []string) {
	// 使用map提高查找效率
	existingMap := make(map[string]bool)
	newMap := make(map[string]bool)

	// 构建现有IP集合
	for _, ip := range existing {
		existingMap[ip] = true
	}

	// 构建新IP集合
	for _, ip := range new {
		newMap[ip] = true
	}

	// 找出需要添加的IP（新集合中有但现有集合中没有的）
	for _, ip := range new {
		if !existingMap[ip] {
			toAdd = append(toAdd, ip)
		}
	}

	// 找出需要删除的IP（现有集合中有但新集合中没有的）
	for _, ip := range existing {
		if !newMap[ip] {
			toRemove = append(toRemove, ip)
		}
	}

	return toAdd, toRemove
}
