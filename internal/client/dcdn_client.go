package client

import (
	"fmt"
	"os"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dcdn20180115 "github.com/alibabacloud-go/dcdn-20180115/v3/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	credential "github.com/aliyun/credentials-go/credentials"

	"aliyun-dcdn-firewall-sync/internal/config"
	"aliyun-dcdn-firewall-sync/pkg/models"
)

// DCDNClient 阿里云DCDN客户端
type DCDNClient struct {
	config *config.DCDNConfig
	client *dcdn20180115.Client
}

// NewDCDNClient 创建新的DCDN客户端
func NewDCDNClient(cfg *config.DCDNConfig) *DCDNClient {
	client, err := createClient(&cfg.AliyunConfig)
	if err != nil {
		panic(fmt.Sprintf("创建DCDN客户端失败: %v", err))
	}

	return &DCDNClient{
		config: cfg,
		client: client,
	}
}

// createClient 使用凭证初始化账号Client
func createClient(cfg *config.AliyunConfig) (*dcdn20180115.Client, error) {
	// 使用更安全的凭证管理方式
	// 如果配置文件中提供了AK/SK，优先使用
	var cred credential.Credential
	var err error

	if cfg.AccessKeyId != "" && cfg.AccessKeySecret != "" {
		// 使用配置文件中的AK/SK
		cred, err = credential.NewCredential(&credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(cfg.AccessKeyId),
			AccessKeySecret: tea.String(cfg.AccessKeySecret),
		})
	} else {
		// DCDN客户端优先使用DCDN专用环境变量，回退到默认环境变量
		cred, err = createDCDNCredential()
	}

	if err != nil {
		return nil, fmt.Errorf("创建凭证失败: %v", err)
	}

	config := &openapi.Config{
		Credential: cred,
		RegionId:   tea.String(cfg.Region),
	}
	
	// 根据区域设置对应的endpoint
	// DCDN服务默认使用全球endpoint
	config.Endpoint = tea.String("dcdn.aliyuncs.com")

	client, err := dcdn20180115.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("创建 DCDN 客户端失败: %v", err)
	}

	return client, nil
}

// QuerySourceIPs 查询DCDN L2节点IP段
func (c *DCDNClient) QuerySourceIPs(domains []string) ([]*models.DCDNSourceIPInfo, error) {
	runtime := &util.RuntimeOptions{}

	// 调用DescribeDcdnL2IpsWithOptions获取L2节点IP段
	response, err := c.client.DescribeDcdnL2IpsWithOptions(runtime)
	if err != nil {
		return nil, fmt.Errorf("调用DescribeDcdnL2Ips API失败: %w", err)
	}

	if response.Body == nil {
		return nil, fmt.Errorf("API响应体为空")
	}

	// 解析响应，转换为我们的数据模型
	return c.parseL2IPs(response.Body)
}

// parseL2IPs 解析L2 IP段响应
func (c *DCDNClient) parseL2IPs(responseBody *dcdn20180115.DescribeDcdnL2IpsResponseBody) ([]*models.DCDNSourceIPInfo, error) {
	var sourceIPs []*models.DCDNSourceIPInfo

	if responseBody.Vips == nil {
		return sourceIPs, nil
	}

	// 遍历返回的IP段
	for _, vip := range responseBody.Vips {
		if vip == nil {
			continue
		}

		// 在新版本SDK中，vip是*string类型，只包含IP地址
		sourceIP := &models.DCDNSourceIPInfo{
			IP:          tea.StringValue(vip),
			Location:    "Global", // 默认值，因为新API不返回位置信息
			ISP:         "阿里云",    // 默认值
			Status:      "Active",
			LastUpdated: time.Now(), // 使用当前时间
		}

		sourceIPs = append(sourceIPs, sourceIP)
	}

	return sourceIPs, nil
}

// GetL2IPList 获取DCDN L2节点IP列表（别名方法，保持兼容性）
func (c *DCDNClient) GetL2IPList() ([]*models.DCDNSourceIPInfo, error) {
	// 不需要域名列表，直接调用QuerySourceIPs
	return c.QuerySourceIPs(nil)
}

// createDCDNCredential 创建DCDN专用凭证
func createDCDNCredential() (credential.Credential, error) {
	// 1. 优先使用DCDN专用环境变量
	dcdnAccessKeyId := os.Getenv("DCDN_ALIBABA_CLOUD_ACCESS_KEY_ID")
	dcdnAccessKeySecret := os.Getenv("DCDN_ALIBABA_CLOUD_ACCESS_KEY_SECRET")

	if dcdnAccessKeyId != "" && dcdnAccessKeySecret != "" {
		return credential.NewCredential(&credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(dcdnAccessKeyId),
			AccessKeySecret: tea.String(dcdnAccessKeySecret),
		})
	}

	// 2. 回退到标准环境变量
	standardAccessKeyId := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	standardAccessKeySecret := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")

	if standardAccessKeyId != "" && standardAccessKeySecret != "" {
		return credential.NewCredential(&credential.Config{
			Type:            tea.String("access_key"),
			AccessKeyId:     tea.String(standardAccessKeyId),
			AccessKeySecret: tea.String(standardAccessKeySecret),
		})
	}

	// 3. 使用默认凭证链
	return credential.NewCredential(nil)
}
