# 阿里云DCDN防火墙地址薄同步工具

这个工具用于自动同步阿里云DCDN的回源IP地址段到云防火墙的地址薄中。它可以定期获取DCDN的L2节点IP地址列表，并自动更新到指定的云防火墙地址薄中。

## 功能特点

- 自动获取DCDN L2节点IP地址段
- 支持IPv4地址段同步
- 支持CIDR格式的IP地址
- 自动创建和更新云防火墙地址薄
- 支持定时执行（基于cron表达式）
- 支持IP地址过滤（包含/排除模式）
- 支持多种凭证管理方式

## 系统要求

- Go 1.16 或更高版本
- 阿里云账号访问凭证
  - DCDN访问凭证（只读权限即可）
  - 云防火墙访问凭证（需要管理权限）

## 构建

1. 克隆代码仓库：
   ```bash
   git clone https://github.com/MarcosS99461/aliyun-dcdn-firewall-sync.git
   cd aliyun-dcdn-firewall-sync
   ```

2. 构建程序：
   ```bash
   GOOS=linux GOARCH=amd64 go build -o aliyun-dcdn-firewall-sync-linux-amd64 cmd/main.go
   ```

## 配置

1. 生成示例配置文件：
   ```bash
   aliyun-dcdn-firewall-sync --gen-config
   ```

2. 编辑配置文件 `/etc/aliyun-dcdn-firewall-sync/config.yaml`：
   ```yaml
   # DCDN配置
   dcdn:
     region: "ap-southeast-1"  # 区域设置

   # 防火墙配置
   firewall:
     region: "ap-southeast-1"  # 区域设置

   # 调度器配置
   scheduler:
     cron: "0 0 2 * * 0,3"   # 每周日和周三凌晨2点执行
     run_on_start: true      # 启动时立即执行一次
     timeout: "30m"          # 超时时间
     max_retries: 3          # 最大重试次数

   # 同步配置
   sync:
     address_groups:
       - group_name: "dcdn-source-ips-v4"
         description: "DCDN源IPv4地址组"
         ip_type: "ipv4"
         include_patterns:
           - "*"             # 包含所有IPv4
         exclude_patterns:   # 排除私有网络
           - "127.*"
           - "192.168.*"
           - "10.*"
           - "172.16.*"
   ```

3. 配置访问凭证（推荐使用环境变量）：
   ```bash
   # DCDN用户凭证
   export DCDN_ALIBABA_CLOUD_ACCESS_KEY_ID=your_dcdn_key
   export DCDN_ALIBABA_CLOUD_ACCESS_KEY_SECRET=your_dcdn_secret

   # 防火墙用户凭证
   export FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_ID=your_firewall_key
   export FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_SECRET=your_firewall_secret
   ```

## 运行

### 作为服务运行

1. 创建systemd服务文件 `/etc/systemd/system/aliyun-dcdn-firewall-sync.service`：
   ```ini
   [Unit]
   Description=Aliyun DCDN Firewall Sync Service
   After=network.target

   [Service]
   Type=simple
   ExecStart=/usr/local/bin/aliyun-dcdn-firewall-sync
   Restart=always
   User=root
   Environment=DCDN_ALIBABA_CLOUD_ACCESS_KEY_ID=your_dcdn_key
   Environment=DCDN_ALIBABA_CLOUD_ACCESS_KEY_SECRET=your_dcdn_secret
   Environment=FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_ID=your_firewall_key
   Environment=FIREWALL_ALIBABA_CLOUD_ACCESS_KEY_SECRET=your_firewall_secret

   [Install]
   WantedBy=multi-user.target
   ```

2. 启动服务：
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable aliyun-dcdn-firewall-sync
   sudo systemctl start aliyun-dcdn-firewall-sync
   ```

### 手动运行

- 执行一次性同步：
  ```bash
  aliyun-dcdn-firewall-sync --once
  ```

- 生成示例配置：
  ```bash
  aliyun-dcdn-firewall-sync --gen-config
  ```

- 显示版本信息：
  ```bash
  aliyun-dcdn-firewall-sync --version
  ```

## 日志

- 服务日志可通过 systemd journal 查看：
  ```bash
  journalctl -u aliyun-dcdn-firewall-sync -f
  ```

- 日志级别和格式可在配置文件中设置：
  ```yaml
  logging:
    level: "info"    # debug, info, warn, error
    format: "text"   # text, json
    file_path: "logs/sync.log"
  ```

## 注意事项

1. 权限要求：
   - DCDN用户需要只读权限
   - 防火墙用户需要地址薄管理权限

2. 安全建议：
   - 使用专门的子账号
   - 遵循最小权限原则
   - 定期轮换访问密钥
   - 避免在代码或配置文件中硬编码凭证

3. 运维建议：
   - 定期检查日志
   - 监控服务状态
   - 设置适当的执行间隔
   - 保持系统时间准确

## 故障排除

1. 服务无法启动：
   - 检查配置文件权限和格式
   - 验证环境变量是否正确设置
   - 查看系统日志获取详细错误信息

2. 同步失败：
   - 确认网络连接正常
   - 验证访问凭证有效性
   - 检查防火墙策略设置
   - 查看详细错误日志

3. 地址薄更新问题：
   - 确认地址薄名称正确
   - 验证IP地址格式
   - 检查过滤规则设置

## 维护和支持

- 定期更新依赖包
- 关注阿里云API变更
- 保持系统和Go环境更新
