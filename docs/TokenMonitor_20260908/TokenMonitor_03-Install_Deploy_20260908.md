# Phase 03: Installation & Deployment Guide
## System: Token & Usage Monitor (`TokenMonitor`)
**Date:** 2026-09-08 | **Author:** Senior Infrastructure & System Operations Expert  
**Standard Reference:** Skill `infra_research_runbook` & `golang-expert-guidelines`

---

## 1. Chuẩn bị Môi trường (OS Preparation)

### 1.1. Trên môi trường Windows (PowerShell Administrator)
Đảm bảo cổng mạng không bị chặn bởi Windows Defender Firewall:
```powershell
# Cho phép mở cổng Dashboard (9090) nội bộ
New-NetFirewallRule -DisplayName "TokenMonitor-Dashboard" -Direction Inbound -LocalPort 9090 -Protocol TCP -Action Allow

# Tùy chọn: Mở cổng Proxy (8080) chỉ khi bật proxy.enabled: true
# New-NetFirewallRule -DisplayName "TokenMonitor-Proxy" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow
```

### 1.2. Trên môi trường Linux (Ubuntu / RHEL)
Tăng giới hạn File Descriptors để xử lý đồng thời lượng lớn socket kết nối:
```bash
# Thêm vào /etc/security/limits.conf
* soft nofile 65535
* hard nofile 65535

# Áp dụng sysctl tăng backlog socket
sudo sysctl -w net.core.somaxconn=4096
sudo sysctl -w fs.file-max=2097152
```

---

## 2. Biên dịch Ứng dụng Golang (Pure Go - Zero CGO Dependency)

Hệ thống được thiết kế theo chuẩn Go hiện đại với driver **`modernc.org/sqlite`** (100% Pure Go). Do đó, bạn có thể biên dịch trực tiếp trên Windows hay Linux mà **không cần cài đặt MinGW, GCC hay CGO**.

### 2.1. Cài đặt các thư viện phụ thuộc
Tại thư mục gốc của dự án `TokenMonitor`:
```bash
# Tải và đồng bộ toàn bộ Go modules
go mod tidy
```

### 2.2. Biên dịch tệp thực thi tối ưu (Production Build)
Sử dụng các cờ tối ưu `CGO_ENABLED=0` và `-ldflags="-s -w"` để loại bỏ bảng symbol và debug info, tạo binary độc lập duy nhất:
```bash
# Build trên Windows (PowerShell)
$env:CGO_ENABLED="0"; go build -ldflags="-s -w" -o token_monitor.exe .

# Build cho Linux AMD64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o token_monitor .
```

---

## 3. Cấu hình Dịch vụ (Service Configuration)

Tạo file cấu hình tại `./config.yaml` dựa trên mẫu chuẩn:

> Trích dẫn toàn bộ file mẫu tại: [TokenMonitor_Ref_002_config_template.yaml](./references/TokenMonitor_Ref_002_config_template.yaml) (Dòng 1 - 58)

Đảm bảo khởi tạo thư mục dữ liệu trước khi chạy:
```powershell
# Windows
New-Item -ItemType Directory -Force -Path "./data"

# Linux
mkdir -p ./data
chmod 750 ./data
```

Khởi tạo cấu trúc bảng SQLite ban đầu:
```bash
sqlite3 ./data/token_monitor.db < ./docs/TokenMonitor_20260908/references/TokenMonitor_Ref_001_schema.sql
```

---

## 4. Thiết lập Chạy Nền (Service Setup)

### 4.1. Chạy dưới dạng Windows Service (Khuyên dùng NSSM)
```powershell
# Cài đặt dịch vụ qua NSSM (Non-Sucking Service Manager)
nssm.exe install TokenMonitor "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\token_monitor.exe"
nssm.exe set TokenMonitor AppDirectory "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"
nssm.exe set TokenMonitor Start SERVICE_AUTO_START
nssm.exe start TokenMonitor
```

### 4.2. Chạy dưới dạng Systemd Service trên Linux
Tạo file `/etc/systemd/system/token-monitor.service`:
```ini
[Unit]
Description=TokenMonitor - LLM Token & Quota Telemetry Service
After=network.target

[Service]
Type=simple
User=appuser
Group=appuser
WorkingDirectory=/opt/TokenMonitor
ExecStart=/opt/TokenMonitor/token_monitor -config /opt/TokenMonitor/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

Kích hoạt và khởi chạy:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now token-monitor
sudo systemctl status token-monitor
```

---

## 5. Kiểm tra Sức khỏe & Xác minh (Health Check & Verification)

### 5.1. Lệnh kiểm tra trạng thái dịch vụ (Sanity Test)
```bash
# 1. Kiểm tra Liveness endpoint
curl -s http://localhost:9090/healthz
# Kỳ vọng phản hồi: {"status":"UP","timestamp":"2026-09-08T13:40:00Z"}

# 2. Kiểm tra truy vấn thống kê tổng hợp ban đầu
curl -s http://localhost:9090/api/metrics/summary
# Kỳ vọng phản hồi JSON chứa: total_grand_tokens, prompt_tokens, cached_tokens, thinking_tokens

# 3. Kiểm tra API Multi-Agent Fleet Telemetry
curl -s http://localhost:9090/api/agents/summary
# Kỳ vọng phản hồi JSON chứa: total_fleet, active_concurrency, concurrency_status, task_success_rate
```

### 5.2. Xác minh dữ liệu trực tiếp trong CSDL SQLite
Kiểm tra số lượng bản ghi thực tế được thu thập:
```bash
# Kiểm tra số lượng cuộc gọi LLM đã thu thập
sqlite3 ./data/token_monitor.db "SELECT count(*), sum(total_tokens) FROM token_usage_logs;"

# Kiểm tra số lượng tác vụ Subagents đã bóc tách
sqlite3 ./data/token_monitor.db "SELECT count(*), count(distinct role_name) FROM agent_fleet_telemetry;"
```
