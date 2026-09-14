# TokenMonitor — AI Observability & FinOps Daemon

Hệ thống giám sát, phân tích chuyên sâu lưu lượng Token (Prompt, CoT Thinking, Output, Context Caching) và tần suất sử dụng mô hình AI (Gemini 3.8/3.7 Flash, Pro, Ultra, Claude Sonnet/Opus, GPT-OSS 120B) theo thời gian thực dành cho máy trạm cá nhân và đội ngũ kỹ thuật.

---

## 🚀 Khởi Chạy Nhanh

1. **Khởi chạy ứng dụng:**
   ```powershell
   ./token_monitor.exe
   ```
2. **Mở Bảng Điều Khiển (Web Dashboard):**
   * Truy cập trình duyệt tại: [http://127.0.0.1:9090](http://127.0.0.1:9090)

---

## 💾 Tự Động Sao Lưu & Khôi Phục Thảm Họa (Auto-Backup & Disaster Recovery)

Hệ thống tích hợp sẵn động cơ **Auto-Backup định kỳ chuẩn Enterprise**:
* **SQLite Native `VACUUM INTO`**: Tạo bản chụp snapshot nguyên tử (Atomic Snapshot) không khóa ghi (Non-blocking), tự động hợp nhất toàn bộ Write-Ahead Log (`.db-wal`) vào 1 file `.db` duy nhất.
* **Xoay Vòng Thông Minh (7-Day Daily Retention)**: Mỗi ngày duy trì đúng 1 file `token_monitor_backup_YYYYMMDD.db` (cập nhật mới nhất mỗi giờ) và chỉ giữ lại tối đa 7 ngày gần nhất. Tổng dung lượng toàn bộ backup cố định $\le 86\text{ MB}$, tuyệt đối không gây đầy ổ đĩa.
* **Khôi Phục 1-Click (RPO $\le$ 60p, RTO $<$ 10s)**: Luôn đồng bộ bản sao chuẩn hóa [data/backup/token_monitor.db](data/backup/token_monitor.db). Khi gặp sự cố máy trạm, chỉ cần copy 1 file này đè lại file chính để khôi phục toàn vẹn dữ liệu.
* **Không Xung Đột Google Drive**: Triệt tiêu hoàn toàn hiện tượng tạo file rác trùng lặp `token_monitor (1).*` khi thư mục dự án nằm trên Google Drive.

---

## 🛡️ Đảm Bảo An Toàn & Tuân Thủ Chính Sách AI (100% Policy Compliant)

Hệ thống hoạt động theo cơ chế **Nội bộ, Ngoại tuyến, Thụ động & Không Xâm Lấn (100% Passive & Offline)**:
* **Không gửi bất kỳ request mạng nào ra ngoài Internet** tới Google hay bất kỳ nhà cung cấp AI nào.
* **Đọc log thụ động (`LocalTailer`)**: Quét đa thư mục brain tại `~/.gemini/*/brain/**/transcript.jsonl` (loại trừ `backup`, `tmp`, `profile`, hợp nhất `IDEBrainDir`) bằng cơ chế con trỏ delta byte offset `f.Seek(lastOffset, io.SeekStart)` và lùi 32KB cho các phiên active (<30m). Lọc sự kiện `Source == "MODEL" && Type == "PLANNER_RESPONSE"` chống đếm trùng.
* **Bóc tách danh tính an toàn từ `state.vscdb`**: Mở CSDL IDE bằng driver độc lập `sqlite_detector` ở chế độ chỉ đọc tuyệt đối `file:%s?mode=ro`, thiết lập `SetMaxOpenConns(1)` và `SetMaxIdleConns(0)`, tự động giải mã Protobuf nhận diện tài khoản `Pham Ethan`, gói `Google AI Ultra (20X Ultra Tier)`, installation UUID và hạn token OAuth mà không bao giờ khóa CSDL IDE, zero mật khẩu, zero can thiệp.
* **OpenAI/Codex Monitor không đọc credential**: Chỉ quét `~/.codex/sessions/**/*.jsonl`; tuyệt đối không mở `~/.codex/auth.json`, không lưu prompt, command, response content hay access token. Bỏ qua êm ái khi thư mục sessions chưa tồn tại mà không spam log.
* **Anthropic Claude Monitor độc lập**: Chỉ đọc `~/.claude/projects/**/*.jsonl`; trả về `UNAVAILABLE` trung thực kèm 0 nodes/links khi vắng thư mục `projects/`, tự động khử spam log cảnh báo định kỳ.
* **Kiến trúc dữ liệu chân thực (Zero Hardcode Contract)**: 100% số liệu token, chi phí FinOps USD, tác vụ hạm đội agent và đồ thị Topology được tính toán động từ CSDL SQLite WAL và nhật ký thực địa, loại bỏ triệt để mock data hay hằng số giả lập.
* **Không thể bị khóa tài khoản AI:** Không làm méo mó lưu lượng mạng, không giả mạo token, không spam API và không gây ảnh hưởng tới hạn ngạch dịch vụ.

Chi tiết báo cáo kiểm toán an ninh: Xem tại [TokenMonitor_06-Security_Policy_Compliance_20260908.md](docs/TokenMonitor_20260908/TokenMonitor_06-Security_Policy_Compliance_20260908.md).

---

## 📊 Bảng Điều Khiển Web Dashboard: Tab Tổng Hợp Đa LLM & 3 Tab Provider Độc Lập

Giao diện Web tại [http://127.0.0.1:9090](http://127.0.0.1:9090) được tổ chức với **Tab đầu tiên là 🌐 Tổng Hợp Đa LLM** mang lại bức tranh FinOps toàn cảnh, tiếp nối bởi **3 Menu/Tab độc lập cho từng loại LLM** đạt 100% tính năng, báo cáo, biểu đồ và số liệu đồng nhất:

### 0. 🌐 Tab 1: Tổng Hợp Đa LLM (Cross-LLM FinOps Master Overview)
* **Vị trí & Mặc định**: Đặt ở **vị trí đầu tiên** trên thanh điều hướng chính (`.provider-master-bar`), tự động kích hoạt làm màn hình mặc định khi mở Dashboard.
* **Bức tranh toàn cảnh FinOps**: Tổng hợp số liệu tiêu thụ token, chi phí quy đổi USD, số cuộc gọi/phiên và số tác vụ agent từ cả 3 nguồn: Google Antigravity (CSDL SQLite WAL), OpenAI Codex (`~/.codex/sessions`), và Anthropic Claude (`~/.claude/projects`).
* **5 Thẻ KPI Toàn Hệ Thống**:
  * **Grand Multi-LLM Tokens**: Tổng token toàn bộ LLM tích lũy.
  * **Grand Cost USD**: Tổng chi phí quy đổi USD tích lũy toàn hệ thống.
  * **#1 Heavy Consumer**: Dự án dẫn đầu tiêu thụ token kèm tỷ lệ % áp đảo.
  * **#1 Most Active**: Dự án hoạt động nhiều nhất theo tổng số lượt gọi và tác vụ agent.
  * **Overall Cache Savings**: Tỷ lệ tiết kiệm Cache chung toàn hệ thống.
* **Bảng Xếp Hạng Dự Án (Project FinOps Leaderboard)**:
  * Huy hiệu thứ hạng trực quan (#1 Vàng 🥇, #2 Bạc 🥈, #3 Đồng 🥉...).
  * 3 nút chuyển đổi sắp xếp tức thì: `[⚡ Theo Token]`, `[💰 Theo Chi Phí USD]`, `[🔥 Theo Mức Độ Hoạt Động]`.
  * Thanh tiến độ (Progress Bar) trực quan tỷ lệ phần trăm so với dự án dẫn đầu.
  * Huy hiệu tỷ lệ phân bổ nhà cung cấp (Google %, Codex %, Claude %) cho từng dự án.
* **Biểu Đồ So Sánh Trực Quan (Interactive ECharts)**:
  * **Biểu đồ Cột Chồng (Stacked Bar Chart)**: So sánh cơ cấu token từng dự án phân rã theo 3 LLM.
  * **Biểu đồ Tròn (Donut Chart)**: Tỷ trọng phân bổ chi phí tiền tệ giữa các dự án trong hệ thống.
* **Đồng bộ 5 mốc thời gian**: Hỗ trợ đầy đủ `today`, `24h`, `7d`, `30d`, `all`.

### 1. 🍄 Google Antigravity (Gemini Ultra / Pro / Flash)
* **Nguồn dữ liệu**: Local Brain Logs tại `~/.gemini/*/brain/**/transcript.jsonl` (quét đa thư mục brain tự động) và `%APPDATA%\...\state.vscdb` (`mode=ro`).
* **Cơ chế thu thập**: Delta Byte Offset con trỏ `f.Seek(lastOffset)` + lùi 32KB cho chat active < 30m; giải mã Protobuf tự động nhận diện `Pham Ethan` và hạng `Google AI Ultra (20X Ultra Tier)`.
* **Mô hình chính**: Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.1 Pro, Gemini Ultra.

### 2. 🟢 OpenAI Codex (GPT-5.6 Sol / GPT-6 Astra)
* **Nguồn dữ liệu**: Local Session Logs tại `~/.codex/sessions/**/*.jsonl`.
* **Cơ chế bóc tách**: Bóc tách chính xác từ sự kiện `event_msg -> token_count -> info` (`total_token_usage` và `last_token_usage`) khi `tot.TotalTokens > *prevTotalTokens`, triệt tiêu đếm trùng theo turn.
* **Số liệu thực địa (Ground Truth)**: **40 sessions .jsonl**, **360.88M tokens (360,889,443 tokens)** (359.14M prompt, 1.74M output, 642.6k reasoning, 332.36M cached với **92.5% cache hit**), phân bổ trên **19 workspaces**, đồ thị Topology 4 tầng gồm **134 nodes** (1 root, 19 hubs, 19 orchs, 95 subworkers) và **133 links**.
* **Quy tắc lọc thời gian**: Do phiên gần nhất vào ngày **2026-08-24**, các mốc `today`/`24h`/`7d` trả về **0 tokens**, `30d` trả về **1.54M tokens**, `all` trả về toàn bộ **360.88M tokens**.
* **Mô hình chính**: `gpt-5.6-sol`, `gpt-6-astra`, `gpt-4o`.
* **Đặc thù riêng**: Tích hợp đồng hồ hạn mức Rate Limits 5h & 7d (`primary` & `secondary` quota windows), bỏ qua êm ái khi chưa có thư mục sessions.

### 3. 🟣 Anthropic Claude (Claude 3.7 Sonnet / 3.5 Sonnet)
* **Nguồn dữ liệu**: Local Project Logs tại `~/.claude/projects/**/*.jsonl`.
* **Cơ chế bóc tách**: Trích xuất chính xác từ metadata `usage` trong tin nhắn trợ lý (`input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `thinking_tokens`).
* **Tính toán động**: `ToolSuccessPercent` tính 100% động từ telemetry thực tế (0 tool calls $\to$ **`0.0%`**, loại bỏ hoàn toàn mock `98.5%`).
* **Trạng thái rỗng trung thực**: Khi máy trạm chưa có thư mục `projects/`, hệ thống trả về `UNAVAILABLE` trung thực kèm đồ thị rỗng **0 nodes, 0 links, 0 projects**.
* **Mô hình chính**: `claude-3-7-sonnet`, `claude-3-5-sonnet`.
* **Đặc thù riêng**: Tích hợp bảng thống kê tần suất công cụ CLI (`Bash`, `FileEdit`, `Glob`, `Grep`, `WebSearch`), khử spam log cảnh báo định kỳ qua `os.IsNotExist` và deduplication `lastLoggedErr`.

---

### 🌟 Hệ Thống 100% Đồng Nhất Trên Cả 3 Tab (Shared Parity Features)

Mỗi tab của từng loại AI đều sở hữu đầy đủ toàn bộ các phân hệ giám sát cao cấp:
1. **6 Thẻ KPI Metric Cards Đỉnh Cao (Top Metrics)**:
   * **Grand Tokens**: Tổng lưu lượng token tích lũy.
   * **Prompt (Input)**: Ngữ cảnh nạp mô hình.
   * **Thinking (CoT) / Reasoning**: Tư duy suy luận sâu Chain-of-Thought.
   * **Output Tokens**: Kết quả văn bản/mã nguồn sinh ra.
   * **Cached Tokens & % Cache Hit**: Lượng token đọc từ bộ đệm ngữ cảnh và tỷ lệ trúng cache.
   * **Quy Đổi USD ($)**: Giá trị ước tính theo biểu giá thực tế kèm số tiền tiết kiệm được nhờ Context Caching. Bấm vào thẻ để mở **Hộp Thoại FinOps Modal** tương tác linh hoạt với Preset tự động theo từng LLM.
2. **4 Góc Nhìn Sub-Navigation Đồng Bộ**:
   * **📊 Tổng Quan & Đa Model**: Biểu đồ Chart.js Timeline 3 chế độ (Token / Model / Calls) + Apache ECharts Donut phân bổ tỷ lệ và Timeline xu hướng theo từng model kèm các Chip lọc tương tác.
   * **📅 Bảng Lịch Sử Chi Tiết**: Bảng thống kê tiêu thụ chi tiết chuẩn 10 cột theo ngày/giờ (Prompt, Thinking, Output, Cache, Cache %, Cuộc gọi, Tốc độ, USD) + Bảng danh sách phiên/dự án thực tế.
   * **🤖 Multi-Agent Fleet & Topology**: 4 thẻ KPI Tác nhân, đồ thị mạng lưới phân cấp 4 tầng 60 FPS với hiệu ứng dòng chảy năng lượng GPU-accelerated:
     - **Neo Nhãn Đường Cong Chuẩn Xác (`edgeLabel`)**: Cấu hình chuẩn `edgeLabel` với `position: 'middle'` giúp nhãn số liệu (tokens, calls, loại luồng) bám sát chính xác vào vị trí trung điểm của đường cong Bezier.
     - **Đồng Bộ Tọa Độ Toàn Cục Sống (`transformCoordToGlobal`)**: Lớp canvas overlay `#topo-flow-overlay` tính toán tọa độ trung điểm nhãn `(lx, ly)` đồng bộ tuyệt đối với ma trận biến đổi tọa độ toàn cục khi người dùng Zoom / Pan / Roam, đảm bảo nhãn và đường nối không bao giờ bị lệch vị trí (Zero Drift).
     - **Hai Chế Độ Xem Nhãn**: Hỗ trợ đầy đủ chế độ tinh gọn `🏷️ Gọn Gàng` (hiện nhãn khi hover) và chế độ `📑 Hiện Tất Cả` (hiện nhãn tĩnh trên mọi đường truyền).
     - **Tự Động Chuyển Đổi Theo Provider (`syncAgentFleetControlsForProvider`)**: Khi chọn tab OpenAI Codex hoặc Anthropic Claude, hệ thống tự động ẩn các nút Dual View, Concurrency, Gantt và kích hoạt ngay đồ thị Topology tương ứng. Khi quay lại Google Antigravity, toàn bộ các nút điều khiển được tự động khôi phục.
     - **Tự Động Nhận Diện & Focus Vào Dự Án Đang Hoạt Động (Active Project Auto-Focus)**: Tự động phát hiện dự án đang có tác vụ `RUNNING` / `ACTIVE` (ví dụ: `TokenMonitor (GoLangDev)`), tự động chọn option trên dropdown và refetch/zoom vào riêng cụm dự án đang chạy mà không cần người dùng thao tác thủ công (`updateTopologyProjectSelect`, `loadAgentFleetData`).
     - **Động Cơ Responsive Auto-Fit & Căn Giữa Động [midX, midY]**: Thuật toán `calculateTopologyAutoFit` tự động xác định bounding box của tập node, căn giữa hình học tại $[midX, midY]$, bổ sung đệm an toàn $160\text{px} \times 150\text{px}$ và co giãn tối ưu theo khung nhìn ($92\%$ width, $84\%$ height). Triệt tiêu hoàn toàn hiện tượng tràn lề ngang hay trôi lệch tọa độ khi tải lại trang (Anti-Drift Storage Contract).
   * **🔲 Xem Toàn Bộ**: Cuộn mượt mà xem trọn gói toàn bộ dashboard trên một màn hình.
3. **Bộ Điều Khiển Thời Gian Đơn Nhất (Single Unified Time-Range Controller)**:
   * Duy nhất 1 cụm 5 mốc thời gian chuẩn (`⚡ Hôm Nay`, `24 Giờ`, `7 Ngày`, `🗓️ 30 Ngày`, `♾️ Toàn Bộ`) ở góc phải thanh sub-navigation trên cùng, loại bỏ hoàn toàn hiện tượng trùng lặp nút gây rối mắt.
4. **Nhận Diện Trạng Thái Tải Chân Thực (Zero Ghost Concurrency)**:
   * Khi người dùng đã đóng app Codex hoặc Claude, hệ thống nhận diện tức thì và hiển thị chính xác **`ACTIVE CONCURRENCY: 0 / 16`** (`Idle • 0 Active Tasks (Đã tắt ứng dụng)`), badge header tự chuyển sang **`STANDBY`**, tuyệt đối không đếm ảo số lượng node lịch sử.

---

## 💡 Hệ Thống Interactive Floating Type-Hint Tooltip

Hệ thống tích hợp công cụ giải thích kỹ thuật tương tác trực quan với cơ chế **Dynamic Type-Hints theo ngữ cảnh từng AI Provider**:
* **Rê chuột (Hover)** vào bất kỳ thẻ KPI, huy hiệu gói cước hay tiêu đề cột bảng nào để xem:
  * 💡 Tiêu đề chuẩn hóa kèm tên tiếng Việt dễ hiểu.
  * 🏷️ Thẻ phân loại kỹ thuật (`REAL-TIME LOAD`, `INPUT CONTEXT`, `RELIABILITY`...).
  * 📝 Mô tả bản chất và ý nghĩa của thông số (tự động điều chỉnh theo Antigravity, OpenAI Codex hoặc Anthropic Claude).
  * 🎯 Hướng dẫn hành động và ngưỡng cảnh báo an toàn.
  * ⏱️ Chu kỳ cập nhật dữ liệu tương ứng.
* **Đồng Bộ Header Theo Provider**: Khi chuyển giữa các tab Google Antigravity, OpenAI Codex và Anthropic Claude, hàm `updateCodexHeader()` và `updateClaudeHeader()` tự động cập nhật động các thuộc tính `data-hint-title`, `data-hint-tag`, `data-hint-desc`, `data-hint-source`, `data-hint-note` trên các thành phần Header (Account Badge, Quota Pill, Reset Expiry, Status, Workspace CWD, Sync button) phản ánh chính xác ngữ cảnh kỹ thuật của từng provider.
* **Tự Động Căn Biên Tránh Tràn Màn Hình (Auto-Flip):** Tooltip tự tính toán khoảng cách viền cửa sổ để mở lên trên, xuống dưới, sang trái hoặc sang phải mượt mà.

---

## 🌐 Danh Sách REST APIs Chuẩn

Hệ thống cung cấp danh mục 20 endpoint REST API chuẩn (cùng 3 static/doc routes và 1 endpoint /healthz = 24 routes) qua cổng `9090`:

| Method | Endpoint | Mô tả |
| :---: | :--- | :--- |
| `GET` | `/healthz` | Kiểm tra tính sống còn của Daemon (`{"status":"UP",...}`) |
| `GET` | `/api/projects/leaderboard` | Bảng xếp hạng FinOps Đa LLM hợp nhất (Google, Codex, Claude) (`?range=today\|24h\|7d\|30d\|all&sort=tokens\|cost\|activity`) |
| `GET` | `/api/account` | Lấy hồ sơ tài khoản, gói cước và TTL đếm ngược phiên làm việc Antigravity |
| `GET` | `/api/metrics/summary` | Thống kê tổng quan Token, cuộc gọi, độ trễ và Cache Hit Rate (`?range=today\|24h\|7d\|30d\|all`) |
| `GET` | `/api/metrics/timeseries` | Dữ liệu chuỗi thời gian cho biểu đồ Chart.js (`?range=today\|24h\|7d\|30d\|all`) |
| `GET` | `/api/metrics/daily` | Dữ liệu tổng hợp lịch sử theo ngày (`?range=today\|30d` hoặc `?days=30`) |
| `GET` | `/api/metrics/models` | Tỷ lệ phân bổ Token và cuộc gọi theo từng Model AI (`?range=today\|24h\|7d\|30d\|all`) |
| `GET` | `/api/metrics/models/timeseries` | Chuỗi thời gian chuyên sâu cho từng model AI cụ thể (`?model=...&range=today\|30d`) |
| `GET` | `/api/agents/summary` | Tóm tắt Multi-Agent Fleet Antigravity: Tổng agent, Active Concurrency, tỷ lệ thành công, token offloaded |
| `GET` | `/api/agents/concurrency` | Chuỗi thời gian tải đồng thời và trần an toàn |
| `GET` | `/api/agents/gantt` | Danh sách tiến trình Gantt thực tế gom theo vai trò (`roles`) hoặc phiên (`sessions`) |
| `GET` | `/api/agents/gantt/packets` | Danh sách các gói tin tiến trình tác vụ Subagent định dạng timeline packets (`?range=today\|24h\|7d\|30d\|all`) |
| `GET` | `/api/agents/graph` | Lấy sơ đồ mạng lưới topo tác nhân & workspace Antigravity (`?project=...&range=today\|24h\|7d\|30d\|all`) |
| `GET` | `/api/openai/dashboard` | Dashboard OpenAI/Codex cục bộ (`?range=today\|24h\|7d\|30d\|all`), gồm token, workflow, models, rate limits và sessions an toàn |
| `GET` | `/api/openai/graph` | Sơ đồ mạng lưới Topology phân cấp 4 tầng cho OpenAI Codex (`?range=today\|24h\|7d\|30d\|all`) |
| `POST` | `/api/openai/refresh` | Quét lại `~/.codex/sessions/**/*.jsonl` theo yêu cầu; không truy cập credential |
| `GET` | `/api/claude/dashboard` | Dashboard Anthropic Claude Code CLI cục bộ (`?range=today\|24h\|7d\|30d\|all`), gồm token, tools, models, projects |
| `GET` | `/api/claude/graph` | Sơ đồ mạng lưới Topology phân cấp 4 tầng cho Anthropic Claude (`?range=today\|24h\|7d\|30d\|all`) |
| `POST` | `/api/claude/refresh` | Quét lại `~/.claude/projects/**/*.jsonl` theo yêu cầu; không truy cập credential |
| `GET\|POST`| `/api/sync/history` | Kích hoạt quét và đồng bộ lại 154 thư mục lịch sử vào SQLite |
| `POST` | `/api/test/simulate` | Phát sự kiện gọi mô hình mẫu phục vụ thử nghiệm giao diện |

---

## 🎨 Sơ Đồ Kiến Trúc & Dòng Chảy Tương Tác (Interactive Archify Diagrams)

Dự án tích hợp các sơ đồ kiến trúc động chuẩn Production tạo bởi công cụ **Archify**:
* 🌐 **Sơ Đồ Hạm Đội Đa Dự Án & Hoạt Họa Dòng Chảy 60 FPS:** [docs/workspace-overview.html](docs/workspace-overview.html)  
  *Toàn cảnh phối hợp song song 3 cụm dự án độc lập (`MCREDIT ↔ TieuChuanHardeningLinux ↔ TokenMonitor`), thuật toán phân giải dự án động 4 tầng (`resolveSubagentProject`), phân tách cụm ngang 620px và hiệu ứng dòng chảy năng lượng 60 FPS GPU-accelerated.*
* 🏛️ **Bản Đồ Kiến Trúc & Ranh Giới Cách Ly:** [docs/tokenmonitor-architecture.html](docs/tokenmonitor-architecture.html)  
  *Trực quan hóa cấu trúc thành phần, ranh giới an toàn của tiến trình cục bộ, hỗ trợ Pan/Zoom, xem theo 4 góc nhìn (Guided Views), chế độ Sáng/Tối và xuất ảnh PNG/SVG.*
* 🌊 **Dòng Chảy Dữ Liệu Token (Telemetry Dataflow Trace):** [docs/tokenmonitor-dataflow.html](docs/tokenmonitor-dataflow.html)  
  *Mô phỏng động (Trace Animation) hành trình dữ liệu token 5 giai đoạn: Nguồn Log -> Thu Thập -> Xử Lý & Đệm -> Lưu Trữ Bền Vững -> Trực Quan Hóa.*
* 🤖 **Đặc Tả Kiến Trúc Hạm Đội Đa Tác Nhân:** [docs/TokenMonitor_Team_Agent_Fleet_Architecture.md](docs/TokenMonitor_Team_Agent_Fleet_Architecture.md)  
  *Cẩm nang kỹ thuật 7 chương: 3 cụm dự án song song, 5 vai trò Subagent, giải thuật phân giải dự án 4 tầng, giao thức streaming, thuật toán canvas 60 FPS và 5 REST API chuyên biệt.*

---

## 📚 Bộ Tài Liệu Kỹ Thuật (Architecture & Runbook Suite)

Bộ tài liệu vận hành và kiến trúc hoàn chỉnh theo chuẩn Production nằm trong thư mục `docs/TokenMonitor_20260908/`:

| Phase | Tài liệu | Nội dung chính |
| :---: | :--- | :--- |
| **00** | [Prerequisites & Environment](docs/TokenMonitor_20260908/TokenMonitor_00-Prerequisites_20260908.md) | Yêu cầu phần cứng, hệ điều hành, ma trận cổng mạng và phụ thuộc runtime |
| **01** | [System Architecture & ERD](docs/TokenMonitor_20260908/TokenMonitor_01-Architecture_20260908.md) | Kiến trúc tổng thể, In-Memory Ring Buffer, SQLite 5 bảng, Multi-Agent Fleet, Type Hints |
| **02** | [High Availability & Deployment](docs/TokenMonitor_20260908/TokenMonitor_02-HA_Deployment_20260908.md) | Phương án triển khai dự phòng, failover, backup và khôi phục dữ liệu |
| **03** | [Installation & Deployment Guide](docs/TokenMonitor_20260908/TokenMonitor_03-Install_Deploy_20260908.md) | Hướng dẫn cài đặt từng bước trên Windows (PowerShell) và Linux với Pure Go |
| **04** | [Tuning & Optimization](docs/TokenMonitor_20260908/TokenMonitor_04-Tuning_20260908.md) | Tối ưu hóa hiệu năng, tinh chỉnh SQLite PRAGMA, Ring Buffer và TTL sweep |
| **05** | [Troubleshooting & Edge Cases](docs/TokenMonitor_20260908/TokenMonitor_05-Troubleshooting_20260908.md) | Ma trận xử lý sự cố, playbooks xử lý Active Concurrency và định dạng log chuẩn |
| **06** | [Security & AI Policy Compliance](docs/TokenMonitor_20260908/TokenMonitor_06-Security_Policy_Compliance_20260908.md) | Báo cáo kiểm toán an ninh, luồng cách ly dữ liệu và chứng minh tuân thủ ToS |

### 📑 Tài Liệu Tham Chiếu Kỹ Thuật (References)
* 🗄️ **[Ref 001 - SQL Schema](docs/TokenMonitor_20260908/references/TokenMonitor_Ref_001_schema.sql):** Tệp DDL khởi tạo cơ sở dữ liệu SQLite đầy đủ 5 bảng và 7 indexes.
* ⚙️ **[Ref 002 - Config Template](docs/TokenMonitor_20260908/references/TokenMonitor_Ref_002_config_template.yaml):** Mẫu cấu hình YAML chuẩn đầy đủ các trường.
* 📋 **[Ref 003 - Gemini Metadata Spec](docs/TokenMonitor_20260908/references/TokenMonitor_Ref_003_gemini_usage_metadata_spec.json):** Đặc tả JSON mẫu cấu trúc `usageMetadata` của Google Gemini API.
* 📊 **[Ref 004 - Database ERD & Dictionary](docs/TokenMonitor_20260908/references/TokenMonitor_Ref_004_database_erd.md):** Sơ đồ thực thể mối quan hệ ERD và từ điển dữ liệu chi tiết cho 5 bảng.
