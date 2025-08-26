package models

import (
	"time"
)

// FirewallAddressEntry 云防火墙地址簿条目
type FirewallAddressEntry struct {
	IP          string    `json:"ip"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags,omitempty"`
	CreatedTime time.Time `json:"created_time"`
	UpdatedTime time.Time `json:"updated_time"`
}

// FirewallAddressBook 云防火墙地址簿
type FirewallAddressBook struct {
	GroupName   string                 `json:"group_name"`
	GroupId     string                 `json:"group_id"`
	Description string                 `json:"description"`
	Entries     []FirewallAddressEntry `json:"entries"`
	UpdateTime  time.Time              `json:"update_time"`
}

// AddAddressRequest 添加地址请求
type AddAddressRequest struct {
	GroupName   string   `json:"group_name"`
	AddressList []string `json:"address_list"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// RemoveAddressRequest 移除地址请求
type RemoveAddressRequest struct {
	GroupName   string   `json:"group_name"`
	AddressList []string `json:"address_list"`
}

// FirewallAPIResponse 云防火墙API响应
type FirewallAPIResponse struct {
	RequestId string      `json:"request_id"`
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// SyncTask 同步任务
type SyncTask struct {
	TaskId     string    `json:"task_id"`
	Status     string    `json:"status"` // pending, running, completed, failed
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	SourceIPs  []string  `json:"source_ips"`
	AddedIPs   []string  `json:"added_ips"`
	RemovedIPs []string  `json:"removed_ips"`
	ErrorMsg   string    `json:"error_msg,omitempty"`
}
