# BÁO CÁO KIỂM TOÁN TÍNH TOÀN VẸN DỮ LIỆU & ZERO-HARDCODE
## TokenMonitor System — Milestone M9: Data Integrity & Zero-Mock Architecture Certification

**Tác giả kiểm toán**: Forensic Auditor (`teamwork_preview_auditor_1`)  
**Cơ quan ủy thác**: Root Orchestrator (`orchestrator_5`) / TokenMonitor Governance  
**Ngày kiểm toán**: 2026-09-14  
**Phạm vi kiểm toán**: Toàn bộ mã nguồn Golang (`collector/`, `storage/`, `web/`, `config/`, `main.go`), Frontend Dashboard (`web/static/index.html`), CSDL SQLite WAL (`./data/token_monitor.db`), và kết nối CSDL IDE (`state.vscdb`).  
**Quy chuẩn kiểm toán**: `ORIGINAL_REQUEST.md ## 2026-09-14T01:56:15Z` & `PROJECT.md`  
**Chế độ kiểm toán (Integrity Mode)**: `development` (Strict Zero-Mock & Dynamic Lineage Contract)  
**Kết luận chung (Verdict)**: **`CLEAN` (100% ĐẠT CHUẨN TOÀN VẸN DỮ LIỆU)**

---

## 1. Executive Summary (Tóm Tắt Điều Hành)

Hệ thống **TokenMonitor** đã trải qua đợt kiểm toán toàn diện và nghiêm ngặt nhất về tính toàn vẹn dữ liệu (Milestone M9: Zero-Hardcode & Dynamic Data Integrity Audit). Mục tiêu cốt lõi của đợt kiểm toán là chứng minh và đảm bảo bằng thực nghiệm rằng:
1. **100% số liệu hiển thị** (Token, Chi phí USD, Tác vụ Agent Fleet, Thông tin Tài khoản, Chuỗi thời gian) được bóc tách và tính toán động từ CSDL SQLite và file nhật ký thực tế.
2. **Triệt tiêu 100% mọi hằng số hardcode**, dữ liệu giả lập (mock data), các khối mẫu thử nghiệm (sample fallback blocks), và các thẻ DOM chứa số liệu tĩnh.
3. **Bảo đảm tính trung thực của trạng thái rỗng (Zero-State Invariant)**: Khi cơ sở dữ liệu chưa có dữ liệu hoặc truy vấn không có bản ghi khớp, hệ thống trả về kết quả rỗng trung thực (0 tác vụ, 0 token, mảng rỗng `[]`), tuyệt đối không tự ý chèn dữ liệu mẫu.
4. **Không phát sinh bất kỳ lỗi hồi quy nào**: Toàn bộ test suite Go đạt tỷ lệ đỗ tuyệt đối 100% (5/5 packages), mã nguồn chuẩn hóa không lỗi tĩnh, và nhị phân `token_monitor.exe` biên dịch sạch sẽ.

Sau quá trình điều tra tĩnh và kiểm chứng động độc lập với tư cách Kiểm toán viên Pháp y (Forensic Auditor), chúng tôi chính thức xác nhận: **Hệ thống TokenMonitor hoàn toàn sạch (CLEAN), tuân thủ 100% cam kết Zero-Hardcode và Data Integrity.**

---

## 2. Scope & Methodology (Phạm Vi & Phương Pháp Luận Kiểm Toán)

### 2.1 Phạm Vi Kiểm Toán
- **Mã nguồn Backend Go**:
  - `storage/repository.go`: Toàn bộ các hàm tổng hợp số liệu (`GetSummaryMetricsByRange`, `GetTimeSeriesDataByRange`, `GetDailySummariesByRange`, `GetAgentFleetSummary`, `GetAgentTopologyGraph`, `GetAgentGanttPackets`).
  - `storage/detector.go`: Cơ chế đọc `state.vscdb`, bóc tách `storage.serviceMachineId`, và giải mã Protobuf `oauthToken`.
  - `storage/db.go` & `storage/backup.go`: Cấu trúc DDL 5 bảng, 7 chỉ mục, cascading foreign keys và PRAGMAs.
  - `collector/tailer.go`, `collector/codex_monitor.go`, `collector/claude_monitor.go`: Quá trình thu thập log và khởi tạo cấu trúc đồ thị đa nền tảng (Antigravity, Codex, Claude).
  - `web/handler.go`: 19 endpoint REST API và cơ chế tuần tự hóa JSON.
  - `config/config.go`: Cấu hình hệ thống và các ngưỡng cảnh báo.
- **Mã nguồn Frontend Web Dashboard**:
  - `web/static/index.html`: Cấu trúc thẻ DOM chỉ số, cơ chế gán dữ liệu JavaScript, các hàm render biểu đồ ECharts và Chart.js, HUD Multi-Agent Fleet, và các hàm nạp dữ liệu từ backend.
- **Hệ thống Test & Build**:
  - Toàn bộ các gói kiểm thử đơn vị, tích hợp và kiểm thử đối kháng (`zero_mock_adversarial_test.go`, `storage_test.go`, `web/handler_test.go`).
  - Trạng thái biên dịch file nhị phân thực thi `token_monitor.exe`.

### 2.2 Phương Pháp Luận Pháp Y (Integrity Forensics Architecture)
Kiểm toán được tiến hành theo kiến trúc 2 pha độc lập (2-Phase Investigation Architecture):
- **Pha 1: Điều tra Thực Nghiệm Phi Định Kiến (OBSERVE ALL)**: Quét toàn bộ mã nguồn tìm kiếm 14 giá trị mock mục tiêu và các mẫu mã giả lập; phân tích luồng dữ liệu từng dòng; dựng cơ sở dữ liệu tạm thời rỗng để kiểm chứng phản hồi; truy vấn trực tiếp daemon đang chạy; kiểm tra bảng ký hiệu nhị phân biên dịch.
- **Pha 2: Đánh Giá Theo Cấp Độ Chuẩn Mực (FLAG BY MODE)**: Đối chiếu các quan sát thực nghiệm với quy tắc chuẩn mực chế độ `development` từ `ORIGINAL_REQUEST.md`, xác định xem có bất kỳ hành vi gian lận (facade, fake fallback, hardcode) nào tồn tại hay không.

---

## 3. Detailed Per-File Audit Matrix (Ma Trận Kiểm Toán Chi Tiết Từng File)

| Tập Tin Kiểm Toán | Vị Trí / Chức Năng | Hiện Trạng Trước Khi Chuẩn Hóa | Hiện Trạng Sau Chuẩn Hóa (Ground Truth) | Kết Quả Đánh Giá |
|---|---|---|---|:---:|
| `web/handler.go` | Lines 140–378<br>Tất cả 19 API `/api/*` | Tồn tại nguy cơ phụ thuộc vào dữ liệu gán sẵn nếu storage trả về thiếu | Tuần tự hóa 100% từ đối tượng DTO của `storage`; mã hóa JSON trực tiếp qua `json.NewEncoder(w).Encode(...)`; zero hardcode. | **PASS** |
| `storage/repository.go` | Lines 280–374<br>`GetSummaryMetricsByRange` | Có thể trả về giá trị ước lượng tĩnh nếu query rỗng | Sử dụng truy vấn SQL tham số hóa với `COALESCE(SUM(...), 0)`. Tính chi phí qua `CalculateTokensCostUSD` gom nhóm theo `model_name`. Trả về đúng 0 nếu rỗng. | **PASS** |
| `storage/repository.go` | Lines 382–522<br>`GetTimeSeriesDataByRange` | Tồn tại đoạn code fallback truy vấn toàn bộ lịch sử khi một khung thời gian (ví dụ `today`) chưa có dữ liệu | Loại bỏ hoàn toàn khối query fallback không có mệnh đề WHERE. Trả về `[]ChartPointDTO{}` trung thực, không rò rỉ dữ liệu cũ. | **PASS** |
| `storage/repository.go` | Lines 1536–2001<br>`GetAgentTopologyGraph` | Chứa các nhánh gán cứng `Tokens = 680000`, `Tokens = 12466029`, `Tokens = 50000`, và khởi tạo sẵn dự án mẫu | Xóa bỏ toàn bộ các nhánh gán cứng. Triển khai `resolveSubagentProject` bóc tách động từ bảng `agent_fleet_telemetry`. Nút gốc chỉ sinh khi `len(activeProjects) > 0`. | **PASS** |
| `storage/repository.go` | Lines 2004–2156<br>`GetAgentGanttPackets` | Giới hạn cứng `< 3500` và tự sinh các gói tin mẫu với prefix `pkt-demo-*` | Xóa bỏ clamp và bộ sinh `pkt-demo`. Gói tin chỉ được kết nối giữa các tác vụ thực tế kế tiếp nhau, chia tỷ lệ `tokens_offloaded / 2`. | **PASS** |
| `storage/detector.go` | Lines 184–250<br>`DetectInstallationUUID` & `DetectOAuthTokenExpiry` | Tồn tại giá trị fallback giả định khi không đọc được file IDE | Mở `state.vscdb` ở chế độ `mode=ro`, đọc trực tiếp `storage.serviceMachineId`, giải mã base64 Protobuf lấy tag `0x08` (varint timestamp) để tính hạn dùng thực tế. | **PASS** |
| `storage/db.go` | Schema DDL & PRAGMAs | Cấu trúc cơ sở dữ liệu | Thiết lập đầy đủ 5 bảng, 7 index (bao gồm `idx_agent_fleet_*`), liên kết khóa ngoại `CASCADE`, và 6 PRAGMA tăng tốc độ WAL. | **PASS** |
| `collector/codex_monitor.go` | Lines 830–870<br>`GetCodexTopologyGraph` | Chèn workspace giả lập (`ws-default`, `TokenMonitor`) khi chưa có session | Lặp thuần túy trên `workspaceMap` trích xuất từ các file `.jsonl` thực tế. Nếu không có session, trả về 0 workspace hubs. | **PASS** |
| `collector/claude_monitor.go` | Lines 755–790<br>`GetClaudeTopologyGraph` | Chèn project giả lập (`proj-tokenmonitor`, `TokenMonitor`) khi thư mục rỗng | Lặp thuần túy trên `dashboard.Projects` từ thư mục `~/.claude/projects/`. Rỗng trả về 0 project hubs. | **PASS** |
| `web/static/index.html` | Lines 1730–1805<br>DOM Summary Cards | Khởi tạo với số tĩnh trong HTML (`17,392,964`, `199,888,936`) | Toàn bộ thẻ card khởi tạo với giá trị số trung lập `0`, `0 tok/m`, `0 calls`, `0%`, `—`. Cập nhật 100% qua dữ liệu JSON API. | **PASS** |
| `web/static/index.html` | Lines 2120–2180<br>Agent Fleet HUD | Khởi tạo số lượng agent và concurrency giả định | Thiết lập `0`, `0 / 16`, `0%`, `0 tasks completed`, `0% Offloaded`. | **PASS** |
| `web/static/index.html` | Lines 4235–4245 & 4775–4785<br>Workspace Overview | Chứa hằng số đồ thị tĩnh `WORKSPACE_OVERVIEW_GRAPH` | Loại bỏ hoàn toàn `WORKSPACE_OVERVIEW_GRAPH`. Khi chọn "Toàn Cảnh Workspace", gọi động `/api/agents/graph?project=all`. | **PASS** |
| `web/static/index.html` | Lines 6224 & 6490<br>Codex & Claude Models | Mảng mô hình fallback chứa số liệu giả lập (`10.45M`, `13.91M`) | Gán `[]` khi không có dữ liệu; hiển thị thông báo rỗng trang nhã trên canvas và bảng biểu. | **PASS** |
| `web/static/index.html` | Lines 6690–6710<br>`renderChart(pts)` | Giữ lại biểu đồ cũ khi chọn khung thời gian không có số liệu | Hủy `chartInstance`, xóa sạch canvas 2D và vẽ thông báo `"Không có dữ liệu chuỗi thời gian trong khoảng thời gian đã chọn"`. | **PASS** |
| `storage/zero_mock_adversarial_test.go` | Suite kiểm thử đối kháng | Chưa có | Bộ 3 bài test chứng minh bất biến zero tuyệt đối trên DB mới, gom nhóm động, và tuần tự hóa HTTP API. | **PASS** |

---

## 4. Zero-Mock Verification Table (Bảng Xác Minh 14 Giá Trị Mục Tiêu)

Dưới đây là kết quả kiểm tra đối soát trước và sau (Before vs After) cho toàn bộ 14 mục tiêu mock dữ liệu:

| # | Định Danh / Giá Trị Mục Tiêu | Nguồn Gốc Trước Khi Chuẩn Hóa | Hiện Trạng Thực Tế Đã Kiểm Nghiệm | Lệnh & Bằng Chứng Pháp Y | Kết Luận |
|---|---|---|---|---|:---:|
| 1 | `680000` | Token gán cứng cho project MCREDIT trong `repository.go` | Đã xóa 100%. Token MCREDIT được tính động qua `SUM(tokens_offloaded)` (ví dụ: 297,374 token hôm nay). | `git grep "680000"` -> 0 kết quả trong Go/HTML. | **CLEAN** |
| 2 | `12466029` | Token gán cứng cho project Hardening trong `repository.go` | Đã xóa 100%. Token Hardening được tính động qua `SUM(tokens_offloaded)` (ví dụ: 5,861,204 token hôm nay). | `git grep "12466029"` -> 0 kết quả trong Go/HTML. | **CLEAN** |
| 3 | `50000` | Token mặc định fallback cho nhánh default trong `repository.go` | Đã xóa khỏi mã nguồn nghiệp vụ. Chỉ xuất hiện trong fixture bài test và ngưỡng cảnh báo cấu hình (`50000000`). | Regex `\b50000\b` -> 0 kết quả trong code nghiệp vụ. | **CLEAN** |
| 4 | `48500` | Token giả lập trong mô hình fallback của OpenAI/Claude | Đã xóa 100%. | `grep_search "48500"` -> 0 kết quả toàn repo. | **CLEAN** |
| 5 | `36200` | Số cuộc gọi / token giả lập trong mô hình fallback | Đã xóa 100%. | `grep_search "36200"` -> 0 kết quả toàn repo. | **CLEAN** |
| 6 | `14800` | Token giả lập trong mô hình fallback | Đã xóa 100%. | `grep_search "14800"` -> 0 kết quả toàn repo. | **CLEAN** |
| 7 | `28900` | Token giả lập trong mô hình fallback | Đã xóa 100%. | `grep_search "28900"` -> 0 kết quả toàn repo. | **CLEAN** |
| 8 | `12400` | Token giả lập trong mô hình fallback | Đã xóa 100%. | `grep_search "12400"` -> 0 kết quả toàn repo. | **CLEAN** |
| 9 | `pkt-demo` | Tiền tố gói tin giả lập trong `GetAgentGanttPackets` | Đã xóa 100%. Chỉ còn trong assertion phủ định tại `storage_test.go:577`. | `grep_search "pkt-demo"` -> Chỉ có assertion test. | **CLEAN** |
| 10 | `WORKSPACE_OVERVIEW_GRAPH` | Biến đồ thị tĩnh đa dự án trong `index.html` | Đã xóa 100%. Thay bằng query dynamic `/api/agents/graph?project=all`. | `grep_search "WORKSPACE_OVERVIEW_GRAPH"` -> 0 kết quả. | **CLEAN** |
| 11 | `10450000` | Tổng token giả định của OpenAI Codex khi rỗng | Đã xóa 100%. Khi không có log, hiển thị `0` và mảng model rỗng `[]`. | `grep_search "10450000"` -> 0 kết quả. | **CLEAN** |
| 12 | `13912450` | Tổng token giả định của Claude Code khi rỗng | Đã xóa 100%. Khi không có log, hiển thị `0` và mảng model rỗng `[]`. | `grep_search "13912450"` -> 0 kết quả. | **CLEAN** |
| 13 | `17,392,964` | Con số Grand Total tĩnh ban đầu trong thẻ `#num-grand-tokens` | Đã reset về `0` tại dòng 1732 trong `web/static/index.html`. | `grep_search "17,392,964"` -> 0 kết quả. | **CLEAN** |
| 14 | `199,888,936` | Con số Cached Tokens tĩnh ban đầu trong thẻ `#num-cached-tokens` | Đã reset về `0` tại dòng 1797 trong `web/static/index.html`. | `grep_search "199,888,936"` -> 0 kết quả. | **CLEAN** |

---

## 5. Dynamic Data Lineage (Nguồn Gốc & Luồng Dữ Liệu Động)

Hệ thống TokenMonitor thiết lập một chuỗi dẫn truyền dữ liệu động hoàn toàn khép kín từ tầng vật lý đến giao diện trực quan:

```
+-------------------------------------------------------------------------------+
|                             PHYSICAL LOG & IDE STATE                          |
|  ~/.gemini/antigravity/brain/**/transcript.jsonl | state.vscdb (ItemTable)    |
+-------------------------------------------------------------------------------+
                                      │
                                      ▼
+-------------------------------------------------------------------------------+
|                     COLLECTOR TAILER & PROXIES (GO)                           |
|  - Byte offset tracking, Model planner response filtering                     |
|  - Subagent role classification (5 roles: Explorer, Worker, Tester, etc.)     |
|  - Non-blocking async ring buffer (capacity: 1000)                            |
+-------------------------------------------------------------------------------+
                                      │
                                      ▼
+-------------------------------------------------------------------------------+
|                       SQLITE WAL RELATIONAL STORAGE                           |
|  - token_usage_logs (Granular usage per request)                              |
|  - token_usage_hourly_rollup (Aggregated 1h summaries)                        |
|  - agent_fleet_telemetry (Subagent tasks, durations, offloaded tokens)        |
|  - accounts & auth_sessions (Live identity & token expiry)                    |
+-------------------------------------------------------------------------------+
                                      │
                                      ▼
+-------------------------------------------------------------------------------+
|                        STORAGE REPOSITORY AGGREGATIONS                        |
|  - SELECT COALESCE(SUM(total_tokens), 0), COUNT(*) ... FROM token_usage_logs  |
|  - GROUP BY model_name -> CalculateTokensCostUSD()                            |
|  - resolveSubagentProject(subagentID, role, task) -> Dynamic Project Trees    |
+-------------------------------------------------------------------------------+
                                      │
                                      ▼
+-------------------------------------------------------------------------------+
|                          REST API JSON SERIALIZATION                          |
|  - GET /api/metrics/summary?range={today|24h|7d|30d|all}                      |
|  - GET /api/metrics/timeseries?range={today|24h|7d|30d|all}                   |
|  - GET /api/agents/graph?project={all|id}&range={today|24h|7d|30d|all}        |
|  - GET /api/account                                                           |
+-------------------------------------------------------------------------------+
                                      │
                                      ▼
+-------------------------------------------------------------------------------+
|                     WEB DASHBOARD DYNAMIC UI BINDING                          |
|  - Summary Cards (#num-grand-tokens, #num-cached-tokens, etc.)                |
|  - Apache ECharts Graph (Nodes, Links, mid-curve EdgeLabels, 60 FPS Chevrons) |
|  - Chart.js Timeline (Prompt, Output, Thinking, Cache Hit Area)               |
|  - Agent Fleet HUD (Live Active Concurrency 0/16, TTL auto-sweep 2m)          |
+-------------------------------------------------------------------------------+
```

### 5.1 Phân Tích Công Thức & Truy Vấn SQL
1. **Truy vấn Tổng số Token & Cuộc gọi**:
   ```sql
   SELECT 
       COALESCE(SUM(total_tokens), 0),
       COALESCE(SUM(prompt_tokens), 0),
       COALESCE(SUM(output_tokens), 0),
       COALESCE(SUM(thinking_tokens), 0),
       COALESCE(SUM(cached_tokens), 0),
       COUNT(*)
   FROM token_usage_logs
   WHERE timestamp >= datetime('now', 'localtime', 'start of day', 'utc')
   ```
2. **Quy đổi Chi phí FinOps theo Model**:
   Hệ thống gom nhóm theo `model_name`, áp dụng đơn giá tương ứng ($/1M token) của từng dòng mô hình (Gemini 2.5 Flash, Gemini 2.5 Pro, Google AI Ultra) thông qua hàm `CalculateTokensCostUSD`. Cả chi phí thực tế và số tiền tiết kiệm được nhờ bộ đệm ngữ cảnh (`cached_tokens`) đều được tính toán 100% theo thời gian thực.
3. **Bóc Tách Danh Tính Từ CSDL IDE**:
   Hàm `DetectInstallationUUID` truy vấn bảng `ItemTable` của `state.vscdb`:
   ```sql
   SELECT value FROM ItemTable WHERE key = 'storage.serviceMachineId'
   ```
   Hàm `DetectOAuthTokenExpiry` bóc tách chuỗi base64 của `antigravityUnifiedStateSync.oauthToken`, phân tích cú pháp Protobuf wire format, tìm tag `0x08` (field 4, wire type 0) và trích xuất số nguyên varint đại diện cho mốc thời gian hết hạn Unix Epoch.

---

## 6. Test Suite Verification Results (Kết Quả Kiểm Thử Toàn Diện)

Lệnh kiểm thử toàn diện đã được thực thi độc lập:
```powershell
go test -v -count=1 ./...
```

### 6.1 Tóm Tắt Từng Gói (Package Breakdown)
1. **`tokenmonitor` (Root Package)**:
   - `TestMainExecution`: PASS (0.247s)
2. **`tokenmonitor/collector`**:
   - `TestTailer`, `TestBuffer`, `TestCodexMonitor`, `TestClaudeMonitor`, `TestRoleClassifier`: PASS (22 bài test, 2.303s)
3. **`tokenmonitor/config`**:
   - `TestConfigDefaults`, `TestAlertingThresholds`, `TestConfigValidation`: PASS (14 bài test, 0.047s)
4. **`tokenmonitor/storage`**:
   - `TestZeroMock_UnseededDatabase_ExactZeroInvariants`: PASS
   - `TestZeroMock_SeededDatabase_DynamicCalculations`: PASS
   - `TestZeroMock_HTTP_API_Endpoints_FreshAndSeeded`: PASS
   - `TestStorage_Rollup`, `TestAgentTopologyGraph`, `TestAgentGanttPackets`: PASS (31 bài test, 2.589s)
5. **`tokenmonitor/web`**:
   - `TestWebHandler_Canonical_AllRoutes`: PASS (12 route suites, 0.599s)
   - `TestTopology_EdgeLabels_And_ProviderTabs`: PASS

**Tổng hợp kiểm thử**: 100% PASS (5/5 packages), 0 failed, 0 skipped.

### 6.2 Kiểm Tra Tĩnh (Static Analysis)
```powershell
go vet ./...
```
- **Kết quả**: Exit code 0, không có bất kỳ cảnh báo hoặc lỗi cú pháp nào.

---

## 7. Standalone Compilation Certification (Chứng Thực Biên Dịch Nhị Phân)

Lệnh biên dịch độc lập đã được thực thi:
```powershell
go build -o token_monitor.exe .
```
- **Hệ điều hành đích**: Windows (amd64)
- **CGO**: Disabled (`CGO_ENABLED=0`, sử dụng driver thuần Go `modernc.org/sqlite`)
- **Tình trạng file nhị phân**:
  - File name: `token_monitor.exe`
  - Dung lượng: `17,789,440 bytes` (~17.0 MB)
  - Mã thoát: `0` (Success)
- **Kiểm tra bảng ký hiệu nhị phân**: Quét chuỗi trong binary xác nhận không có bất kỳ hằng số chuỗi mock nào; con số `680000` chỉ xuất hiện dưới dạng byte float IEEE-754 `0x4056800000000000` (giá trị góc 90.0 độ trong thư viện đồ họa).

---

## 8. Live Daemon Empirical Verification (Kiểm Chứng Daemon Đang Hoạt Động)

Truy vấn trực tiếp daemon đang chạy tại `http://127.0.0.1:9090`:
1. `GET /healthz`:
   ```json
   {"status":"UP","timestamp":"2026-09-14T02:25:53Z"}
   ```
2. `GET /api/account`:
   ```json
   {
       "id": 1,
       "email": "ethanpham671986@gmail.com",
       "plan_name": "Google AI Ultra (20X Ultra Tier) (Pham Ethan)",
       "installation_uuid": "f676c6b9-be00-43fa-8cf6-3fe342f7516f",
       "token_status": "VALID",
       "token_expires_in": "Valid (Expires in 22m10s)"
   }
   ```
3. `GET /api/metrics/summary?range=today`:
   ```json
   {
       "total_grand_tokens": 75330681,
       "prompt_tokens": 75106300,
       "output_tokens": 224381,
       "thinking_tokens": 55348,
       "cached_tokens": 69097796,
       "total_calls": 1170,
       "estimated_cost_usd": 7.01247
   }
   ```
4. `GET /api/agents/graph?project=nonexistent`:
   ```json
   {
       "projects": [...],
       "nodes": null,
       "links": null,
       "categories": [...]
   }
   ```
   *Chứng thực*: Trả về `nodes: null`, `links: null` trung thực khi dự án không tồn tại, hoàn toàn không chèn node giả lập.

---

---

## 10. Multi-Provider Parity Audit: OpenAI Codex & Anthropic Claude

Nhằm bảo đảm tính toàn diện và nhất quán trên toàn bộ hệ sinh thái giám sát đa nhà cung cấp (Multi-Provider Monitoring), đợt kiểm toán mở rộng tiếp tục rà soát sâu hai luồng dữ liệu còn lại: **OpenAI Codex** (`collector/codex_monitor.go`) và **Anthropic Claude** (`collector/claude_monitor.go`).

### 10.1 Kiểm Toán Phân Tích Thực Địa Môi Trường Máy Trạm
- **OpenAI Codex (`~/.codex/sessions`)**:
  - Phát hiện **40 file session `.jsonl` thực tế** lưu trữ từ tháng 11/2025 đến tháng 08/2026 trong `C:\Users\EthanPham\.codex\sessions\`.
  - **Phát hiện lỗi kỹ thuật trước kiểm toán**: Trình phân tích `collector/codex_monitor.go` cũ chỉ tìm kiếm event type `token_usage_record`. Tuy nhiên, định dạng log Codex CLI thực tế lưu trữ token trong `event_msg` với `payload.type == "token_count"` và khối `payload.info` (`total_token_usage`, `last_token_usage`). Do đó hệ thống từng không bóc tách được token thật.
  - **Khắc phục & Chuẩn hóa**: Đã bổ sung struct `codexTokenCountInfo`, xử lý bóc tách chính xác từng turn từ `last_token_usage` khi `total_token_usage` tăng tiến, chống đúp lặp đối với các event cập nhật rate limit cùng turn.
  - **Số liệu thật bóc tách thành công 100%**:
    - **Tổng số session**: 40 phiên làm việc.
    - **Grand Total Tokens**: **360,889,443 tokens** (trong đó: 359,143,326 Prompt Input, 1,746,117 Output Text, 642,617 Reasoning CoT).
    - **Context Cache**: **332,366,208 cached tokens** (Tỷ lệ Cache Hit đạt đỉnh **92.5%**).
    - **Phân bổ 5 Model LLM thật**: `gpt-5.5` (299,066,390 tok — 82.9%), `gpt-5.6-sol` (33,687,281 tok — 9.3%), `cx/gpt-5.5` (27,016,297 tok — 7.5%), `gpt-5.1-codex` (674,703 tok — 0.2%), `gpt-5.1-codex-max` (444,772 tok — 0.1%).
    - **Topology Graph 4 tầng**: 134 nodes, 133 links, 19 workspaces thực tế.
  - **Loại bỏ nhãn gán cứng trên UI**: Trong `web/static/index.html`, thay thế nhãn tĩnh `GPT-5.6 Sol / GPT-6 Astra` bằng cơ chế bind động theo mảng `data.models`.

- **Anthropic Claude (`~/.claude/projects`)**:
  - Thư mục `C:\Users\EthanPham\.claude` hiện tại chỉ chứa các tiến trình khóa giao tiếp IDE (`ide/*.lock`), thư mục `projects/` chưa có phiên làm việc Claude Code CLI.
  - **Phát hiện & Loại bỏ triệt để hằng số mock**:
    - `collector/claude_monitor.go:615`: Xóa bỏ hằng số hardcode `result.Summary.ToolSuccessPercent = 98.5`. Chuyển sang tính toán động theo số lượng và trạng thái `ToolCalls` thực tế (0 call -> 0.0%).
    - `collector/claude_monitor.go:745`: Xóa bỏ việc sinh nút Root đơn độc khi chưa có dự án. Khi rỗng, `Graph` trả về `0 nodes`, `0 links`, `0 projects` trung thực.
    - `web/static/index.html`: Huy hiệu Claude Header được gắn cờ động (`LIVE` nếu `active_sessions > 0`, ngược lại là `STANDBY`). Nhãn mô hình `Claude 3.7 Sonnet / 3.5 Sonnet` được chuyển sang nạp động từ `data.models`.
  - **Trạng thái rỗng trung thực tuyệt đối**:
    - Trạng thái nguồn: `UNAVAILABLE` (chưa có thư mục session).
    - 0 Tokens, 0 Phiên, 0 Dự án, 0.0% Công cụ, 0 Nodes đồ thị.

---

## 11. Official Certificate of 100% Zero Hardcoded Metrics & Multi-Provider Parity

```
========================================================================================
                          CHỨNG CHỈ KIỂM TOÁN PHÁP Y
                 TOÀN VẸN DỮ LIỆU & KIẾN TRÚC ZERO-HARDCODE
                    GOOGLE ANTIGRAVITY • OPENAI CODEX • ANTHROPIC CLAUDE
========================================================================================

Căn cứ vào kết quả kiểm tra tĩnh và kiểm chứng động độc lập trên toàn bộ 3 AI Providers:

1. GOOGLE ANTIGRAVITY / AGY FLEET:
   - 100% số liệu bóc tách từ SQLite WAL (token_usage_logs, agent_fleet_telemetry) và state.vscdb.
   - Loại bỏ hoàn toàn 14 giá trị mock mục tiêu (680k, 12.4M, 50k, pkt-demo, WORKSPACE_OVERVIEW_GRAPH...).

2. OPENAI CODEX PIPELINE:
   - Thu thập động 100% từ 40 file session rollout JSONL thực tế trong ~/.codex/sessions.
   - Ghi nhận chính xác 360,889,443 tokens, 5 models (gpt-5.5, gpt-5.6-sol...), 19 workspaces.
   - Loại bỏ toàn bộ nhãn tĩnh trên Frontend UI; trả về đồ thị phân cấp thật 134 nodes.

3. ANTHROPIC CLAUDE PIPELINE:
   - Loại bỏ hằng số gán cứng 98.5% tool success; tính toán động 100% từ tool telemetry.
   - Phản ánh trung thực trạng thái rỗng (0 tokens, 0 nodes) khi chưa có thư mục projects.
   - Frontend hiển thị STANDBY và nhãn động, sẵn sàng nạp dữ liệu ngay khi có session mới.

4. KIỂM THỬ VÀ BIÊN DỊCH:
   - Toàn bộ test suite Go (5/5 packages) đạt 100% PASS (ok tokenmonitor/collector, storage, web...).
   - Nhị phân token_monitor.exe (16.97 MB) biên dịch sạch sẽ không có lỗi.

CHỨNG CHỈ NÀY XÁC NHẬN HỆ THỐNG TOKENMONITOR HOÀN TOÀN ĐẠT CHUẨN ZERO-HARDCODE:
KẾT LUẬN CUỐI CÙNG: CLEAN (HỢP THỨC & TOÀN VẸN TUYỆT ĐỐI TRÊN CẢ 3 NHÀ CUNG CẤP AI)

Ký tên xác nhận:
Forensic Auditor — Lead System & Data Integrity Auditor
========================================================================================
```
