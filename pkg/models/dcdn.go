package models

import (
	"time"
)

// DCDNSourceIPInfo 表示DCDN源IP信息
type DCDNSourceIPInfo struct {
	IP          string    `json:"ip"`
	Location    string    `json:"location"`
	ISP         string    `json:"isp"`
	Status      string    `json:"status"`
	LastUpdated time.Time `json:"last_updated"`
}

// DCDNDomainInfo 表示DCDN域名信息
type DCDNDomainInfo struct {
	DomainName string             `json:"domain_name"`
	SourceIPs  []DCDNSourceIPInfo `json:"source_ips"`
	UpdateTime time.Time          `json:"update_time"`
}

// DCDNQueryRequest DCDN查询请求
type DCDNQueryRequest struct {
	DomainName string `json:"domain_name"`
	StartTime  string `json:"start_time,omitempty"`
	EndTime    string `json:"end_time,omitempty"`
}

// DCDNQueryResponse DCDN查询响应
type DCDNQueryResponse struct {
	RequestId string           `json:"request_id"`
	Domains   []DCDNDomainInfo `json:"domains"`
	Success   bool             `json:"success"`
	Message   string           `json:"message,omitempty"`
}
