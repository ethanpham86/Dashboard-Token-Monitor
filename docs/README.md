# Trung Tâm Tài Liệu & Sơ Đồ Kỹ Thuật - TokenMonitor

> **TokenMonitor**: Hệ thống AI Observability & FinOps Daemon giám sát hạn mức Token & Quota AI thời gian thực dành cho môi trường lập trình tự hành (Agentic Coding Loop).  
> **Kiến trúc**: Golang Daemon (ModernC Pure Go SQLite, WAL Mode) + Web Dashboard Glassmorphism + Master Offline Documentation Portal.  
> **Phiên bản**: v1.0 Production | **Mã nguồn**: 100% Read-Only Ground Truth Parity | **Milestone M10**: 100% Code-to-Doc Synchronization & Master Offline Portal Re-compilation.  

---

## 1. Truy Cập Nhanh Tài Liệu & Sơ Đồ Trực Quan

Toàn bộ tài liệu chính và sơ đồ tương tác được đặt trực tiếp tại thư mục gốc `docs/` theo chuẩn **Root Placement Contract**:

| Tài Liệu / Sơ Đồ | Loại Tệp | Nội Dung & Đặc Tả Kỹ Thuật |
| :--- | :--- | :--- |
| [**Cổng Tra Cứu Duy Nhất (Master Portal)**](./index.html) | `HTML` | **Cổng hiển thị trung tâm tự chứa 100% (Self-Contained Offline Portal)**, nhúng sẵn toàn bộ sơ đồ Archify, cẩm nang cấu hình, đặc tả CSDL, 7 phase Runbook và các file reference. Miễn nhiễm 100% lỗi CORS trên giao thức `file:///`. |
| [**Sơ Đồ Kiến Trúc Hệ Thống**](./tokenmonitor-architecture.html) | `HTML` | **Sơ đồ kiến trúc 3 tầng tương tác Archify**: Tầng Nguồn (Antigravity IDE), Tầng Đệm & Lưu Trữ (Async Buffer, SQLite WAL), Tầng Trình Diễn (Web Dashboard & REST API). |
| [**Sơ Đồ Luồng Dữ Liệu 6 Trạm**](./tokenmonitor-dataflow.html) | `HTML` | **Sơ đồ luồng thu thập dữ liệu 6 trạm tương tác Archify**: Thể hiện đầy đủ nhãn giao tiếp kỹ thuật, chu kỳ quét và cơ chế xả lô (Batching). |
| [**Sơ Đồ ERD Database (Interactive)**](./tokenmonitor-database-erd.html) | `HTML` | **Sơ đồ ERD CSDL tương tác Archify**: 5 bảng quan hệ 1:N, 7 chỉ mục chiến lược B-Tree và cấu hình SQLite WAL Mode tải cao. |
| [**Sơ Đồ Đa Dự Án & Dòng Chảy 60 FPS**](./workspace-overview.html) | `HTML` | **Sơ đồ toàn cảnh tương tác Multi-Agent Fleet**: Phối hợp hạm đội song song 4 cụm dự án độc lập (`MCREDIT ↔ TieuChuanHardeningLinux ↔ TokenMonitor ↔ ProjectScriptOS`), thuật toán phân giải dự án động 4 tầng (`resolveSubagentProject`), phân tách cụm ngang 780px và hiệu ứng dòng chảy năng lượng 60 FPS GPU-accelerated. |
| [**Kiến Trúc Hạm Đội Đa Tác Nhân**](./TokenMonitor_Team_Agent_Fleet_Architecture.md) | `Markdown` | **Đặc tả toàn diện kiến trúc Team Agent Fleet**: 4 cụm dự án song song (`TokenMonitor`, `MCREDIT`, `TieuChuanHardeningLinux`, `ProjectScriptOS`), 5 vai trò Subagent, giải thuật phân giải dự án 4 tầng (ưu tiên CWD path matching kèm collision guard `!tokenmonitor`), phân tách cụm ngang 780px, chuẩn Zero Fake Motion (ngưỡng sống 45s), giao thức trao đổi streaming, thuật toán hoạt họa 60 FPS Canvas overlay, cấu trúc CSDL và 5 REST API chuyên biệt. |
| [**Đặc Tả CSDL & Database ERD**](./TokenMonitor_Database_ERD.md) | `Markdown` | Data Dictionary chi tiết cho 5 bảng (`accounts`, `auth_sessions`, `token_usage_logs`, `token_usage_hourly_rollup`, `agent_fleet_telemetry`), ràng buộc khóa ngoại `ON DELETE CASCADE`, 7 chỉ mục hiệu năng, 5 cấp độ xác thực schema, cơ chế phục hồi sau crash (WAL Crash Recovery), và lệnh kiểm tra toàn vẹn CSDL `PRAGMA integrity_check`. |
| [**Cẩm Nang Luồng Dữ Liệu & Logic Xử Lý**](./TokenMonitor_Core_Logic_and_DataFlow.md) | `Markdown` | Hướng dẫn chi tiết chu trình 6 trạm, giải thích tường tận logic lọc `PLANNER_RESPONSE` chống đếm vống, thuật toán ước tính context window [16k, 85k], bóc tách tài khoản sống không hardcode, **Ma trận quy chuẩn xác thực 5 tầng** (Log intake, RAM Buffer, Config, SQLite, REST API), và **6 cơ chế bảo vệ dữ liệu chống lỗi / crash** (WAL, Atomic Transactions, Graceful Shutdown, Idempotency, `mode=ro` IDE safety, Deadlock prevention). |
| [**Đặc Tả Đo Lường Token Agentic**](./TokenMonitor_Token_Estimation_Spec.md) | `Markdown` | Đặc tả thu thập kép (Local Tailer Heuristic vs Reverse Proxy Ground Truth), phân biệt `GENERIC` vs `PLANNER_RESPONSE`, công thức toán học 3 giai đoạn `EstimatePromptTokens`, bóc tách Output/Thinking tokens, và cơ chế Context Caching. |
| [**Hướng Dẫn Cấu Hình Hệ Thống**](./TokenMonitor_Configuration_Guide.md) | `Markdown` | Cẩm nang cấu hình chi tiết `config.yaml`, cơ chế phát hiện tài khoản an toàn qua 3 đường dẫn `state.vscdb`, driver `sqlite_detector`, danh mục 5 bảng, phân loại 5 vai trò Subagent, danh mục phương thức Repository và danh sách 20 REST API (+ 3 static/doc routes, 1 healthz = 24 routes). |
| [**Bộ Runbook Vận Hành Production**](./TokenMonitor_20260908/) | `Thư mục` | 7 tài liệu Runbook chuẩn Production: Điều kiện tiên quyết, Kiến trúc, Triển khai HA, Cài đặt, Tối ưu hiệu năng (TTL sweep 2m), Xử lý sự cố (Active Concurrency), Tuân thủ bảo mật và 4 tài liệu Reference. |

---

## 2. Kiến Trúc Luồng Thu Thập Dữ Liệu 6 Trạm (6-Station Pipeline)

Dữ liệu token được xử lý qua chuỗi 6 trạm tuần tự và tối ưu:

```
[Trạm 1: File Log IDE] ──(1. File I/O 10s)──► [Trạm 2: Local Tailer]
                                                        │
                                          (2. RAM Channel 1,000 cap)
                                                        ▼
[Trạm 4: SQLite WAL] ◄──(3. Batch Insert 100/1s)── [Trạm 3: Async Buffer]
        │
(4. Cron Goroutine 5m) -> RollupHourlyMetrics() & TTL Auto-Sweep (2m)
        │
        ▼
[Trạm 5: Web REST APIs] ──(6. HTTP AJAX 5s)──► [Trạm 6: Dashboard UI]
```

1. **Trạm 1 — Nguồn Dữ Liệu Cục Bộ (Event Log Source)**: IDE Antigravity ghi nhật ký hội thoại vào file `transcript.jsonl` tại thư mục `~/.gemini/antigravity/brain` (và toàn bộ 154 thư mục brain workspace).
2. **Trạm 2 — Trình Thu Thập Log Cục Bộ (`collector/tailer.go`)**:
   - Quét định kỳ 10 giây một lần theo con trỏ byte offset (`fileOffsets`), chỉ nạp phần dữ liệu mới phát sinh.
   - Tự động bỏ qua các thư mục `backup`, `tmp`, `profile`.
   - Lùi 32 KB offset đối với file trò chuyện tích cực trong 30 phút qua để nạp tức thì tin nhắn gần nhất.
   - Tiến trình `BackfillAllHistory` quét ngầm lịch sử và xả trực tiếp lô 200 bản ghi qua `FlushDirect`.
   - Lọc chính xác `step.Source == "MODEL" && step.Type == "PLANNER_RESPONSE"`, loại trừ hoàn toàn các bước công cụ `GENERIC`, `RUN_COMMAND`, `LIST_DIRECTORY`.
   - Bóc tách lệnh điều phối đa agent `invoke_subagent`, phân loại 5 vai trò Subagent (`Research Agent`, `Codebase Explorer`, `Self-Branch Worker`, `Verification Tester`, `PKI Auditor`) và ghi nhận vào `agent_fleet_telemetry`.
   - Tính toán Prompt Tokens theo hàm 3 giai đoạn `EstimatePromptTokens(stepIndex)` (16k -> 52k -> trần 85k).
3. **Trạm 3 — Bộ Đệm RAM Kép (`collector/buffer.go`)**:
   - Go Channel an toàn đa luồng dung lượng 1,000 sự kiện (`capacity = 1000`).
   - Xả lô theo cơ chế kép (Dual Trigger): Khi đủ 100 sự kiện (`batchSize = 100`) **hoặc** sau mỗi 1 giây (`flushTick = 1s`).
   - Khi đầy đệm: Cơ chế Non-blocking drop ghi cảnh báo và bỏ qua, bảo vệ hệ thống không bị nghẽn.
4. **Trạm 4 — Lưu Trữ CSDL & Động Cơ FinOps (`storage/db.go` & `storage/repository.go`)**:
   - Cơ sở dữ liệu SQLite thuần Go (`modernc.org/sqlite`) chế độ WAL, biên dịch không cần CGO (`CGO_ENABLED=0`).
   - Áp dụng 6 PRAGMA tải cao qua cơ chế đăng ký kép `sqlite.RegisterConnectionHook` trong `init()` kết hợp `applyPragmas()` trong `NewStorage()`.
   - 5 bảng chuẩn: `accounts`, `auth_sessions`, `token_usage_logs`, `token_usage_hourly_rollup`, `agent_fleet_telemetry` với ràng buộc `ON DELETE CASCADE`.
   - 7 chỉ mục chiến lược bao gồm Partial Unique Index `idx_token_dedup_chat` và 3 chỉ mục chuyên biệt cho fleet telemetry.
   - Goroutine chạy định kỳ 5 phút gọi phương thức `RollupHourlyMetrics()` gộp dữ liệu sang bảng rollup. Cơ chế tự động quét dọn TTL (2 phút) bảo đảm chỉ số Concurrency phản ánh chính xác thời gian thực.
   - **Động cơ Auto-Backup chuẩn Enterprise (`storage/backup.go`)**: Tự động sao lưu định kỳ qua SQLite `VACUUM INTO`, hợp nhất toàn bộ WAL vào snapshot `.db` nguyên tử không khóa ghi (Non-blocking), dọn dẹp xoay vòng bản cũ (`max_keep = 7`), và duy trì bản sao chuẩn hóa `data/backup/token_monitor.db` phục vụ khôi phục 1-click tức thì.
   - **Động cơ định giá FinOps (`CalculateTokensCostUSD`)**: Phân tách biểu giá theo Ultra Tier ($2.50 prompt, $10 output, $0.625 cache), Pro Tier ($1.25 prompt, $5 output, $0.3125 cache), Flash Tier ($0.075 prompt, $0.30 output, $0.01875 cache), tự động tính toán chi phí USD, khoản tiết kiệm 75% nhờ Context Cache và giá trị Gross tương đương.
5. **Trạm 5 — Máy Chủ REST API (`web/handler.go`)**:
   - Phục vụ toàn bộ dữ liệu thống kê qua giao thức HTTP JSON trên cổng `9090` (20 endpoint REST API chuẩn, 3 routes phục vụ giao diện & tài liệu, cùng 1 endpoint `/healthz` = 24 routes).
   - Tích hợp endpoint tổng hợp FinOps đa LLM `GET /api/projects/leaderboard` (`?range=today|24h|7d|30d|all&sort=tokens|cost|activity`).
   - Tích hợp các trường FinOps mở rộng: `estimated_cost_usd`, `estimated_savings_usd`, `equivalent_gross_usd`.
   - Hỗ trợ đồng bộ tham số thời gian thực (`?range=today|24h|7d|30d|all`) xuyên suốt toàn bộ các API Metrics, Leaderboard và Multi-Agent Fleet Telemetry.
6. **Trạm 6 — Giao Diện Web Trực Quan (`web/static/index.html`)**:
   - **Tab 1: 🌐 Tổng Hợp Đa LLM**: Đặt làm tab đầu tiên và mặc định khi mở Dashboard, cung cấp 5 thẻ KPI toàn hệ thống, Bảng xếp hạng FinOps Leaderboard với 3 chế độ sắp xếp (`tokens`, `cost`, `activity`), thanh tiến độ %, huy hiệu tỷ lệ nhà cung cấp (Google, Codex, Claude), cùng biểu đồ ECharts Stacked Bar và Donut Chart.
   - Dashboard phong cách Glassmorphism 4 góc nhìn kèm **Thẻ KPI thứ 6 `#card-finops-usd`** quy đổi trực tiếp sang USD ($) và VNĐ (₫) trên 3 tab AI Provider độc lập (Google Antigravity, OpenAI Codex, Anthropic Claude).
   - **Bộ Máy Tính FinOps Tương Tác**: Mở nhanh qua nút `💰 Quy Đổi USD` trên Header, hỗ trợ tính token tùy ý (1M, 5M, 10M, 50M), chọn preset mô hình và kéo thanh trượt tỷ lệ Cache Hit.
   - **Cột Est. USD ($) Trong Bảng Lịch Sử Hàng Ngày**: Giúp theo dõi chi tiêu AI theo từng ngày.
    - **Sơ Đồ Mạng Lưới Topo Đa Tác Nhân (Topology Network Graph)**:
      - Phân cấp 4 tầng kim tự tháp (Nút Gốc Root Controller Level 0 điều phối 4 cụm Project Hubs Level 1 qua `ROOT_ORCHESTRATION` vàng kim `#fbbf24`).
      - Cụm toolbar lọc 5 mốc thời gian (`today`, `24h`, `7d`, `30d`, `all`) tích hợp trực tiếp trên đồ thị.
      - Chuẩn mực **Zero Fake Motion** tĩnh lặng tuyệt đối khi dự án dừng: Ngưỡng kích hoạt 45 giây (`collector/tailer.go:577`) kết hợp cơ chế dọn dẹp TTL 2 phút (`storage/repository.go:1004-1010`); khi dự án dừng vẽ đúng 0 mũi tên động, 0 hạt photon (HUD đếm đúng 0 hạt), và 1 resting chevron tĩnh tại $t=0.5$.
      - **Căn Chỉnh Nhãn Bám Sát Đường Cong (`edgeLabel`)**: Chuẩn hóa cấu hình `edgeLabel` trong ECharts series với `position: 'middle'`, tự động neo nhãn số liệu bám sát chính xác vào vị trí trung điểm của đường cong Bezier.
      - **Đồng Bộ Tọa Độ Toàn Cục Sống (`transformCoordToGlobal`)**: Lớp canvas overlay `#topo-flow-overlay` tính toán tọa độ trung điểm nhãn `(lx, ly)` đồng bộ tuyệt đối với ma trận biến đổi tọa độ toàn cục khi người dùng Zoom / Pan / Roam, đảm bảo nhãn và đường nối không bao giờ bị lệch vị trí (Zero Drift).
      - **Hai Chế Độ Xem Nhãn**: Hỗ trợ chế độ tinh gọn `🏷️ Gọn Gàng` (hiện nhãn khi hover) và chế độ `📑 Hiện Tất Cả` (hiện nhãn tĩnh bám sát đường cong trên mọi đường truyền).
      - **Tự Động Chuyển Đổi Theo Provider (`syncAgentFleetControlsForProvider`)**: Khi chuyển sang tab OpenAI Codex hoặc Anthropic Claude, hệ thống tự động ẩn các nút Dual View, Concurrency, Gantt và kích hoạt ngay đồ thị Topology tương ứng (`/api/openai/graph`, `/api/claude/graph`).
      - **Tự Động Nhận Diện & Focus Vào Dự Án Đang Hoạt Động (Active Project Auto-Focus)**: Tự động phát hiện dự án đang có tác vụ `RUNNING` / `ACTIVE` (ví dụ: `TokenMonitor (GoLangDev)`), tự động chọn option trên dropdown và refetch/zoom vào riêng cụm dự án đang chạy mà không cần người dùng thao tác thủ công (`updateTopologyProjectSelect`, `loadAgentFleetData`).
      - **Động Cơ Responsive Auto-Fit & Căn Giữa Động [midX, midY]**: Thuật toán `calculateTopologyAutoFit` tự động xác định bounding box của tập node, căn giữa hình học tại $[midX, midY]$, bổ sung đệm an toàn $160\text{px} \times 150\text{px}$ và co giãn tối ưu theo khung nhìn ($92\%$ width, $84\%$ height). Triệt tiêu hoàn toàn hiện tượng tràn lề ngang hay trôi lệch tọa độ khi tải lại trang (Anti-Drift Storage Contract).
    - Tích hợp Engine Tooltip Type-Hint tương tác nổi (Interactive Floating Type-Hint Tooltip Engine) tự động căn biên và giải thích cặn kẽ từng chỉ số khi hover chuột, đồng bộ động theo từng AI Provider (`updateCodexHeader`, `updateClaudeHeader`).
    - Tự động gọi AJAX làm mới dữ liệu mỗi 5 giây.

---

### 2.1. Khung Độ Tin Cậy Cấp Doanh Nghiệp (Enterprise Reliability Framework)

Để bảo đảm hệ thống vận hành 24/7 không lỗi và dữ liệu luôn chính xác 100%, TokenMonitor triển khai hai trụ cột bảo vệ:

#### A. Ma Trận Quy Chuẩn Xác Thực 5 Tầng (5-Tier Validation Matrix)
1. **Tầng 1 (Log Intake)**: Bóc tách JSON Lines an toàn (`json.Unmarshal`), lọc vị từ `PLANNER_RESPONSE` chống đếm trùng, chặn dưới token không âm ($\ge 15$), kiểm soát cửa sổ trượt $[16.000, 85.000]$ tokens.
2. **Tầng 2 (RAM Buffer)**: Dung lượng channel $1.000$ events, cơ chế xả kép (100 events / 1s), non-blocking push với drop cảnh báo chống deadlock và OOM.
3. **Tầng 3 (Config & IDE)**: Xác thực dải cổng TCP $1 \le \text{port} \le 65535$, timeout $> 0$, quét giải mã Protobuf IDE với dải padding $0..3$ bytes và byte tag `0x7a`.
4. **Tầng 4 (SQLite Schema)**: Ràng buộc `CHECK(token_status)`, `FOREIGN KEY ON DELETE CASCADE`, Partial Unique Index `idx_token_dedup_chat`, Composite Unique trên Rollup, và `NOT NULL DEFAULT 0`.
5. **Tầng 5 (REST API)**: Whitelist dải thời gian (`24h`, `7d`, `30d`, `all`), 100% Parameterized Queries chống SQL Injection, chặn đứng tấn công Path Traversal trên route `/docs/*`.

#### B. 7 Cơ Chế Bảo Vệ Dữ Liệu & Chống Crash (Fault Tolerance Architecture)
1. **SQLite WAL Crash Recovery**: Ghi nhật ký tuần tự ra file `-wal`. Khi sập nguồn đột ngột, SQLite tự động replay log khôi phục dữ liệu nguyên vẹn 100% khi khởi động lại.
2. **Atomic Batch Transactions**: Mọi đợt xả 100 events đều bọc trong giao dịch `BeginTx` -> `defer tx.Rollback()`. Hoặc thành công toàn bộ, hoặc rollback sạch sẽ, không tạo rác dữ liệu.
3. **Graceful Shutdown & Buffer Draining**: Bắt tín hiệu `SIGINT`/`SIGTERM` và xả kiệt toàn bộ sự kiện còn trong RAM Channel xuống CSDL trước khi tắt (Zero Data Loss).
4. **Anti-Deduplication & Idempotency**: Ngăn chặn quét lặp log cũ qua Partial Unique Index và cơ chế Upsert Rollup bảo đảm chạy lại nhiều lần kết quả vẫn đồng nhất.
5. **Zero Intrusion to Host IDE**: Mở SQLite IDE bằng cờ `mode=ro` (chỉ đọc), streaming tailer không khóa file `transcript.jsonl`, bảo vệ IDE không bao giờ bị lock hay crash.
6. **Deadlock & Resource Protection**: `busy_timeout = 5000` triệt tiêu lỗi `database is locked`. Auto-sweep TTL 2 phút tự giải phóng tác vụ treo, và Connection Pooling bảo vệ File Descriptors.
7. **Automated Zero-Downtime Backup (`VACUUM INTO`)**: Tự động sao lưu định kỳ không khóa ghi (Non-blocking), tự động merge toàn bộ WAL vào file snapshot theo ngày `token_monitor_backup_YYYYMMDD.db` (mỗi ngày 1 file duy nhất, cập nhật liên tục), xoay vòng dọn dẹp giữ tối đa 7 ngày gần nhất (`max_keep = 7`), dung lượng cố định $\le 86\text{ MB}$ không bao giờ đầy đĩa, triệt tiêu vĩnh viễn xung đột Google Drive, đạt RPO $\le$ 60 phút và RTO $<$ 10 giây.

---

### 2.2. Động Cơ Định Giá FinOps & Quy Đổi Token Ra USD / VNĐ

TokenMonitor tích hợp bảng giá Pay-As-You-Go chính thức của Google Cloud Vertex AI & Gemini API:
* **Ultra Tier** (`gemini-2.5-pro-ultra`): Prompt **$2.50/1M**, Output & Thinking **$10.00/1M**, Cache Read **$0.625/1M** (tiết kiệm **75%**).
* **Pro Tier** (`gemini-1.5/2.5-pro`): Prompt **$1.25/1M**, Output & Thinking **$5.00/1M**, Cache Read **$0.3125/1M** (tiết kiệm **75%**).
* **Flash Tier** (`gemini-1.5/2.0/2.5-flash`): Prompt **$0.075/1M**, Output & Thinking **$0.30/1M**, Cache Read **$0.01875/1M** (tiết kiệm **75%**).
* **Tỷ giá quy đổi tiền tệ**: $1\text{ USD} = 25,400\text{ VNĐ}$.
* **Hiệu quả đầu tư (ROI)**: Giúp người dùng so sánh chi phí thuê bao phẳng (Flat Rate) với giá trị sử dụng thực tế tương đương (Gross Value), đo lường mức độ sinh lời và tiết kiệm từ Context Caching.

---

## 3. Danh Mục REST API Chuẩn Xác

Toàn bộ 20 endpoint REST API chuẩn (cùng 3 static/doc routes và 1 endpoint `/healthz` = 24 routes) được định tuyến thống nhất qua router chuẩn của `web/handler.go`:

| Phương Thức | Endpoint | Tham Số Query | Mô Tả & Kiểu Dữ Liệu |
| :---: | :--- | :--- | :--- |
| `GET` | `/healthz` | Không | Kiểm tra tính sống còn: `{"status": "UP", "timestamp": "..."}` |
| `GET` | `/api/account` | Không | Hồ sơ tài khoản, gói cước Ultra/Pro, ngày hết hạn và TTL token |
| `GET` | `/api/metrics/summary` | `range=today\|24h\|7d\|30d\|all` | Tổng quan token kèm chi phí FinOps (`estimated_cost_usd`, `estimated_savings_usd`, `equivalent_gross_usd`) |
| `GET` | `/api/metrics/timeseries` | `range=today\|24h\|7d\|30d\|all` | Chuỗi thời gian phục vụ biểu đồ đường kèm phân tách Ultra vs Flash |
| `GET` | `/api/metrics/daily` | `range=today\|30d` hoặc `days=...` | Thống kê theo ngày kèm chi phí quy đổi `estimated_cost_usd` phục vụ bảng lịch sử |
| `GET` | `/api/metrics/models` | `range=today\|24h\|7d\|30d\|all` | Tỷ lệ phần trăm token, lượt gọi và chi phí USD phân bổ theo từng Model AI |
| `GET` | `/api/metrics/models/timeseries` | `range=today\|30d`, `model=...` | Chuỗi thời gian chi tiết theo model cụ thể |
| `GET` | `/api/agents/summary` | `range=today\|24h\|7d\|30d\|all` | Tóm tắt Multi-Agent Fleet: Active Concurrency (với TTL sweep 2 phút), tổng nhiệm vụ, tỷ lệ thành công, token offloaded |
| `GET` | `/api/agents/concurrency` | `range=today\|24h\|7d\|30d\|all` | Chuỗi thời gian tải đồng thời và trần dung lượng an toàn |
| `GET` | `/api/agents/gantt` | `groupBy=roles\|sessions&range=today\|24h\|7d\|30d\|all` | Danh sách tiến trình Gantt thực tế gom theo vai trò (`roles`) hoặc phiên (`sessions`) |
| `GET` | `/api/agents/gantt/packets` | `range=today\|24h\|7d\|30d\|all` | Danh sách các gói tin tiến trình tác vụ Subagent định dạng timeline packets phục vụ vẽ Gantt |
| `GET` | `/api/agents/graph` | `project=...&range=today\|24h\|7d\|30d\|all` | Sơ đồ mạng lưới topo quan hệ tác nhân và dự án (Topology Network Graph) |
| `GET` | `/api/projects/leaderboard` | Bảng xếp hạng FinOps Đa LLM hợp nhất (Google, Codex, Claude) (`?range=today\|24h\|7d\|30d\|all&sort=tokens\|cost\|activity`) |
| `GET` | `/api/openai/dashboard` | `range=today\|24h\|7d\|30d\|all` | Dashboard OpenAI/Codex cục bộ: token, workflow, models, rate limits (5h & 7d) và sessions |
| `GET` | `/api/openai/graph` | `range=today\|24h\|7d\|30d\|all` | Sơ đồ mạng lưới Topology phân cấp 4 tầng cho OpenAI Codex |
| `POST` | `/api/openai/refresh` | Không | Quét lại `~/.codex/sessions/**/*.jsonl` theo yêu cầu (Read-only, không credential) |
| `GET` | `/api/claude/dashboard` | `range=today\|24h\|7d\|30d\|all` | Dashboard Anthropic Claude Code CLI cục bộ: token, tools, models, projects |
| `GET` | `/api/claude/graph` | `range=today\|24h\|7d\|30d\|all` | Sơ đồ mạng lưới Topology phân cấp 4 tầng cho Anthropic Claude |
| `POST` | `/api/claude/refresh` | Không | Quét lại `~/.claude/projects/**/*.jsonl` theo yêu cầu (Read-only, không credential) |
| `GET/POST` | `/api/sync/history` | Không | Kích hoạt quét vét toàn bộ lịch sử trò chuyện trên máy trạm vào CSDL |
| `POST` | `/api/test/simulate` | `model=...` | Nạp sự kiện giả lập để kiểm thử (Từ chối GET với mã HTTP 405) |
| `GET` | `/docs/` | Đường dẫn file tĩnh | Cung cấp tài liệu kỹ thuật và sơ đồ qua HTTP |
| `GET` | `/config.yaml` | Không | Trả về nội dung cấu hình thô phục vụ Cổng tra cứu tài liệu |
| `GET` | `/` | Không | Giao diện Web Dashboard chính từ `web/static/index.html` (3 LLM Tabs, 6 KPI Cards + FinOps Calculator) |

---

## 4. Tuân Thủ 6 Quy Chuẩn Tài Liệu Hóa Sản Xuất (Production Standards Compliance)

Hệ thống tài liệu của TokenMonitor tuân thủ 100% sáu quy chuẩn kỹ thuật theo `documentation-standards.md`:

1. **Quy Chuẩn 1 — Vị Trí Gốc Thư Mục `docs/` (Root Placement Contract)**:  
   Tất cả tài liệu chính (`.md`), sơ đồ tương tác (`.html`), file mục lục (`README.md`) và Cổng tra cứu (`index.html`) được đặt trực tiếp tại `docs/`, ngăn chặn lỗi lệch timestamp trên Windows File Explorer và Google Drive Sync.
2. **Quy Chuẩn 2 — Cổng Tra Cứu Tự Chứa 100% (Master Portal Contract)**:  
   `docs/index.html` tích hợp toàn bộ dữ liệu nội bộ (`DOCS_DATA`) và bộ biên dịch Markdown phía client, hoạt động hoàn hảo 0ms trên giao thức `file:///` mà không bị chặn bởi chính sách CORS của trình duyệt.
3. **Quy Chuẩn 3 — Bản Đồ Dây Dẫn Cụ Thể (Wiring & Communication Protocols)**:  
   Mọi sơ đồ luồng dữ liệu đều chỉ rõ file cấu hình (`config.yaml`), file mã nguồn đảm nhiệm (`storage/db.go`, `collector/tailer.go`, v.v.) và cơ chế giao tiếp được đánh số thứ tự từ 1 đến 6.
4. **Quy Chuẩn 4 — Đặc Tả CSDL Quan Hệ & ERD (Database ERD Standard)**:  
   Đặc tả đầy đủ 5 bảng, 7 chỉ mục chiến lược, ràng buộc `ON DELETE CASCADE`, kiến trúc đăng ký kép 6 PRAGMA SQLite WAL, và kịch bản DDL hoàn chỉnh sẵn sàng copy-paste.
5. **Quy Chuẩn 5 — Minh Bạch Bảo Mật & Thông Tin Nhạy Cảm (Security Compliance Contract)**:  
   Tuyệt đối không lưu mật khẩu Gmail dạng plaintext, sử dụng chế độ chỉ đọc `mode=ro` với driver riêng `sqlite_detector` để trích xuất phiên làm việc, đảm bảo 100% dữ liệu xử lý nội bộ ngoại tuyến.
6. **Quy Chuẩn 6 — Trải Nghiệm Tương Tác Sơ Đồ Archify (Viewer UX Contract)**:  
   Hỗ trợ chế độ Focus/Trace mode (nhấp chuột vào khối để làm sáng luồng liên quan và làm mờ các khối còn lại, nhấp vào nền đen để khôi phục); hướng dẫn làm mới bộ nhớ đệm trình duyệt bằng `Ctrl + F5`.

---

## 5. Cấu Trúc Thư Mục `docs/`

```
docs/
├── index.html                          # [BẮT BUỘC] Cổng tra cứu Master Portal tự chứa offline 100%
├── README.md                           # Trang mục lục văn bản trung tâm (Tài liệu này)
├── TokenMonitor_Team_Agent_Fleet_Architecture.md # Đặc tả toàn diện kiến trúc Team Agent Fleet
├── TokenMonitor_Database_ERD.md        # Đặc tả chi tiết CSDL, ERD & Data Dictionary
├── TokenMonitor_Token_Estimation_Spec.md # Đặc tả đo lường Token & Context Caching
├── TokenMonitor_Configuration_Guide.md # Hướng dẫn cấu hình toàn diện & bảo mật
│
├── workspace-overview.html             # Sơ đồ đa dự án & hoạt họa dòng chảy 60 FPS
├── tokenmonitor-architecture.html      # Sơ đồ kiến trúc 3 tầng tương tác Archify
├── tokenmonitor-dataflow.html          # Sơ đồ luồng dữ liệu 6 trạm tương tác Archify
├── tokenmonitor-database-erd.html      # Sơ đồ CSDL & Index tương tác Archify
│
└── TokenMonitor_20260908/              # Bộ tài liệu Runbook chuyên sâu 7 phần & References
    ├── TokenMonitor_00-Prerequisites_20260908.md
    ├── TokenMonitor_01-Architecture_20260908.md
    ├── TokenMonitor_02-HA_Deployment_20260908.md
    ├── TokenMonitor_03-Install_Deploy_20260908.md
    ├── TokenMonitor_04-Tuning_20260908.md
    ├── TokenMonitor_05-Troubleshooting_20260908.md
    ├── TokenMonitor_06-Security_Policy_Compliance_20260908.md
    └── references/
        ├── TokenMonitor_Ref_001_schema.sql                 # DDL schema SQL chuẩn SQLite
        ├── TokenMonitor_Ref_002_config_template.yaml       # Template config YAML chuẩn mẫu
        ├── TokenMonitor_Ref_003_gemini_usage_metadata_spec.json # Đặc tả usageMetadata Gemini
        └── TokenMonitor_Ref_004_database_erd.md            # Đặc tả chi tiết ERD & Data Dictionary
```

---

## 6. Hướng Dẫn Tra Cứu & Vận Hành

- **Truy cập Cổng tra cứu Offline (Master Portal)**:
  - Mở trực tiếp file [`docs/index.html`](./index.html) bằng bất kỳ trình duyệt nào (`file:///.../docs/index.html`). Nhờ dữ liệu được nạp sẵn (`DOCS_DATA`), trang chạy tức thì 0ms, không cần cài web server và không bị lỗi CORS.
  - Khi server đang chạy: Mở `http://localhost:9090/docs/`.
- **Xem sơ đồ kiến trúc tương tác**:
  - Nhấp đúp chuột vào các file HTML tương ứng (`tokenmonitor-architecture.html`, `tokenmonitor-dataflow.html`, `tokenmonitor-database-erd.html`).
  - *Mẹo tương tác*: Nhấp chuột vào từng khối để kích hoạt Focus/Trace mode; nhấp chuột ra vùng trống để phục hồi toàn bộ sơ đồ; bấm `Ctrl + F5` nếu cần xóa cache trình duyệt.
- **Tra cứu cấu hình**: Đọc file [`TokenMonitor_Configuration_Guide.md`](./TokenMonitor_Configuration_Guide.md).
- **Sao chép DDL schema CSDL**: Đọc file [`TokenMonitor_Database_ERD.md`](./TokenMonitor_Database_ERD.md) hoặc file tham chiếu [`references/TokenMonitor_Ref_001_schema.sql`](./TokenMonitor_20260908/references/TokenMonitor_Ref_001_schema.sql).
- **Khởi động ứng dụng**:
  ```powershell
  .\token_monitor.exe
  ```
  Sau đó truy cập Dashboard tại: `http://localhost:9090`.

