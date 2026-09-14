# Phase 05: Troubleshooting & Edge Cases
## System: Token & Usage Monitor (`TokenMonitor`)
**Date:** 2026-09-08 | **Author:** Senior Infrastructure & System Operations Expert  
**Standard Reference:** Skill `infra_research_runbook` & `golang-expert-guidelines`

---

## 1. Ma trận Xử lý Sự cố & Ngoại lệ (Troubleshooting Matrix)

| Mã lỗi / Hiện tượng | Nguyên nhân gốc rễ (Root Cause) | Mức độ | Biện pháp Khắc phục Tức thì |
| :--- | :--- | :--- | :--- |
| **HTTP 401 Unauthorized** từ Upstream Gemini | Access Token đã hết hạn quá thời gian 3600 giây hoặc Refresh Token bị thu hồi trên Google Cloud Console. | **HIGH** | 1. Kiểm tra trường `Token Status` trên Dashboard.<br>2. Kích hoạt lệnh refresh token mới qua Google OAuth.<br>3. Kiểm tra biến môi trường `GOOGLE_APPLICATION_CREDENTIALS`. |
| **HTTP 429 Too Many Requests** | Vượt ngưỡng hạn ngạch tần suất (RPM) hoặc lưu lượng token (TPM) của gói tài khoản. | **HIGH** | 1. Đối chiếu biểu đồ cột `Requests/min` trên Dashboard để xác định loại hạn ngạch bị chạm (RPM hay TPM).<br>2. Bật cờ giới hạn `max_concurrent_requests` trong file `config.yaml`. |
| **Cached Tokens tụt giảm đột ngột (< 50%)** | Tỷ lệ Cache Hit thấp do phần tiền tố prompt (Prompt Prefix) bị thay đổi vị trí, hoặc thêm bớt các file context ở phần đầu prompt. | **MEDIUM** | 1. Kiểm tra lại cấu trúc System Prompt và thứ tự nạp Workspace Files.<br>2. Đảm bảo giữ nguyên các khối văn bản tĩnh ở đầu context window để tận dụng Gemini Implicit Caching. |
| **Thiếu chỉ số `Thinking Tokens`** | Gọi vào mô hình không hỗ trợ Chain-of-Thought (như Gemini 1.5 Flash bản cũ) hoặc tính năng Thinking bị tắt trong config. | **LOW** | 1. Kiểm tra trường `model_name` trong log.<br>2. Chỉ các dòng model như `Gemini 2.0 Flash Thinking` hoặc `Gemini 3.8 Flash (High)` mới trả về `candidatesTokensDetails[modality='THINKING']`. |
| **Lỗi `database is locked` trên SQLite** | Có tiến trình bên ngoài (như DB Browser for SQLite) đang mở khóa ghi file `.db` mà không commit. | **MEDIUM** | 1. Đảm bảo `PRAGMA busy_timeout = 5000;` đã được nạp.<br>2. Kiểm tra và đóng các ứng dụng xem database của bên thứ ba.<br>3. Chuyển sang chế độ WAL nếu chưa bật. |
| **`ACTIVE CONCURRENCY` nhảy lên quá cao (vd: 46 / 48)** | Tích tụ tác vụ cũ (Stale Tasks Accumulation) do nạp hàng loạt thư mục lịch sử cũ mà các task chưa được đánh dấu `COMPLETED`. | **MEDIUM** | 1. Kích hoạt cơ chế **TTL Auto-Sweep 2 phút** trong `storage/repository.go`.<br>2. Chạy lệnh SQL cập nhật thủ công nếu cần: `UPDATE agent_fleet_telemetry SET status = 'COMPLETED' WHERE status = 'RUNNING' AND started_at < datetime('now', '-2 minutes');`.<br>3. Kiểm tra Dashboard: chỉ số sẽ trở về `0 / 5` hoặc `1 - 3 / 5` (`Safe Load • 0 Throttling`). |
| **Lỗi tranh chấp khóa file `state.vscdb`** | Mở CSDL nội bộ của Antigravity IDE bằng driver SQLite chính kế thừa hook ghi WAL. | **LOW** | 1. Đảm bảo module `storage/detector.go` sử dụng driver riêng `sqlite_detector`.<br>2. Mở file theo đường dẫn `file:%s?mode=ro` (chế độ chỉ đọc tuyệt đối). |
| **Streaming Response bị đứt quãng** | Mạng người dùng chập chờn khiến SSE (Server-Sent Events) bị disconnect trước khi nhận được chunk metadata cuối cùng. | **LOW** | 1. Khởi chạy fallback: ước tính số token dựa trên kích thước payload văn bản đã nhận được ($\approx 4\text{ chars} = 1\text{ token}$).<br>2. Gắn cờ `request_type = 'INCOMPLETE_STREAM'` để phân loại. |
| **Chạy CLI trên Server từ xa không hiện số liệu** | TokenMonitor mặc định chạy `local_tailer` đọc log trên ổ cứng máy Windows local (`~/.gemini/antigravity-ide/brain`). Phiên làm việc CLI trên máy chủ server từ xa có filesystem riêng, không tự động đồng bộ qua mạng. | **INFO** | 1. **Cách 1 (Khuyên dùng)**: Cài đặt và chạy TokenMonitor binary Linux (`token_monitor_linux`) trực tiếp trên máy chủ server.<br>2. **Cách 2 (Reverse Proxy)**: Bật `proxy.enabled: true` trên máy local (port 8080) và cấu hình CLI server export `HTTPS_PROXY` trỏ về IP máy local. |

---

## 2. Kịch bản Xử lý Chi tiết (In-Depth Incident Playbooks)

### 2.1. Playbook: Khắc phục sự cố Token Hết hạn (OAuth Token Expired)
1. **Triệu chứng:** Biểu tượng trạng thái trên Dashboard chuyển sang màu đỏ: `Expired (0m0s)`, các lượt gọi tiếp theo trả về HTTP 401.
2. **Kiểm tra nhật ký:**
   ```bash
   sqlite3 ./data/token_monitor.db "SELECT * FROM auth_sessions ORDER BY id DESC LIMIT 1;"
   ```
3. **Thao tác khắc phục:**
   * Nếu đang chạy trong Antigravity IDE: Mở bảng cài đặt tài khoản của IDE để re-authenticate tài khoản Google.
   * File token mới sẽ được tự động cập nhật và TokenMonitor sẽ đọc lại trạng thái `Valid (Expires in 59m59s)`.

### 2.2. Playbook: Xử lý Tắc nghẽn Bộ đệm ghi (Channel Congestion)
1. **Triệu chứng:** Log hiển thị cảnh báo `[WARN] Ingestion ring buffer full! Spooling to disk fallback.`
2. **Nguyên nhân:** Ổ cứng đang bị tải I/O quá nặng (ví dụ đang build code nặng hoặc copy file lớn), khiến SQLite không flush kịp.
3. **Thao tác khắc phục:**
   * TokenMonitor tự động chuyển hướng ghi tạm ra `./data/spool/fallback_*.jsonl`.
   * Khi tải đĩa giảm, tiến trình background worker sẽ tự động đọc các file spool này và nạp ngược lại vào SQLite mà không làm mất bất kỳ token nào.

### 2.3. Playbook: Khắc phục Dồn tích Tác vụ Subagent (Active Concurrency Stale Accumulation)
1. **Triệu chứng:** Thẻ `ACTIVE CONCURRENCY` hiển thị thanh màu cam/đỏ `46 / 48 (Peak Concurrency • Heavy Load)` dù máy tính đang rảnh rỗi không chạy lệnh nào.
2. **Nguyên nhân:** Trong cơ sở dữ liệu `agent_fleet_telemetry`, có nhiều tác vụ từ các phiên làm việc 15–30 phút trước vẫn đang mang `status = 'RUNNING'` do chưa có sự kiện hoàn tất rõ ràng.
3. **Kiểm tra trạng thái qua terminal:**
   ```bash
   sqlite3 ./data/token_monitor.db "SELECT count(*), status FROM agent_fleet_telemetry GROUP BY status;"
   ```
4. **Thao tác khắc phục:**
   * Hệ thống hiện tại đã tích hợp **Auto-Sweep 2 phút** tự động trong `GetAgentFleetSummary()`, chỉ số sẽ tự hạ về `0 / 5` sau khi refresh Dashboard.
   * Để cưỡng bức dọn dẹp tức thì qua CLI:
     ```bash
     sqlite3 ./data/token_monitor.db "UPDATE agent_fleet_telemetry SET status = 'COMPLETED' WHERE status = 'RUNNING' AND started_at < datetime('now', '-2 minutes');"
     ```

### 2.4. Playbook: Giám sát CLI chạy trên máy Server từ xa (Remote Server CLI Ingestion)
1. **Triệu chứng:** Người dùng dùng tài khoản của mình chạy CLI / SDK / script tương tác với Gemini trên máy server (Linux / Cloud VPS), nhưng Dashboard TokenMonitor mở trên máy trạm Windows local không ghi nhận bất kỳ lượt gọi hoặc token nào.
2. **Nguyên nhân kỹ thuật:**
   - Mặc định, TokenMonitor hoạt động ở chế độ `local_tailer: enabled: true`, tức là chỉ quét các file transcript JSONL trong thư mục cục bộ của máy Windows trạm: `%USERPROFILE%\.gemini\antigravity-ide\brain\**\transcript.jsonl`.
   - Khi chạy CLI trên máy server từ xa, log và network request hoàn toàn độc lập trên môi trường server đó, không tự động đồng bộ qua ổ cứng máy local.
3. **Các phương án khắc phục:**
   - **Phương án A (Khuyến nghị - Chạy daemon trên Server):**
     Biên dịch TokenMonitor sang Linux bằng lệnh:
     ```bash
     GOOS=linux GOARCH=amd64 go build -o token_monitor_linux .
     ```
     Copy binary `token_monitor_linux` và file `config.yaml` lên server, chạy background service qua `systemd` hoặc `nohup ./token_monitor_linux &`. Dashboard web trên server có thể truy cập qua cổng 9090 (hoặc qua SSH tunnel).
   - **Phương án B (Dùng Reverse Proxy trung gian):**
     1. Mở file `config.yaml` trên máy local, chuyển `proxy.enabled: true`, mở firewall cổng `8080`.
     2. Trên máy server, khi chạy CLI / script, export biến môi trường proxy trỏ về máy local:
        ```bash
        export HTTPS_PROXY=http://<IP_MAY_LOCAL>:8080
        # hoặc cấu hình Base URL trong SDK:
        export GEMINI_API_ENDPOINT=http://<IP_MAY_LOCAL>:8080
        ```
     Module `collector/proxy.go` trên máy local sẽ chặn bắt gói tin `usageMetadata` và lưu thẳng vào SQLite local.


---

## 3. Định dạng Log Chuẩn (Structured Logging)

TokenMonitor tuân thủ định dạng JSON Structured Log theo chuẩn ELK / OpenTelemetry:

```json
{
  "timestamp": "2026-09-08T13:40:15.123Z",
  "level": "INFO",
  "module": "PROXY_INTERCEPTOR",
  "account": "neurozextra@gmail.com",
  "model": "gemini-3.8-flash",
  "prompt_tokens": 15287172,
  "output_tokens": 2105792,
  "thinking_tokens": 695763,
  "cached_tokens": 199888936,
  "total_tokens": 17392964,
  "latency_ms": 1420,
  "status_code": 200
}
```
Log có thể được đẩy trực tiếp lên hệ thống ELK Stack tập trung theo đúng chuẩn kỹ thuật nội bộ của bạn.
