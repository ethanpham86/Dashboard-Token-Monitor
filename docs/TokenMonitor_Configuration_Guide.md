# Hướng Dẫn Cấu Hình Toàn Diện Hệ Thống (Configuration Guide) - TokenMonitor

> **Tài liệu**: Hướng dẫn cấu hình chi tiết file `config.yaml`, cơ chế bảo mật và vai trò kỹ thuật của tất cả các file trong dự án TokenMonitor.  
> **Phiên bản áp dụng**: TokenMonitor v1.0 Production  
> **Hệ điều hành hỗ trợ**: Windows 11 / 10, Linux, macOS  

---

## 1. Chi Tiết Cấu Hình File `config.yaml`

File `config.yaml` là trung tâm điều khiển cấu hình của toàn bộ ứng dụng. Dưới đây là phân tích chi tiết từng khối thuộc tính, ý nghĩa và giá trị khuyến nghị:

```yaml
# ==============================================================================
# TOKENMONITOR CONFIGURATION SPECIFICATION
# ==============================================================================

# 1. Khối cấu hình Dashboard Web Server
server:
  dashboard_port: 9090          # Cổng mạng mở giao diện giám sát (Mặc định: 9090)
  bind_address: "127.0.0.1"     # Địa chỉ IP lắng nghe (Khuyến nghị 127.0.0.1 để bảo mật nội bộ)
  read_timeout_seconds: 15      # Thời gian chờ tối đa khi nhận HTTP request (giây)
  write_timeout_seconds: 30     # Thời gian chờ tối đa khi gửi dữ liệu HTTP response (giây)

# 2. Khối cấu hình Reverse Proxy (Tùy chọn)
proxy:
  enabled: false                # false: Dùng Local Tailer (Khuyến nghị, thụ động, an toàn 100%)
                                # true: Mở cổng trung gian bắt gói tin API trực tiếp
  listen_port: 8080             # Cổng proxy nếu bật chế độ proxy
  upstream_target: "https://generativelanguage.googleapis.com" # Máy chủ gốc của Google Gemini
  max_concurrent_requests: 100  # Giới hạn số kết nối đồng thời qua proxy
  upstream_timeout_seconds: 180 # Timeout khi gọi Google API (giây)

# 3. Khối cấu hình Cơ Sở Dữ Liệu SQLite
database:
  sqlite_path: "./data/token_monitor.db" # Đường dẫn file CSDL trên ổ cứng cục bộ
  max_open_conns: 25            # Số kết nối mở tối đa tới SQLite
  max_idle_conns: 10            # Số kết nối nhàn rỗi duy trì trong pool
  conn_max_lifetime_minutes: 60 # Thời gian tồn tại tối đa của một kết nối (phút)
  enable_wal_mode: true         # Bật chế độ Write-Ahead Logging (đọc ghi song song)
  rollup_interval_seconds: 300  # Chu kỳ tự động gộp dữ liệu theo giờ (300s = 5 phút)

# 4. Khối thông tin hồ sơ tài khoản Google AI
account_profile:
  email: "ethanpham671986@gmail.com" # Email tài khoản sở hữu gói bản quyền
  account_type: "Google Consumer Account (Individual)" # Loại tài khoản (Consumer/Workspace)
  plan_name: "Google AI Ultra (Pham Ethan)"           # Gói dịch vụ hiển thị
  quota_bandwidth: "Unlimited Local Quota"            # Băng thông hạn mức quy định
  installation_uuid_path: "C:\\Users\\EthanPham\\.gemini\\antigravity\\installation_id" # File UUID máy trạm
  subscription_start: "2026-09-06"                    # Ngày bắt đầu chu kỳ thuê bao (YYYY-MM-DD)
  subscription_expiry: "2026-10-06"                   # Ngày hết hạn chu kỳ hiện tại (YYYY-MM-DD)
  auto_renew: true                                    # Tự động gia hạn (true = có)

# 5. Khối cấu hình Local Tailer (Trình thu thập log cục bộ từ IDE)
local_tailer:
  enabled: true                 # Bật trình quét log cục bộ (Cơ chế hoạt động chính)
  ide_brain_dir: "C:\\Users\\EthanPham\\.gemini\\antigravity\\brain" # Thư mục chứa transcript hội thoại
  ide_conv_db_dir: "C:\\Users\\EthanPham\\.gemini\\antigravity\\conversations" # Thư mục CSDL chat
  poll_interval_seconds: 10     # Tần suất kiểm tra file log mới (10 giây/lần)

# 6. Khối cấu hình Giám Sát OpenAI Codex (Thụ động từ session JSONL cục bộ)
openai_monitor:
  enabled: true                 # Bật trình thu thập OpenAI Codex (Khuyến nghị: true)
  sessions_dir: ""              # Để trống để tự động dùng ~/.codex/sessions (Zero credentials, không đọc auth.json)
  poll_interval_seconds: 10     # Tần suất kiểm tra phiên làm việc mới (giây)
  max_session_rows: 50          # Số phiên làm việc gần nhất hiển thị trên dashboard
  max_files: 1000               # Giới hạn số file session JSONL quét tối đa

# 7. Khối cấu hình Giám Sát Anthropic Claude Code CLI (Thụ động từ project JSONL cục bộ)
claude_monitor:
  enabled: true                 # Bật trình thu thập Claude Code (Khuyến nghị: true)
  projects_dir: ""              # Để trống để tự động dùng ~/.claude/projects (Zero credentials, không đọc .credentials.json)
  poll_interval_seconds: 10     # Tần suất kiểm tra dự án mới (giây)
  max_session_rows: 50          # Số phiên dự án gần nhất hiển thị trên dashboard
  max_files: 1000               # Giới hạn số file project JSONL quét tối đa

# 8. Khối cấu hình Cảnh Báo (Alerting)
alerting:
  enabled: false                # true = Gửi tin nhắn cảnh báo khi vượt ngưỡng
  daily_token_threshold: 50000000 # Ngưỡng token cảnh báo trong ngày (vd: 50 triệu token)
  token_expiry_warning_minutes: 10 # Cảnh báo trước khi phiên token hết hạn (phút)
  telegram:
    bot_token: ""               # Mã Bot Token do BotFather cấp (nếu dùng)
    chat_id: ""                 # ID người nhận trên Telegram

# 9. Khối cấu hình Tự Động Sao Lưu Định Kỳ (Auto-Backup & Disaster Recovery)
backup:
  enabled: true                 # Bật cơ chế tự động backup SQLite định kỳ (Khuyến nghị: true)
  interval_minutes: 60          # Chu kỳ cập nhật bản sao lưu trong ngày (mặc định: 60 phút = 1 giờ/lần)
  backup_dir: "./data/backup"   # Thư mục lưu trữ các file snapshot .db an toàn
  max_keep: 7                   # Lưu trữ tối đa 7 ngày gần nhất (mỗi ngày 1 file duy nhất, tự xoay vòng dọn dẹp)
```

### 1.1. Các Cơ Chế Cấu Hình & Bóc Tách Nâng Cao Cần Lưu Ý

#### A. Phân Định 3 Tầng Giá Trị Hồ Sơ Tài Khoản (3-Tier Profile Value Hierarchy)
Hệ thống TokenMonitor quản lý thông tin tài khoản qua 3 cấp độ ưu tiên rõ ràng:
1. **Tầng 1 — Go Struct Defaults (`config.NewDefaultConfig`)**: Giá trị dự phòng khi không có cấu hình: `plan_name = "20X ULTRA PLAN"`, `quota_bandwidth = "20x Quota Bandwidth"`.
2. **Tầng 2 — Cấu hình máy trạm (`config.yaml`)**: Thiết lập người dùng khai báo tĩnh: `plan_name = "Google AI Ultra (Pham Ethan)"`, `quota_bandwidth = "Unlimited Local Quota"`.
3. **Tầng 3 — Nhận diện động thời gian thực (Runtime Detected)**: Module `storage/detector.go` bóc tách trực tiếp từ CSDL IDE `state.vscdb`: `plan_name = "Google AI Ultra (20X Ultra Tier)"` cùng Email thực tế `ethanpham671986@gmail.com` và Tên `Pham Ethan`. Giá trị tầng 3 này có độ ưu tiên cao nhất khi hiển thị trên Dashboard.

#### B. Cơ Chế Cấu Hình Sao Lưu Kép (Dual Backup Hierarchy)
Bộ phân tích cú pháp `config/config.go:215-226` hỗ trợ định nghĩa khối cấu hình sao lưu ở cả hai vị trí:
- Cấp gốc: `backup:` (như mẫu trong `config.yaml`).
- Khối con lồng nhau: `database.backup:` (hỗ trợ phân cấp theo chuẩn database config).
- **Quy tắc ghi đè**: Nếu người dùng khai báo `database.backup`, các thuộc tính con (`enabled`, `interval_minutes`, `backup_dir`, `max_keep`) sẽ tự động ghi đè lên cấu hình tại cấp gốc.

#### C. Cơ Chế Quét Đa Thư Mục Não Bộ Động (Dynamic Multi-Brain Discovery)
Thay vì chỉ đọc duy nhất một đường dẫn cố định, `collector/tailer.go:46-79` triển khai thuật toán phát hiện thư mục động:
- Quét toàn bộ các thư mục con trong `~/.gemini/*` để tìm kiếm các thư mục có dạng `~/.gemini/<dir>/brain`.
- **Quy tắc loại trừ an toàn**: Tự động bỏ qua các thư mục chứa từ khóa `"backup"`, `"tmp"`, hoặc `"profile"` trong tên để triệt tiêu việc đọc trùng lặp log cũ hoặc file tạm rác.
- Hợp nhất và khử trùng lặp với đường dẫn `cfg.LocalTailer.IDEBrainDir` được cấu hình trong `config.yaml`.

---

## 2. Giải Thích Cơ Chế "Mật Khẩu Gmail" (Gmail Security Compliance)

Một câu hỏi quan trọng trong quá trình vận hành là: **"Tại sao trong file cấu hình không có trường mật khẩu Gmail (Password)?"**

### 2.1. Nguyên Tắc An Toàn Của Google & Tiêu Chuẩn Bảo Mật OAuth 2.0
- **Tuyệt đối không lưu mật khẩu Plaintext**: Google cấm hoàn toàn các ứng dụng bên thứ ba lưu trữ mật khẩu Gmail dạng văn bản thô. Bất kỳ phần mềm nào yêu cầu người dùng nhập mật khẩu Gmail trực tiếp đều bị Google gắn cờ là phần mềm độc hại (Malware/Phishing) và có nguy cơ dẫn tới khóa tài khoản Google vĩnh viễn.
- **Cơ chế xác thực qua Single Sign-On (SSO)**: 
  - Người dùng đăng nhập tài khoản Google an toàn thông qua trình duyệt và Antigravity IDE.
  - Sau khi đăng nhập thành công, Google cấp một phiên làm việc (OAuth Session Token) được mã hóa và lưu tại file hệ thống `state.vscdb`.
- **Thứ tự ưu tiên 3 đường dẫn ứng viên (Candidate Paths)**:
  1. `os.Getenv("ANTIGRAVITY_STATE_DB")`: Biến môi trường tùy chỉnh dành cho môi trường container hóa, debug hoặc kiểm thử tự động.
  2. `filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb")`: Thư mục cài đặt chuẩn chính thức của Antigravity IDE trên hệ điều hành Windows (`%APPDATA%\Antigravity IDE\...`).
  3. `filepath.Join(appData, "Antigravity", "User", "globalStorage", "state.vscdb")`: Thư mục cài đặt phụ / phiên bản tương thích (`%APPDATA%\Antigravity\...`).
- **Driver độc lập `sqlite_detector` & Chế độ chỉ đọc (`mode=ro`)**:
  - Module `storage/detector.go` chủ động đăng ký driver riêng biệt mang tên `"sqlite_detector"` (`sql.Register("sqlite_detector", &sqlite.Driver{})`), hoàn toàn tách rời khỏi connection hook PRAGMA của CSDL chính.
  - Mở kết nối dạng `file:%s?mode=ro` (thông qua `filepath.ToSlash`), tuyệt đối không chiếm lock độc quyền (Exclusive Lock), không can thiệp hay gây nghẽn tiến trình làm việc của Antigravity IDE.
- **Cơ chế phân tích kép (Dual-Mode Extraction Engine)**:
  - **Mode 1 (Primary - JSON Payload)**: Truy vấn khóa `antigravityAuthStatus` từ bảng `ItemTable`. Phân tích cấu trúc JSON chứa `name`, `email`, và `userStatusProtoBinaryBase64` để nhận diện chính xác tài khoản cùng hạng gói cước (`Google AI Ultra (20X Ultra Tier)` nếu chứa `"Google AI Ultra"` hoặc `"g1-ultra-tier"`, ngược lại là `Google AI Pro`).
  - **Mode 2 (Secondary - Protobuf Stream Fallback)**: Kích hoạt khi không tìm thấy khóa JSON. Truy vấn khóa `antigravityUnifiedStateSync.userStatus`, giải mã Base64 (hỗ trợ cả Padded và Raw Unpadded), quét Regex RFC 5322 bóc tách email (`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), tìm kiếm chữ ký gói cước và trích xuất tên hiển thị ASCII.
- **Tuân thủ chính sách Zero Password & Zero Credential Storage**:
  - Không bao giờ yêu cầu hay lưu trữ mật khẩu Gmail.
  - Khóa API (nếu có trong payload) chỉ xử lý thoáng qua trong bộ nhớ RAM, **tuyệt đối không bao giờ ghi ra đĩa hay lưu vào CSDL SQLite**.
  - Bảng `accounts` và `auth_sessions` trong SQLite chỉ lưu hồ sơ tài khoản, gói cước và thời hạn phiên, hoàn toàn không lưu OAuth tokens hay header ủy quyền. Đảm bảo an toàn tuyệt đối 100%.

---

## 3. Bản Đồ Kỹ Thuật & Vai Trò Của Từng File Trong Dự Án

Dưới đây là cấu trúc thư mục chi tiết và nhiệm vụ của từng file mã nguồn trong dự án TokenMonitor:

```
TokenMonitor/
├── config.yaml                    # File cấu hình trung tâm của toàn bộ hệ thống
├── main.go                        # Entry point: Khởi tạo, điều phối goroutine và tắt an toàn
├── tokenmonitor_test.go           # Bộ kiểm thử tự động (Unit test & Integration test)
├── token_monitor.exe              # File thực thi nhị phân đã biên dịch sẵn cho Windows 64-bit
│
├── config/
│   └── config.go                  # Parser đọc config.yaml, gán default và parse thời gian
│
├── storage/
│   ├── db.go                      # Khởi tạo SQLite, Connection Pooling, WAL Pragma & DDL schema
│   ├── detector.go                # Tự động đọc phiên đăng nhập từ Antigravity state.vscdb (Read-Only)
│   └── repository.go              # Tầng CRUD, thực hiện các truy vấn thống kê, rollup, top models
│
├── collector/
│   ├── buffer.go                  # Hàng đợi bất đồng bộ (Async Queue) ghi dữ liệu theo lô (Batching)
│   ├── models.go                  # Định nghĩa các cấu trúc dữ liệu log token và thông số kỹ thuật
│   ├── proxy.go                   # Reverse Proxy trung gian (Tùy chọn khi muốn bắt gói tin trực tiếp)
│   └── tailer.go                  # Quét file transcript.jsonl cục bộ, bóc tách prompt/thinking/output
│
├── web/
│   ├── handler.go                 # Controller xử lý các API RESTful JSON cho Web Dashboard
│   └── static/
│       └── index.html             # Giao diện Web Dashboard trực quan (Glassmorphism, Dark Mode)
│
└── docs/                          # Thư mục chứa toàn bộ tài liệu kỹ thuật & sơ đồ hệ thống
    ├── README.md                  # Cổng thông tin tài liệu trung tâm
    ├── TokenMonitor_Database_ERD.md # Đặc tả chi tiết CSDL, quan hệ 1:N, Data Dictionary
    ├── TokenMonitor_Configuration_Guide.md # Hướng dẫn cấu hình chi tiết (Tài liệu này)
    ├── tokenmonitor-architecture.html # Sơ đồ kiến trúc tương tác 3 tầng (Interactive Diagram)
    ├── tokenmonitor-dataflow.html     # Sơ đồ luồng thu thập dữ liệu bất đồng bộ (Data Flow)
    ├── tokenmonitor-database-erd.html # Sơ đồ thực thể CSDL tương tác (Database ERD Diagram)
    └── TokenMonitor_20260908/         # Bộ tài liệu Runbook 7 giai đoạn theo chuẩn Production
        ├── TokenMonitor_00-Prerequisites_20260908.md
        ├── TokenMonitor_01-Architecture_20260908.md
        ├── TokenMonitor_02-HA_Deployment_20260908.md
        ├── TokenMonitor_03-Install_Deploy_20260908.md
        ├── TokenMonitor_04-Tuning_20260908.md
        ├── TokenMonitor_05-Troubleshooting_20260908.md
        └── TokenMonitor_06-Security_Policy_Compliance_20260908.md
```

### 3.1. Chi Tiết Vai Trò Kỹ Thuật Từng File Mã Nguồn

#### `main.go`
- Nạp cấu hình từ `config.yaml`.
- Khởi tạo kết nối SQLite với cấu hình WAL Mode.
- Khởi động Goroutine cho `LocalTailer` (quét log định kỳ mỗi 10s).
- Khởi động Goroutine cho Rollup Scheduler (tự động gộp dữ liệu mỗi 5 phút).
- Khởi động HTTP Web Server trên cổng cấu hình (mặc định `http://127.0.0.1:9090`).
- Lắng nghe tín hiệu hệ thống (`os.Interrupt`, `SIGTERM`) để thực hiện **Graceful Shutdown**: xả toàn bộ dữ liệu trong bộ đệm ra đĩa trước khi dừng tiến trình.

#### `config/config.go`
- Định nghĩa cấu trúc `Config`, `ServerConfig`, `ProxyConfig`, `DatabaseConfig`, `AccountProfileConfig`, `LocalTailerConfig`, `OpenAIMonitorConfig`, `ClaudeMonitorConfig`, `AlertingConfig`, `BackupConfig`.
- Hàm `LoadConfig(path)`: Phân tích file YAML, tự động gán giá trị mặc định an toàn cho các tham số bị bỏ trống.
- Hàm `ParseSubscriptionTimes()`: Chuyển đổi định dạng ngày bắt đầu và hết hạn gói dịch vụ.

#### `storage/db.go`
- Quản lý vòng đời kết nối SQLite (`database/sql` với driver `modernc.org/sqlite` - 100% Pure Go).
- Cấu hình Connection Pool (`SetMaxOpenConns = 25`, `SetMaxIdleConns = 10`, `SetConnMaxLifetime = 60m`).
- Chạy 6 chỉ thị PRAGMA tối ưu: `foreign_keys=ON`, `journal_mode=WAL`, `synchronous=NORMAL`, `cache_size=-64000`, `temp_store=MEMORY`, `busy_timeout=5000`.
- Chạy tự động kịch bản Migration tạo **5 bảng**:
  1. `accounts`: Hồ sơ tài khoản, gói cước và ngày hết hạn.
  2. `auth_sessions`: Quản lý phiên làm việc và trạng thái token OAuth.
  3. `token_usage_logs`: Nhật ký thô từng lượt gọi mô hình LLM.
  4. `token_usage_hourly_rollup`: Bảng tổng hợp dữ liệu theo giờ tối ưu hóa tốc độ vẽ biểu đồ.
  5. `agent_fleet_telemetry`: Lưu vết chi tiết từng tác vụ công cụ của các Subagent và Worker.
- Thiết lập 7 chỉ mục hiệu năng (gồm 3 chỉ mục chuyên biệt cho Multi-Agent Fleet).

#### `storage/detector.go`
- Tự động nhận diện hồ sơ tài khoản và gói dịch vụ AI từ Antigravity IDE qua 3 đường dẫn ứng viên:
  1. `ANTIGRAVITY_STATE_DB` (Biến môi trường tùy chỉnh).
  2. `%APPDATA%\Antigravity IDE\User\globalStorage\state.vscdb` (Thư mục chuẩn của Antigravity IDE).
  3. `%APPDATA%\Antigravity\User\globalStorage\state.vscdb` (Thư mục tương thích phụ).
- Sử dụng driver SQLite độc lập `"sqlite_detector"` (tránh kế thừa connection hook WAL/foreign_keys).
- Mở file ở chế độ chỉ đọc `mode=ro`, không bao giờ chiếm khóa ghi độc quyền, an toàn tuyệt đối khi IDE đang hoạt động.
- Cơ chế bóc tách kép (Dual-Mode Extraction):
  - Khóa JSON `antigravityAuthStatus` (Mode 1): Trích xuất tên, email, gói `Google AI Ultra` / `Google AI Pro`.
  - Khóa Protobuf Base64 `antigravityUnifiedStateSync.userStatus` (Mode 2 Fallback): Regex email RFC 5322 và text signature.
- Bảo mật Zero-Credential: Không lưu trữ mật khẩu Gmail, không lưu API key hay OAuth token vào CSDL.

#### `storage/repository.go`
- Cung cấp các phương thức truy vấn tối ưu cho Web Dashboard và tiến trình ngầm:
  - `CalculateTokensCostUSD(model, prompt, output, thinking, cached)`: **Động cơ FinOps** định giá theo phân lớp Ultra ($2.50 prompt, $10 output, $0.625 cache), Pro ($1.25 prompt, $5 output, $0.3125 cache), Flash ($0.075 prompt, $0.30 output, $0.01875 cache), tự động tính số tiền tiết kiệm 75% nhờ Context Cache và giá trị Gross tương đương.
  - `GetAccountProfile()`: Lấy thông tin tài khoản, chu kỳ thuê bao và trạng thái đếm ngược TTL token.
  - `GetSummaryMetrics(timeRange)`: Thống kê tổng token, prompt, output, thinking tokens, tỷ lệ cache, số lượt gọi và các chỉ số FinOps (`estimated_cost_usd`, `estimated_savings_usd`, `equivalent_gross_usd`) theo khoảng thời gian (`24h`, `7d`, `30d`, `all`).
  - `GetTimeSeriesDataByRange(timeRange)`: Lấy chuỗi thời gian phân bổ token và cuộc gọi.
  - `GetDailySummariesByRange(timeRange)`: Lấy dữ liệu tổng hợp theo từng ngày kèm chi phí `estimated_cost_usd` và tiết kiệm `estimated_savings_usd` cho Bảng Lịch Sử Chi Tiết.
  - `GetModelDistribution(timeRange)`: Thống kê phân bổ tỷ lệ phần trăm token, lượt gọi và chi phí USD theo từng Model AI.
  - `GetModelTimeSeries(timeRange, modelFilter)`: Lấy chuỗi thời gian chi tiết theo model được chọn phục vụ ECharts.
  - `GetAgentFleetSummary(timeRange ...string)`: Tính toán 4 chỉ số KPI của Fleet (`TotalFleet`, `ActiveConcurrency`, `TaskSuccessRate`, `OffloadedTokens`) kèm bộ lọc thời gian (`24h`, `7d`, `30d`, `all`) và cơ chế **TTL Auto-Sweep 2 phút**.
  - `GetAgentConcurrencyTimeline(timeRange ...string)`: Nhóm các tác vụ thực tế theo giờ hoặc theo ngày tùy dải thời gian để vẽ biểu đồ Concurrency Track.
  - `GetAgentGanttTasks(groupBy string, timeRange ...string)`: Truy xuất danh sách tiến trình Gantt thực tế gom theo vai trò (`roles`) hoặc theo phiên (`sessions`), có hỗ trợ lọc theo dải thời gian.
  - `GetAgentTopologyGraph(projectFilter string, timeRange ...string)`: Truy xuất cấu trúc mạng lưới topo đa tác nhân (Topology Network Graph) kết nối Root Agent, các Workspace Projects và 5 vai trò Subagent kèm số lượng tác vụ và token offloaded, hỗ trợ lọc theo dự án cụ thể và cửa sổ thời gian (`24h`, `7d`, `30d`, `all`).
  - `InsertUsageBatch(events)` & `InsertAgentTaskBatch(tasks)`: Ghi gom hàng loạt bản ghi theo transaction an toàn.
  - `RollupHourlyMetrics()`: Chạy logic gộp tự động từ `token_usage_logs` sang `token_usage_hourly_rollup`.

#### `storage/backup.go` & `storage/backup_test.go`
- **Động cơ Auto-Backup chuẩn SQLite Enterprise**:
  - `CreateBackup(destPath)`: Sử dụng câu lệnh native `VACUUM INTO 'destPath'` của SQLite. Tự động hợp nhất (checkpoint & merge) toàn bộ dữ liệu đang ghi trong Write-Ahead Log (`.db-wal`) vào 1 file `.db` đích độc lập hoàn hảo.
  - **Không khóa tiến trình ghi (Non-blocking)**: Các giao dịch từ IDE hay API tiếp tục ghi vào WAL mà không bị trễ hay timeout.
  - **Tránh xung đột Google Drive**: File backup sinh ra là file đơn, không sinh file phụ `.db-wal` hay `.db-shm` trong thư mục backup.
  - `PerformAutoBackup()`: Tự động tạo snapshot theo ngày `token_monitor_backup_YYYYMMDD.db` (mỗi ngày đúng 1 file duy nhất, cập nhật dữ liệu mới nhất liên tục trong ngày), đồng thời duy trì bản sao chuẩn hóa `data/backup/token_monitor.db` phục vụ khôi phục nhanh 1-click.
  - `PruneOldBackups(backupDir, maxKeep)`: Tự động quét và xoay vòng dọn dẹp các bản backup cũ, bảo đảm thư mục backup chỉ giữ đúng số lượng bản sao lưu của 7 ngày gần nhất (`max_keep = 7`). Tổng dung lượng thư mục backup luôn cố định $\le 86\text{ MB}$, tuyệt đối không gây đầy ổ đĩa.
- Bộ kiểm thử tự động `storage/backup_test.go`:
  - `TestBackup_CreateBackup_Integrity`: Xác minh tính toàn vẹn 100% dữ liệu, số lượng bản ghi bảng `agent_fleet_telemetry` và bảo đảm không phát sinh file rác `-wal`/`-shm`.
  - `TestBackup_PerformAutoBackup_And_RetentionPrune`: Kiểm thử cơ chế xoay vòng dọn dẹp file cũ khi số lượng backup vượt quá `max_keep`.
  - `TestBackup_PerformAutoBackup_EndToEnd`: Kiểm thử toàn trình end-to-end từ tạo snapshot đến file chuẩn hóa.

#### `collector/tailer.go`
- Quét định kỳ các thư mục `~/.gemini/*/brain` theo chu kỳ `poll_interval_seconds` (mặc định 10s).
- **Bộ lọc thư mục an toàn**: Tự động bỏ qua các thư mục con có tên chứa `"backup"`, `"tmp"`, hoặc `"profile"`.
- **Cơ chế Byte Offset thông minh & 30-min Lookback**: Nhận diện file mới sửa đổi để nạp tức thì 32 KB gần nhất.
- **Tiến trình Backfill toàn diện (`BackfillAllHistory`)**: Quét toàn bộ 154 thư mục hội thoại và nạp dữ liệu thật vào SQLite.
- **Bộ phân loại 5 Vai trò Subagent (`MapToolToRole`)**:
  - 🔍 `Research Agent`: `search_web`, `read_url_content`
  - 📦 `Codebase Explorer`: `grep_search`, `list_dir`, `view_file`
  - ⚡ `Self-Branch Worker`: `write_to_file`, `replace_file_content`, `multi_replace_file_content`
  - ✅ `Verification Tester`: `run_command`, `browser_subagent`
  - 🛡️ `PKI Auditor`: `manage_task`, `schedule`, `ask_question`, `generate_image`
- **Mô hình ước tính Context Window thực tế (`EstimatePromptTokens`)**: Đường cong 3 giai đoạn (16k -> 52k -> 85k trần).

#### `collector/buffer.go`
- Hàng đợi Thread-safe sử dụng Go Channel có kích thước đệm 1,000 sự kiện (`capacity = 1000`).
- Thuật toán Flush kép: Xả dữ liệu vào SQLite khi hàng đợi đạt 100 bản ghi **hoặc** sau mỗi 1 giây timeout.
- Cơ chế xả trực tiếp `FlushDirect(events)` phục vụ nạp lô lớn khi Backfill lịch sử mà không chiếm giữ channel.

#### `collector/codex_monitor.go`
- Quét và nạp dữ liệu phiên làm việc từ `~/.codex/sessions/**/*.jsonl`.
- Bóc tách token (`input_tokens`, `output_tokens`, `reasoning_tokens`, `cached_input_tokens`), tỷ lệ thành công công cụ (`tool_success_percent`), rate limits (5h & 7d window).
- Tạo đồ thị Topology phân cấp 4 tầng cho OpenAI Codex (`Graph()`).
- Bảo mật tuyệt đối: Không truy cập `auth.json`, không lưu hay hiển thị prompt/command/response.

#### `collector/claude_monitor.go`
- Quét và phân tích log dự án từ `~/.claude/projects/**/*.jsonl`.
- Bóc tách token (`input_tokens`, `output_tokens`, `thinking_tokens`, `cache_read_tokens`), phân tích tần suất sử dụng CLI Tools (`Bash`, `FileEdit`, `Glob`, `Grep`).
- Tạo đồ thị Topology phân cấp 4 tầng cho Anthropic Claude (`Graph()`).
- Bảo mật tuyệt đối: Quyền chỉ đọc, không lưu credential hay nội dung hội thoại nhạy cảm.

#### `web/handler.go` & `web/static/index.html`
- Cung cấp danh mục toàn diện 24 routes chính thức (gồm 20 REST APIs nghiệp vụ, 1 Health Check, và 3 routes phục vụ giao diện & tài liệu tĩnh):
  - `GET /healthz`: Kiểm tra trạng thái máy chủ Web (`{"status": "UP", "timestamp": "..."}`).
  - `GET /api/account`: Trả về hồ sơ tài khoản, gói cước và TTL đếm ngược phiên làm việc Antigravity.
  - `GET /api/metrics/summary`: Trả về tổng quan token Antigravity kèm chỉ số FinOps. Hỗ trợ query `?range=today|24h|7d|30d|all`.
  - `GET /api/metrics/timeseries`: Trả về chuỗi thời gian cho biểu đồ Chart.js Antigravity. Hỗ trợ query `?range=...`.
  - `GET /api/metrics/daily`: Trả về số liệu tổng hợp theo ngày kèm chi phí quy đổi USD. Hỗ trợ query `?range=today|30d` hoặc `?days=30`.
  - `GET /api/metrics/models`: Trả về tỷ lệ phần trăm token và cuộc gọi theo từng Model Gemini. Hỗ trợ query `?range=...`.
  - `GET /api/metrics/models/timeseries`: Trả về chuỗi thời gian chi tiết theo model Gemini cho Apache ECharts.
  - `GET /api/agents/summary`: Trả về các chỉ số KPI của Multi-Agent Fleet Antigravity.
  - `GET /api/agents/concurrency`: Trả về chuỗi dữ liệu tải đồng thời theo thời gian.
  - `GET /api/agents/gantt`: Trả về danh sách tác vụ Gantt theo vai trò hoặc phiên.
  - `GET /api/agents/gantt/packets`: Trả về các gói tin tiến trình tác vụ Subagent định dạng timeline packets.
  - `GET /api/agents/graph`: Trả về dữ liệu đồ thị Topology mạng lưới đa tác nhân Antigravity. Hỗ trợ query `?range=today|24h|7d|30d|all` và `?project=all|<project_id>`. Khi xem chế độ `all`, hệ thống áp dụng **Quy chuẩn lọc dự án hoạt động trong vòng 1 giờ** (`oneHourAgo = now - 75m`, `LatestActivity`, `IsRunning`), tự động loại bỏ các dự án không hoạt động cho đỡ rối và kích hoạt fallback 1 dự án gần nhất nếu toàn bộ hệ thống nhàn rỗi.
  - `GET /api/openai/dashboard`: Trả về dữ liệu tổng hợp telemetry của OpenAI Codex.
  - `GET /api/openai/graph`: Trả về dữ liệu đồ thị Topology phân cấp 4 tầng của OpenAI Codex.
  - `POST /api/openai/refresh`: Kích hoạt quét lại log phiên Codex cục bộ.
  - `GET /api/claude/dashboard`: Trả về dữ liệu tổng hợp telemetry của Anthropic Claude Code CLI.
  - `GET /api/claude/graph`: Trả về dữ liệu đồ thị Topology phân cấp 4 tầng của Anthropic Claude.
  - `POST /api/claude/refresh`: Kích hoạt quét lại log dự án Claude cục bộ.
  - `GET /api/projects/leaderboard`: **Bảng xếp hạng FinOps Đa LLM hợp nhất** (Google Antigravity, OpenAI Codex, Anthropic Claude). Hỗ trợ lọc theo thời gian `?range=today|24h|7d|30d|all` (mặc định: `30d`) và sắp xếp `?sort=tokens|cost|activity` (mặc định: `tokens`).
  - `GET|POST /api/sync/history`: Kích hoạt quét và đồng bộ lại toàn bộ lịch sử trò chuyện vào SQLite.
  - `POST /api/test/simulate`: Endpoint phát sự kiện mẫu thử nghiệm.
  - `GET /docs/`: Phục vụ xem tài liệu kỹ thuật và sơ đồ kiến trúc tương tác.
  - `GET /config.yaml`: Phục vụ file cấu hình raw YAML cho Cổng tra cứu Offline Docs Portal.
  - `GET /`: Phục vụ giao diện Web Dashboard hoàn chỉnh với **Tab 1 🌐 Tổng Hợp Đa LLM ở vị trí đầu tiên**, **3 Tab AI Độc Lập (Google Antigravity, OpenAI Codex, Anthropic Claude) đạt 100% tính năng đồng nhất**, **1 bộ điều khiển thời gian duy nhất**, và **nhận diện Active Concurrency thực tế 0/16**.
- Giao diện Web Dashboard (`web/static/index.html`):
  - **Tab 1: 🌐 Tổng Hợp Đa LLM**: Đặt làm tab đầu tiên và mặc định khi mở Dashboard. Cung cấp 5 thẻ KPI toàn hệ thống (Grand Tokens, Grand Cost USD, #1 Heavy Consumer, #1 Most Active, Overall Cache Hit %), Bảng xếp hạng FinOps Leaderboard trực quan kèm thanh tiến độ tỷ lệ % và badge nhà cung cấp, 3 nút chuyển đổi sắp xếp tức thì (`[⚡ Theo Token]`, `[💰 Theo Chi Phí USD]`, `[🔥 Theo Mức Độ Hoạt Động]`), cùng 2 biểu đồ so sánh trực quan ECharts (Stacked Bar Chart phân bổ token theo LLM và Donut Chart phân chia chi phí giữa các dự án).
  - **6 thẻ KPI Metric Cards đỉnh cao**: Đồng bộ và tự động làm mới trên cả 3 tab AI.
  - **Hộp thoại FinOps Modal tương tác**: Tự động chuyển đổi sang Preset giá cước tương ứng (Gemini Pro/Flash, GPT-5.6 Sol / GPT-4o, Claude 3.7 / 3.5 Sonnet).
  - **Bộ điều khiển thời gian đơn nhất**: 1 cụm 5 mốc thời gian duy nhất tại thanh sub-nav trên cùng (`today`, `24h`, `7d`, `30d`, `all`).
  - **Nhận diện trạng thái Concurrency chân thực**: Tự động hiển thị `0 / 16 (Idle • Đã tắt ứng dụng)` và badge `STANDBY` khi app đóng.
  - **Sơ đồ Topology 60 FPS chuẩn hóa & 3 Chế độ bố cục (Layout Modes)**:
    * **📌 Cố Định (`pinned`)**: Phân cấp 4 tầng kim tự tháp với tọa độ Golang tính sẵn ($canvasCX=1500, stepX=780$), gán `fixed: true` ổn định 100%.
    * **🧲 Tự Do (`force`)**: Áp dụng **Quy luật tương tác vật lý động 4 tầng (Force Physics Scaling Law)** ($N > 40 \to 2200$, $N > 25 \to 1800$, $N > 12 \to 1200$, $N \le 12 \to 800$, `edgeLength: [100, 200] - [180, 350]`, `gravity: 0.03 - 0.06`, `initLayout: 'circular'`, `friction: 0.65`). Tuân thủ nghiêm ngặt **Quy tắc cách ly tọa độ tuyệt đối** (`x: undefined, y: undefined, fixed: false`) ngăn chặn hiện tượng văng vào góc màn hình.
    * **⭕ Vòng Tròn (`circular`)**: Phân bổ đối xứng vòng tròn đồng tâm, `rotateLabel: true`, cách ly tọa độ và căn giữa hoàn hảo.
  - **Responsive Auto-Fit & Layout-Specific Camera Decoupling**: Động cơ `calculateTopologyAutoFit` tự động xác định bounding box và căn giữa $[midX, midY]$. Phân rã độc lập camera ECharts: chế độ Force/Circular bắt buộc gán `center: ['50%', '50%']` và `zoom: 0.85`, trong khi chế độ Pinned sử dụng `fitConfig.center` và `fitConfig.zoom`.
  - **Triệt Tiêu Nhiễu Thị Giác (Zero Visual Redundancy)**: Tinh gọn kích thước nốt ở chế độ Tự Do (Root 44px, Project 34px, Orch 28px, Subagent 22px), đường nối thanh mảnh (`width: 0.8 - 1.8px`), và loại bỏ hoàn toàn huy hiệu chữ tĩnh `⚡ RUNNING` pill đè dưới chân node, chỉ báo trạng thái hoạt động độc quyền qua vòng sóng nhịp radar lan tỏa và luồng hạt photon 60 FPS.
  - **Căn chỉnh nhãn đường cong chuẩn xác (`edgeLabel`) & 2 chế độ nhãn**: Sử dụng thuộc tính chuẩn `edgeLabel` với `position: 'middle'`, neo nhãn số liệu bám sát chính xác vị trí trung điểm đường cong Bezier. Tọa độ trung điểm `(lx, ly)` trên canvas overlay `#topo-flow-overlay` được đồng bộ qua ma trận biến đổi tọa độ toàn cục `transformCoordToGlobal`, đảm bảo không bao giờ bị lệch vị trí khi Zoom / Pan / Roam. Hỗ trợ chế độ tinh gọn `🏷️ Gọn Gàng` (hiện nhãn khi hover) và chế độ `📑 Hiện Tất Cả` (hiện nhãn tĩnh trên mọi đường truyền).
  - **Tự động chuyển đổi theo AI Provider (`syncAgentFleetControlsForProvider`)**: Khi chuyển sang tab OpenAI Codex hoặc Anthropic Claude, tự động ẩn các nút Dual View, Concurrency, Gantt và chuyển ngay sang chế độ Topology Graph tương ứng (`/api/openai/graph`, `/api/claude/graph`). Khi quay lại Google Antigravity, toàn bộ các nút điều khiển được tự động khôi phục.
  - **Dynamic Type-Hints**: Tự động cập nhật tooltip tương tác trên Header (`updateCodexHeader`, `updateClaudeHeader`) phản ánh đúng thông tin kỹ thuật của từng provider.

### 3.2. Đặc Tả Chi Tiết Endpoint Bảng Xếp Hạng Đa LLM (`GET /api/projects/leaderboard`)

Endpoint `GET /api/projects/leaderboard` là cầu nối API tổng hợp FinOps đa nhà cung cấp, hợp nhất số liệu tiêu thụ token, chi phí quy đổi USD, số cuộc gọi/phiên và số tác vụ agent từ cả 3 nguồn dữ liệu: Google Antigravity (CSDL SQLite WAL), OpenAI Codex (`~/.codex/sessions`), và Anthropic Claude (`~/.claude/projects`).

#### A. Tham Số Truy Vấn (Query Parameters)
| Tham Số | Kiểu | Giá Trị Hợp Lệ | Mặc Định | Mô Tả |
| :--- | :---: | :--- | :---: | :--- |
| `range` | string | `today`, `24h` (hoặc `1d`), `7d`, `30d` (hoặc `month`), `all` | `30d` | Cửa sổ thời gian lọc dữ liệu. Không phân biệt hoa thường và tự động trim khoảng trắng. Giá trị không hợp lệ trả về HTTP 400 Bad Request. |
| `sort` | string | `tokens`, `cost`, `activity` | `tokens` | Tiêu chí sắp xếp danh sách dự án. `tokens`: Xếp theo tổng token giảm dần; `cost`: Xếp theo chi phí quy đổi USD giảm dần; `activity`: Xếp theo tổng lượt gọi + tác vụ giảm dần. |

#### B. Cấu Trúc DTO Dữ Liệu Trả Về (Response JSON Schema)
```json
{
  "time_range": "30d",
  "sort_by": "tokens",
  "generated_at": "2026-09-14T07:45:00Z",
  "kpis": {
    "grand_total_tokens": 58941200,
    "grand_total_cost_usd": 12.4582,
    "grand_total_savings_usd": 37.3746,
    "grand_total_calls": 348,
    "grand_total_tasks": 126,
    "grand_total_activity": 474,
    "total_projects_count": 4,
    "active_projects_count": 1,
    "overall_cache_hit_percent": 72.4,
    "top_consumer_project": "TokenMonitor (GoLangDev)",
    "top_consumer_tokens": 42150000,
    "top_consumer_percent": 71.5,
    "top_active_project": "TieuChuanHardeningLinux (Security Standards)",
    "top_active_count": 210,
    "google_total_tokens": 45120000,
    "google_total_cost_usd": 8.1250,
    "google_total_percent": 76.5,
    "openai_total_tokens": 12280000,
    "openai_total_cost_usd": 3.8500,
    "openai_total_percent": 20.8,
    "claude_total_tokens": 1541200,
    "claude_total_cost_usd": 0.4832,
    "claude_total_percent": 2.7
  },
  "projects": [
    {
      "rank": 1,
      "percent_of_top": 100.0,
      "project_id": "proj-tokenmonitor",
      "project_name": "TokenMonitor (GoLangDev)",
      "workspace": "GoLangDev/TokenMonitor",
      "status": "RUNNING",
      "total_tokens": 42150000,
      "prompt_tokens": 32000000,
      "output_tokens": 6150000,
      "cached_tokens": 24000000,
      "thinking_tokens": 4000000,
      "estimated_cost_usd": 7.8540,
      "estimated_savings_usd": 23.5620,
      "total_calls": 210,
      "agent_tasks": 65,
      "total_activity": 275,
      "cache_hit_percent": 75.0,
      "token_share_percent": 71.5,
      "cost_share_percent": 63.0,
      "google_breakdown": {
        "tokens": 35000000,
        "cost_usd": 6.2500,
        "savings_usd": 18.7500,
        "activity": 220,
        "percentage": 83.0,
        "calls": 170,
        "tasks": 50
      },
      "openai_breakdown": {
        "tokens": 6000000,
        "cost_usd": 1.4500,
        "savings_usd": 4.3500,
        "activity": 45,
        "percentage": 14.2,
        "calls": 35,
        "tasks": 10
      },
      "claude_breakdown": {
        "tokens": 1150000,
        "cost_usd": 0.1540,
        "savings_usd": 0.4620,
        "activity": 10,
        "percentage": 2.8,
        "calls": 5,
        "tasks": 5
      }
    }
  ]
}
```

#### C. Bất Biến Toán Học Cốt Lõi (Core Mathematical Invariants)
- **Chuẩn hóa tỷ lệ nhà cung cấp**: Đối với mỗi dự án có `TotalTokens > 0`, tổng phần trăm 3 nhà cung cấp luôn bằng 100%:
  $$\text{GoogleBreakdown.Percentage} + \text{OpenAIBreakdown.Percentage} + \text{ClaudeBreakdown.Percentage} \equiv 100.0\%$$
- **Tính toàn vẹn của thành phần**:
  $$\text{GoogleBreakdown.Tokens} + \text{OpenAIBreakdown.Tokens} + \text{ClaudeBreakdown.Tokens} \equiv \text{TotalTokens}$$
  $$\text{GoogleBreakdown.Activity} + \text{OpenAIBreakdown.Activity} + \text{ClaudeBreakdown.Activity} \equiv \text{TotalActivity} \equiv \text{TotalCalls} + \text{AgentTasks}$$
- **Thứ hạng và tiến độ so với Top 1**:
  $$\text{PercentOfTop} = \frac{\text{Metric}_{\text{project}}}{\text{Metric}_{\text{rank\#1}}} \times 100.0\% \quad (\text{Rank \#1 luôn có } \text{PercentOfTop} = 100.0\%)$$
- **Zero Mock Data Contract**: Nếu cơ sở dữ liệu rỗng hoặc khoảng thời gian chưa có sự kiện phát sinh, endpoint trả về mã HTTP 200 OK với danh sách mảng rỗng chuẩn `"projects": []` (không bao giờ trả về null) và toàn bộ các trường KPI bằng 0 trung thực.

---

## 4. Hướng Dẫn Tùy Chỉnh Khi Chuyển Sang Máy Mới

Khi bạn cài đặt TokenMonitor trên một máy tính khác:
1. **Kiểm tra đường dẫn thư mục Antigravity**:
   - Nếu cài đặt ở đường dẫn mặc định trên Windows, ứng dụng sẽ tự động phát hiện thư mục `C:\Users\<Tên_User>\.gemini` và `%APPDATA%\Antigravity IDE` (hoặc `%APPDATA%\Antigravity`).
   - Nếu bạn chuyển thư mục dữ liệu sang ổ đĩa khác (ví dụ ổ `D:\` hoặc `E:\`), hãy mở `config.yaml` và cập nhật lại đường dẫn tương ứng:
     ```yaml
     local_tailer:
       ide_brain_dir: "E:\\YourPath\\.gemini\\antigravity\\brain"
       ide_conv_db_dir: "E:\\YourPath\\.gemini\\antigravity\\conversations"
     ```
2. **Đổi cổng Dashboard nếu bị trùng**:
   - Nếu cổng `9090` đang được một ứng dụng khác sử dụng (ví dụ Prometheus), bạn có thể đổi sang cổng bất kỳ (ví dụ `9095` hoặc `8888`):
     ```yaml
     server:
       dashboard_port: 9095
     ```
3. **Khởi động ứng dụng**:
   - Chạy lệnh: `.\token_monitor.exe`
   - Mở trình duyệt truy cập: `http://localhost:9095`

---

## 5. Đánh Giá Mức Độ Đáp Ứng Tiêu Chí Giám Sát (Local 01 Máy) & Lộ Trình Nâng Cấp

### 5.1. Bảng Ma Trận Đáp Ứng Tiêu Chí Giám Sát Cục Bộ (01 Máy Local)
Hệ thống TokenMonitor trên 01 máy trạm hiện tại đã **đáp ứng 100% tất cả các tiêu chí kỹ thuật** cho việc quan sát, quản lý và tối ưu chi phí:

| STT | Tiêu Chí Giám Sát | Hiện Trạng Thực Tế trong TokenMonitor | Đánh Giá |
| :---: | :--- | :--- | :---: |
| **01** | **Giám sát Lưu lượng Token** | Đo lường chi tiết 5 chỉ số: Prompt Tokens, Candidates Output Tokens, Thinking Tokens, Cached Tokens và Grand Total. Phân tách theo từng phiên và theo từng model AI (Gemini 2.5 Flash / Pro). | **ĐẠT (100%)** |
| **02** | **Giám sát Tài Chính (FinOps)** | Bảng quy đổi trực tiếp sang $ USD và VNĐ, phân tách chi phí theo ngày, tính toán khoản tiết kiệm nhờ Cache Hit Rate ($/1M tokens) và cung cấp Bộ máy tính FinOps tương tác. | **ĐẠT (100%)** |
| **03** | **Giám sát Multi-Agent Fleet** | Tự động phân loại 5 vai trò Subagents (Research, Codebase, Worker, Tester, Auditor), đo lường Active Concurrency, Task Success Rate, Gantt Timeline và Sơ đồ mạng lưới Topology Network Graph. | **ĐẠT (100%)** |
| **04** | **Trực Quan Hóa & UX** | Dashboard Web Glassmorphism, 4 View nghiệp vụ, Nút Gốc Root Controller (Level 0) điều phối 3 Project Hubs, ECharts Topology Graph phân cấp 4 tầng, Smart Edge Labels ẩn khoảng trống, Midpoint Bezier Pill Badges khi hover, Zero Fake Motion tĩnh lặng chuẩn xác khi dừng, và Floating Type-Hint Tooltips. | **ĐẠT (100%)** |
| **05** | **Thời Gian Thực (Real-Time)** | Cập nhật số liệu liên tục qua Server-Sent Events (SSE) và bộ đếm polling 5 giây mà không cần người dùng F5 tải lại trang. | **ĐẠT (100%)** |
| **06** | **Lọc Thời Gian Đa Khung** | Cung cấp 5 bộ lọc thời gian độc lập (`today`, `24h`, `7d`, `30d`, `all`) đồng bộ 2 chiều tức thời giữa cụm toolbar Topology Graph, thẻ KPI, biểu đồ Chart.js, bảng lịch sử và toàn bộ cụm Multi-Agent Fleet Telemetry. | **ĐẠT (100%)** |
| **07** | **Bảo Mật & Tuân Thủ Chính Sách** | Thu thập log 100% thụ động (`local_tailer`), không can thiệp proxy MITM, không chặn bắt gói tin mạng của IDE, đọc file CSDL `state.vscdb` ở chế độ `mode=ro` (chỉ đọc). | **ĐẠT (100%)** |
| **08** | **Hiệu Năng & Độ Bền ACID** | Sử dụng SQLite Pure Go (`modernc.org/sqlite`, `CGO_ENABLED=0`) với chế độ WAL (Write-Ahead Logging), Connection Pooling an toàn, TTL Auto-Sweep dọn rác stale task sau 2 phút. | **ĐẠT (100%)** |
| **09** | **Tự Động Sao Lưu & Phục Hồi Thảm Họa (Disaster Recovery)** | Tự động sao lưu định kỳ qua SQLite `VACUUM INTO`, hợp nhất toàn bộ WAL vào 1 file snapshot độc lập không khóa ghi (Non-blocking), xoay vòng dọn dẹp bản cũ (`max_keep = 7`), khôi phục 1-click tức thì (RPO <= 60p, RTO < 10s). | **ĐẠT (100%)** |

### 5.2. Các Hướng Nâng Cấp Tiềm Năng (Tùy Chọn Mở Rộng Khi Có Nhu Cầu)
Đối với **01 máy local hiện tại**, hệ thống đã hoàn toàn khép kín và đầy đủ. Tuy nhiên, nếu muốn mở rộng quy mô hoặc tích hợp sâu hơn, các module sau có thể được kích hoạt/nâng cấp:
1. **Cảnh Báo Thông Minh Qua Telegram / Discord Bot**:
   - *Mục tiêu*: Tự động bắn tin nhắn cảnh báo khi có Subagent lặp vô tận (Loop Detection) hoặc khi một phiên làm việc tiêu thụ vượt ngưỡng chi phí đặt trước (ví dụ > $1.00 USD/hội thoại).
   - *Hiện trạng*: Khung cấu hình `alerting.telegram` đã có sẵn trong `config.yaml`, chỉ cần kích hoạt webhook token.
2. **Xuất Báo Cáo Dữ Liệu Định Kỳ (CSV / Excel FinOps Export)**:
   - *Mục tiêu*: Nút bấm xuất file `.csv` hoặc `.xlsx` phục vụ việc quyết toán chi phí, lưu trữ kế toán hoặc báo cáo tuần/tháng cho đội ngũ quản lý.
3. **Mô Hình Giám Sát Đa Máy Trạm / Server CLI (Multi-Node Gateway)**:
   - *Mục tiêu*: Tập trung dữ liệu từ nhiều máy trạm phát triển hoặc các máy chủ chạy CLI tự động về duy nhất một Dashboard TokenMonitor trung tâm (tham khảo hướng dẫn cấu hình Reverse Proxy cổng `8080` hoặc triển khai binary Linux `token_monitor_linux` trên server trong tài liệu Troubleshooting).

---

## 6. Cơ Chế Tự Động Sao Lưu & Phục Hồi Thảm Họa (Auto-Backup & Disaster Recovery)

### 6.1. Tại Sao Không Thể Dùng File Copy Đơn Thuần Với SQLite WAL?
Khi SQLite hoạt động ở chế độ `WAL (Write-Ahead Logging)`, dữ liệu của hệ thống phân bố trên **3 file đồng thời**:
1. `token_monitor.db`: File CSDL gốc lưu trữ dữ liệu đã được checkpoint.
2. `token_monitor.db-wal`: Nhật ký ghi tuần tự lưu toàn bộ các câu lệnh `INSERT`/`UPDATE` mới phát sinh.
3. `token_monitor.db-shm`: Bộ nhớ chia sẻ phối hợp các con trỏ đọc/ghi đồng thời.

Nếu sao chép thủ công bằng lệnh hệ điều hành (`copy` hoặc `cp`) khi ứng dụng đang chạy:
- Dữ liệu giữa `.db` và `.db-wal` sẽ bị lệch trạng thái (inconsistent state), dẫn đến file backup bị hỏng cấu trúc (corrupt database).
- File `-wal` và `-shm` rời rạc trong thư mục backup sẽ gây xung đột đồng bộ Google Drive (sinh ra các file rác như `token_monitor (1).db-shm`).

### 6.2. Cơ Chế SQLite `VACUUM INTO` Đạt Chuẩn Enterprise
TokenMonitor áp dụng lệnh native **`VACUUM INTO 'destPath'`**:
- **Non-blocking Write**: Tiến trình thu thập log và API của TokenMonitor vẫn tiếp tục đọc và ghi dữ liệu bình thường, không xảy ra hiện tượng nghẽn hay khóa cơ sở dữ liệu (`busy_timeout`).
- **Atomic WAL Checkpoint & Merge**: SQLite tự động lấy bản chụp nhất quán (consistent snapshot) tại thời điểm gọi lệnh, hợp nhất toàn bộ dữ liệu từ file `.db-wal` vào file đích và tối ưu hóa phân mảnh (defragmentation).
- **Single Independent File**: File sinh ra trong thư mục `data/backup/` là **1 file `.db` hoàn chỉnh duy nhất**, không cần kèm theo file `-wal` hay `-shm`, an toàn 100% với các công cụ sao lưu đám mây (Google Drive, Dropbox, OneDrive).

### 6.3. Quy Trình Khôi Phục Dữ Liệu Khi Gặp Sự Cố (Disaster Recovery Runbook)
Trong trường hợp máy tính bị mất điện đột ngột hoặc file CSDL chính bị hỏng:
1. **Dừng tiến trình TokenMonitor**:
   ```powershell
   Stop-Process -Name token_monitor -Force
   ```
2. **Khôi phục từ bản sao lưu mới nhất**:
   Sao chép file [data/backup/token_monitor.db](file:///e:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor/data/backup/token_monitor.db) đè vào file chính [data/token_monitor.db](file:///e:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor/data/token_monitor.db):
   ```powershell
   Remove-Item .\data\token_monitor.db-wal, .\data\token_monitor.db-shm -ErrorAction SilentlyContinue
   Copy-Item .\data\backup\token_monitor.db -Destination .\data\token_monitor.db -Force
   ```
3. **Khởi động lại TokenMonitor**:
   ```powershell
   .\token_monitor.exe -config config.yaml
   ```
- **Thời gian phục hồi (RTO - Recovery Time Objective)**: Dưới 10 giây.
- **Mức độ mất mát tối đa (RPO - Recovery Point Objective)**: Tối đa bằng chu kỳ `interval_minutes` trong `config.yaml` (mặc định 60 phút; có thể hạ xuống 15 phút hoặc 30 phút tùy nhu cầu).


