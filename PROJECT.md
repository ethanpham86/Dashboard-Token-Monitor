# Project: TokenMonitor Codebase Review & Parity Normalization

## Architecture
TokenMonitor is an enterprise-grade observability daemon for Gemini / Antigravity IDE token usage.
The system consists of:
- **Station 1 & 2 (Collector Tailer)**: `collector/tailer.go` monitors `~/.gemini/antigravity/brain/**/transcript.jsonl` (and all workspace brain instances, ~154 folders). Tracks byte offsets, filters `source == "MODEL" && type == "PLANNER_RESPONSE"`, discards `GENERIC` tool steps, applies heuristic prompt token estimation (`EstimatePromptTokens`), ignores folders with `backup`, `tmp`, `profile`, rewinds 32 KB for active chats (<30m), and executes background backfill with 200-event direct flush. Additionally parses subagent task invocations (`invoke_subagent`), classifies them into 5 distinct subagent roles (`Research Agent`, `Codebase Explorer`, `Self-Branch Worker`, `Verification Tester`, `PKI Auditor`), and records subagent lifecycles in `agent_fleet_telemetry`.
- **Station 3 (Async Buffer)**: `collector/buffer.go` queues events in an in-memory channel (`capacity = 1000`). Triggers batch flush when reaching 100 items or on a 1s timer. Non-blocking drop on queue full (no disk spooling).
- **Station 4 (SQLite Database, Rollup & Auto-Backup)**: `storage/db.go`, `storage/repository.go` & `storage/backup.go` manage SQLite storage (`./data/token_monitor.db`) via `modernc.org/sqlite` (Pure Go, `CGO_ENABLED=0`). Enforces 6 PRAGMAs registered via driver connection hook `sqlite.RegisterConnectionHook` in `init()` and executed via `applyPragmas()`. 5 tables (`accounts`, `auth_sessions`, `token_usage_logs`, `token_usage_hourly_rollup`, `agent_fleet_telemetry`) with `ON DELETE CASCADE`. 7 strategic indexes including partial unique index `idx_token_dedup_chat` and 3 dedicated agent fleet indexes (`idx_agent_fleet_started`, `idx_agent_fleet_role`, `idx_agent_fleet_subagent`). `RollupHourlyMetrics()` runs on startup and every 5 minutes (`cfg.Database.RollupIntervalSeconds`). `GetAgentFleetSummary()` executes an automatic 2-minute TTL sweep on stale running tasks and enforces a 2-minute query window for real-time active concurrency. **Auto-Backup Subsystem**: `storage/backup.go` runs periodic background snapshots via SQLite native `VACUUM INTO`, atomically merging WAL into single-file `.db` snapshots (`token_monitor_backup_YYYYMMDD_HHMMSS.db`), updating `data/backup/token_monitor.db` for instant 1-click restore, and pruning old backups based on `max_keep` retention policy.
- **Station 5 & 6 (Web APIs & Dashboard)**: `web/handler.go` serves 19 `/api/*` REST APIs plus `/healthz`, including `/api/openai/*` and `/api/claude/*`. `web/static/` hosts 3 independent Top-Level AI Provider Tabs (Google Antigravity, OpenAI Codex, Anthropic Claude) with 100% feature parity, 4 sub-views (Tổng Quan, Bảng Lịch Sử, Multi-Agent Fleet & Topology, Xem Toàn Bộ), single authoritative time filter (`today`, `24h`, `7d`, `30d`, `all`), 60FPS interactive canvas topology, and the Interactive Floating Type-Hint Tooltip Engine.
- **OpenAI/Codex Local Monitor**: `collector/codex_monitor.go` incrementally caches `~/.codex/sessions/**/*.jsonl`, extracts only usage/workflow/rate-limit metadata, never traverses `auth.json`, and never decodes, retains, or emits prompt/response/command content.
- **Anthropic Claude Local Monitor**: `collector/claude_monitor.go` incrementally parses `~/.claude/projects/**/*.jsonl`, extracts token breakdown, model calls, tool executions, and generates 4-layer topology hierarchy without accessing credentials.
- **Detector Subsystem**: `storage/detector.go` opens `%APPDATA%\Antigravity IDE\User\globalStorage\state.vscdb` (or `ANTIGRAVITY_STATE_DB` / fallback) strictly `mode=ro` using isolated driver `sqlite_detector`. Dual-mode JSON and Protobuf stream extraction. Zero credentials stored.
- **Master Offline Documentation Portal**: `docs/index.html` embeds 100% offline documentation via inlined `DOCS_DATA` object, compliant with 6 documentation standards.
- **Git Repository & Security Policy**: Remote URL `https://github.com/ethanpham86/Dashboard-Token-Monitor.git`. Enforces strict exclusion of all skill definition markdown files (`**/*skill*.md`, `skills/`) in `.gitignore` to safeguard workstation instructions.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | F01-Survey-Storage | Survey storage schema, PRAGMA connection hooks, CASCADE foreign keys, 5 tables, 7 indexes, detector | M0 | Survey Miner 1 |
| 2 | F02-Survey-Collector | Survey collector tailer, step filtering, token estimation formula, buffer specs, rollup method, multi-agent classification | M0 | Survey Miner 2 |
| 3 | F03-Survey-Web-Docs | Survey config blocks, 12 `/api/*` endpoints, documentation standards, 4-view dashboard, type-hint engine, topology graph with 5 time ranges (today/24h/7d/30d/all) | M0 | Survey Miner 3 |
| 4 | F04-Doc-ERD-Sync | Update `docs/TokenMonitor_Database_ERD.md` & `Ref_004_database_erd.md` (5 tables, `agent_fleet_telemetry`, 7 indexes, DDL sync) | M1 | Survey Findings |
| 5 | F05-Doc-TokenSpec-Sync | Update `docs/TokenMonitor_Token_Estimation_Spec.md` (fix math table for step 100/210/410, add output/thinking formulas / 3.4, add dual ingestion spec) | M1 | Survey Findings |
| 6 | F06-Doc-ConfigGuide-Sync | Update `docs/TokenMonitor_Configuration_Guide.md` (codebase mapping with 5 tables, 7 indexes, Multi-Agent Fleet, 11 REST APIs, Type-Hint engine) | M1 | Survey Findings |
| 7 | F07-Doc-Runbooks-Sync | Update `docs/TokenMonitor_20260908/` (`01-Architecture` 5 tables & tailer sequence, `03-Install` pure Go modernc, `04-Tuning` subagent TTL sweep 2m, `05-Troubleshooting` active concurrency matrix, `Ref_001_schema.sql` 5 tables) | M1 | Survey Findings |
| 8 | F08-Doc-Readme-Sync | Update `README.md` and `docs/README.md` reflecting complete system architecture, 5 tables, 11 routes, 4 views, Type Hints | M1 | Survey Findings |
| 9 | F09-Portal-Recompile | Master portal verification, verifying zero CORS errors on `file:///`, 100% offline autonomy | M2 | User Requirement R3 |
| 10 | F10-Archify-Diagrams | Synchronize `docs/tokenmonitor-dataflow.json` and `docs/tokenmonitor-architecture.json` wiring | M2 | Survey Findings |
| 11 | F11-Zero-Code-Diff-Verify | Verify `git diff -- "*.go" "config.yaml"` is 100% compliant with actual codebase ground truth | M3 | Strict Constraint |
| 12 | F12-Test-Suite-Pass | Run `go test -v -count=1 ./...` ensuring 100% PASS | M3 | User Requirement R4 |
| 13 | F13-Binary-Compile | Verify `go build -o token_monitor.exe .` compiles cleanly with `modernc.org/sqlite` | M3 | User Requirement R4 |
| 14 | F14-Forensic-Audit-Report | Produce comprehensive audit summary certifying 100% code-doc parity across all 11 technical documents | M3 | User Requirement R4 |
| 15 | F15-Auto-Backup-Enterprise | Periodic atomic non-blocking backup via `VACUUM INTO`, retention pruning (`max_keep`), zero Google Drive sync conflicts | M4 | Enterprise Resiliency |
| 16 | F16-Team-Agent-Fleet-Architecture | Complete architecture specification for Multi-Agent Fleet, Cross-Workspace Parallel Bridge (MCREDIT + TokenMonitor), streaming exchange protocols, 60 FPS GPU-accelerated animated flowing arrows, SQLite schema & 5 REST endpoints (`TokenMonitor_Team_Agent_Fleet_Architecture.md`, `docs/workspace-overview.html`, `web/static/index.html`) | M5 | Team Agent Fleet Spec |
| 17 | F17-OpenAI-Codex-Local-Monitor | Read-only Codex session collector, usage-limit telemetry, privacy-safe workflow metrics, 2 REST APIs, fifth dashboard tab, config and automated tests | M6 | Personal OpenAI Observability |
| 18 | F18-Anthropic-Claude-Monitor | Read-only Claude Code CLI project collector (`~/.claude/projects/**/*.jsonl`), token breakdown, tools telemetry, REST APIs `/api/claude/dashboard`, `/api/claude/graph`, `/api/claude/refresh` | M7 | Personal Claude Observability |
| 19 | F19-Multi-Provider-100-Parity | 100% architectural, visual and functional parity across Google Antigravity, OpenAI Codex, and Anthropic Claude. Standardized 6 Grand Metric KPI cards, dynamic Chart.js timeline (3 tabs), Apache ECharts donut & timeline, 10-column granular daily table, Multi-Agent Fleet / 60FPS Topology Graph, and dynamic FinOps Modal | M7 | Full Parity Engine |
| 20 | F20-Realtime-Active-Concurrency | Zero Ghost Concurrency: Directly map active concurrency to collector live status (`active_sessions == 0` -> `0 / 16 (Idle • 0 Active Tasks (Đã tắt ứng dụng))`, `STANDBY` badge) eliminating historical node-counting bugs | M7 | High-Fidelity Concurrency |
| 21 | F21-Single-Unified-Time-Controller | Remove duplicate time buttons from Topology toolbar; establish single authoritative time selector at top-right sub-navigation bar (`today`, `24h`, `7d`, `30d`, `all`); compact controls to prevent wrapping and clutter | M7 | Ergonomic Clean UI |
| 22 | F22-Topology-EdgeLabel-Curve-Alignment | Standardize `edgeLabel` in ECharts series with `position: 'middle'`, synchronize `(lx, ly)` midpoint coordinate calculation on `#topo-flow-overlay` via `transformCoordToGlobal` across Zoom/Pan/Roam, and support both `🏷️ Gọn Gàng` (smart/hover) and `📑 Hiện Tất Cả` (static on all curves) label modes | M8 | Topology Curve Alignment |
| 23 | F23-Provider-Tabs-Dynamic-TypeHints | Standardize 3 Top-Level AI Provider Tabs (Google Antigravity, OpenAI Codex, Anthropic Claude), automatic view redirection to Topology Graph for Codex/Claude via `syncAgentFleetControlsForProvider()`, and dynamic Type-Hint tooltips tailored to each provider | M8 | Provider Sync & Tooltips |
| 24 | F24-Audit-Backend-Metrics-Zero-Hardcode | Audit all metrics and account APIs to calculate 100% dynamically via SQL SUM and protobuf state.vscdb | M9 | User Request R1 |
| 25 | F25-Audit-Agent-Fleet-Topology-Zero-Mock | Eliminate all sample data blocks (680000, 12466029, 50000, etc.) in agent fleet & topology, derive 100% dynamically from agent_fleet_telemetry | M9 | User Request R2 |
| 26 | F26-Audit-Frontend-Zero-Hardcode | Ensure web UI charts, cards and HUD bind 100% dynamically to backend JSON without hardcoded arrays or DOM static values | M9 | User Request R3 |
| 27 | F27-Test-Rebuild-Integrity-Certification | Run go test -v -count=1 ./... PASS 100%, recompile token_monitor.exe, publish AUDIT_REPORT_DATA_INTEGRITY.md | M9 | User Request R4 |
| 28 | F28-Doc-Multi-Project-Fleet-Sync | Synchronize 4-cluster taxonomy (`TokenMonitor`, `MCREDIT`, `TieuChuanHardeningLinux`, `ProjectScriptOS`), `resolveSubagentProject` 4-tier algorithm in `storage/repository.go:1389-1561` with Priority 1 CWD path matching and collision-blocking guard `!tokenmonitor`, cluster separation `stepX = 780.0px`, and real geometry (L0:105, L1:210, L2:310, L3:350-440) | M10 | User Request R2 |
| 29 | F29-Zero-Fake-Motion-Spec | Document 4-stage value chain and 45s liveness threshold in `collector/tailer.go:577`, 2m inline TTL sweep in `repository.go:1004-1010`, and Canvas 60 FPS rendering invariants (3 chevrons + 3 photons vs 1 resting chevron at t=0.5, 0 photons) | M10 | User Request R2 |
| 30 | F30-Database-ERD-Config-Sync | Resolve duplicate Section 6 in `docs/TokenMonitor_Database_ERD.md`, ensure 100% parity with `storage/db.go` DDL; add `openai_monitor` and `claude_monitor` blocks to YAML spec in `docs/TokenMonitor_Configuration_Guide.md`, complete struct inventory, add `GET /config.yaml` (23 routes total) | M10 | User Request R3 |
| 31 | F31-Master-Offline-Portal-Audit | Re-compile `docs/index.html` inlined `DOCS_DATA`, verify zero CORS on `file:///`, verify all tests PASS, recompile `token_monitor.exe`, publish audit report | M10 | User Request R4 |
| 32 | F32-Leaderboard-DTOs | Data transfer objects for leaderboard items, provider breakdowns, system KPIs, and API responses | M11 | Survey Miner 1 & 2 |
| 33 | F33-Cross-LLM-Storage-Engine | Cross-LLM storage engine in `storage/repository.go` (`GetAntigravityProjectStats`, `GetProjectsLeaderboard`, unified project resolver, FinOps cost calculation) | M11 | Survey Miner 1 & 2 |
| 34 | F34-REST-API-Leaderboard | REST API endpoint `GET /api/projects/leaderboard?range={...}&sort={...}` in `web/handler.go` | M11 | Survey Miner 3 |
| 35 | F35-Frontend-Tab1-Nav | Place Tab 1 "🌐 Tổng Hợp Đa LLM" as first child in `.provider-master-bar` and default view on startup | M12 | Survey Miner 3 |
| 36 | F36-Frontend-KPIs-Table | 5 system-wide KPI cards, interactive sorting switches (`[⚡ Theo Token]`, `[💰 Theo Chi Phí USD]`, `[🔥 Theo Mức Độ Hoạt Động]`), and Project FinOps Leaderboard table with progress bar and badges | M12 | Survey Miner 3 |
| 37 | F37-Frontend-Dual-Charts | ECharts horizontal stacked bar chart (token breakdown by LLM) and donut chart (cost share) | M12 | Survey Miner 3 |
| 38 | F38-Automated-Tests-Leaderboard | Comprehensive unit tests in `storage/` and handler tests in `web/handler_test.go` covering populated and empty states | M13 | User Requirement R3 |
| 39 | F39-Full-Test-Pass-Binary-Build | 100% PASS on `go test -v -count=1 ./...` and clean compilation of `token_monitor.exe` | M13 | User Requirement R3 |
| 40 | F40-Doc-Portal-Sync | Update architecture guides, configuration guide, and recompile Master Offline Portal `docs/index.html` (zero CORS) | M13 | User Requirement R3 |
| 41 | F41-Forensic-Integrity-Audit | Forensic audit verifying 100% Zero Mock Data contract across backend and frontend | M13 | User Requirement R1 & Audit |
| 42 | F42-Topology-Active-Project-Auto-Focus | Tự động phát hiện dự án đang RUNNING/ACTIVE, auto-select dropdown và refetch/zoom vào cụm dự án đang chạy | M14 | User Requirement R1 |
| 43 | F43-Topology-Responsive-Auto-Fit | Thuật toán calculateTopologyAutoFit([midX, midY]), bounding box, padding 160x150, co giãn theo viewport, Anti-Drift contract | M14 | User Requirement R3 |
| 44 | F44-Topology-3-Layout-Modes-Physics-Scaling | Standardize 3 layout modes (📌 Pinned 4-tier, 🧲 Force 4-tier physics scaling law [800-2200, gravity 0.03-0.06, initLayout circular, friction 0.65], ⭕ Circular symmetry) | M15 | User Requirement R1 & R2 |
| 45 | F45-Topology-Coordinate-Camera-Decoupling | Absolute coordinate decoupling (x/y: undefined, fixed: false in Force/Circular), Layout-Specific Camera Decoupling (effectiveCenter: ['50%','50%'], effectiveZoom: 0.85 for Force/Circular vs fitConfig for Pinned) | M15 | User Requirement R1 & R2 |
| 46 | F46-Topology-1Hour-Project-Filter | 1-hour active project filtering protocol (`LatestActivity`, `IsRunning`, `oneHourAgo = now - 75m`, fallback to 1 most recent project, dropdown bypass) | M15 | User Requirement R1 & R2 |
| 47 | F47-Topology-Zero-Visual-Redundancy | Elimination of static RUNNING text pills, refined node sizes (Root 44, Project 34, Orch 28, Subagent 22), slender edges in Force mode | M15 | User Requirement R1 & R2 |
| 48 | F48-Interactive-Topology-Engine-Skill-Norm | Standardize `interactive_topology_engine` skill with 5 golden rules (both global and workspace) | M15 | User Requirement R1 |
| 49 | F49-Doc-Sync-Master-Portal-Git | 100% Code-to-Doc synchronization across all technical documents, recompile Master Offline Portal `docs/index.html` (zero CORS), full test suite PASS, Git synchronization excluding skills | M15 | User Requirement R2, R3, R4 |
| 50 | F50-Codex-Monitor-Audit-Optimization | Bóc tách chính xác usage Codex: khử đếm trùng baseline delta, xử lý compaction/reset, phân rã stacked token, gán model theo turn_context, cô lập hash path workspace, loại bỏ công cụ giả lập và bảo vệ quyền riêng tư credential | M16 | User Request Audit |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M0 | Forensic Survey & Codebase Ground Truth Audit | F01, F02, F03 (Audit 5 tables, tailer, repository, REST APIs, Type Hints) | none | DONE |
| M1 | Markdown Documentation & Runbook Normalization | F04, F05, F06, F07, F08 (`docs/*.md`, `docs/TokenMonitor_20260908/*`, `README.md`) | M0 | DONE |
| M2 | Offline Master Portal Re-compilation & Diagrams | F09, F10 (`docs/index.html`, `docs/*.json`) | M1 | DONE |
| M3 | Testing, Verification & Final Audit Certification | F11, F12, F13, F14 (Go tests, compile check, walkthrough certification) | M1, M2 | DONE |
| M4 | Enterprise Auto-Backup Subsystem | F15 (`storage/backup.go`, unit & integration test suite) | M3 | DONE |
| M5 | Team Agent Fleet Architecture & 60 FPS Dynamic Animation | F16 (`TokenMonitor_Team_Agent_Fleet_Architecture.md`, `workspace-overview.html`, dynamic 60fps canvas) | M4 | DONE |
| M6 | OpenAI / Codex Personal Work Monitor | F17 (`collector/codex_monitor.go`, `/api/openai/*`, OpenAI dashboard tab, tests and policy boundary) | M5 | DONE |
| M7 | Multi-Provider 100% Parity & Clean UI Normalization | F18, F19, F20, F21 (Claude monitor, 100% parity across 3 LLMs, 0/16 idle concurrency, single time selector) | M6 | DONE |
| M8 | Topology Edge Label Precision & Provider Navigation Standardization | F22, F23 (Edge label curve midpoint alignment, 2 label modes, syncAgentFleetControlsForProvider, dynamic type hints) | M7 | DONE |
| M9 | 100% Zero-Hardcode & Dynamic Data Integrity Audit | F24, F25, F26, F27 | M8 | DONE |
| M10 | 100% Code-to-Doc Synchronization & Master Offline Portal Re-compilation | F28, F29, F30, F31 | M9 | DONE |
| M11 | Backend Cross-LLM Aggregation Engine & REST API | F32, F33, F34 (`GET /api/projects/leaderboard`, DTOs, storage engine, FinOps cost calculation) | M10 | DONE |
| M12 | Frontend Tab 1 "🌐 Tổng Hợp Đa LLM" & Visualizations | F35, F36, F37 (`web/static/index.html`, 5 KPI cards, leaderboard table, stacked bar & donut charts) | M11 | DONE |
| M13 | Comprehensive Testing, Binary Build & Doc Sync | F38, F39, F40, F41 (Unit tests, handler tests, `token_monitor.exe`, `docs/index.html`, forensic audit) | M11, M12 | DONE |
| M14 | Topology Auto-Focus, 60 FPS Bézier Midpoint & Responsive Auto-Fit Audit | F42, F43 (Active project auto-focus, Bézier midpoint P(t=0.5) stickiness, Responsive Auto-Fit [midX, midY], test suite 100% PASS, daemon verification) | M13 | DONE |
| M15 | Production Protocol Standardization & System Parity | F44, F45, F46, F47, F48, F49 (Layout modes, coordinate & camera decoupling, 1h filter, zero visual redundancy, skill norm, doc sync, portal recompile, Git sync) | M14 | DONE |
| M16 | OpenAI Codex Observability & Baseline Delta Optimization | F50 (`collector/codex_monitor.go`, `storage/repository.go`, `web/static/index.html`, tests, `docs/Codex_Monitor_Audit_20260915.md`) | M15 | DONE |

## Interface Contracts
### Ground Truth Rule
- Go source code (`*.go`) and `config.yaml` are the ABSOLUTE GROUND TRUTH.
- Zero undocumented discrepancies.
- All documentation, ERD diagrams, DDL scripts, and API specs match Go code reality 100%.

### Quy Chuẩn Bất Biến Về Topology & Hoạt Họa (Topology & Observability Invariants)
1. **Ranh Giới Dự Án Độc Lập (Workspace Boundary Isolation - Zero Fake Bridges)**:
   - Hệ thống hỗ trợ 4 cụm dự án độc lập hoàn toàn về mã nguồn và logic nghiệp vụ: `TokenMonitor (GoLangDev)`, `MCREDIT (ProjectR)`, `TieuChuanHardeningLinux (Security Standards)`, và `ProjectScriptOS`.
   - Thuật toán phân giải dự án 4 tầng động (`resolveSubagentProject` trong `storage/repository.go:1389-1561`) tự động định vị chính xác task vào cụm dự án tương ứng. **Ưu tiên 1 (Priority 1)**: Nhận diện trực tiếp theo đường dẫn CWD / Workspace folder path (`/tokenmonitor`, `/projectr`, `/tieuchuanhardeninglinux`, `/projectscriptos`) kết hợp rào chắn va chạm bắt buộc `!strings.Contains(normLower, "/tokenmonitor")` triệt tiêu hoàn toàn false-positive keyword matching. Tiếp theo là fast-path convPrefix (`227fb340`, `abe42560`, `b71cdefa`, `5fc429ff`, `574184f1`, `511bb89e`, `challenger-002/003`), cache RAM RWMutex, và quét 60 dòng đầu file transcript.
   - Mỗi cụm dự án trên Topology Graph hình thành một cây phân cấp riêng biệt: `Project Hub -> Primary Orchestrator -> Subagents`, được bố trí với khoảng cách phân tách ngang chuẩn xác $780.0\text{px}$ (`stepX = 780.0`). Tọa độ hình học: Root Level 0 ($X=1500, Y=105$), Hub Level 1 ($X=cx, Y=210$), Orch Level 2 ($X=cx, Y=310$), Subagents Level 3 ($X=cx \pm [0..220], Y=350..440$).
   - **Tuyệt đối KHÔNG tạo liên kết giả định / cầu nối ảo** (như `PARALLEL_BRIDGE`) giữa các cụm dự án khác nhau. 4 dự án chỉ dùng chung môi trường máy trạm và hạn mức AI của Antigravity IDE, không hề có luồng gọi hàm hay trao đổi dữ liệu chéo.
2. **Quy Tắc Chân Thực Thời Gian Thực Trong Hoạt Họa (Real-time Observability Fidelity & Zero Fake Motion)**:
   - **Ngưỡng kích hoạt 45 giây (`collector/tailer.go:577`)**: Chỉ các sự kiện có mốc thời gian trong vòng 45 giây gần nhất (`time.Since(parsedTime) < 45*time.Second`) mới được mang trạng thái `RUNNING`. Mọi sự kiện cũ hơn lập tức chuyển sang `COMPLETED`.
   - **Cơ chế Inline TTL Auto-Sweep 2 phút (`storage/repository.go:1004-1010`)**: Tự động chuyển các task `RUNNING` quá 2 phút sang `COMPLETED`, tính toán `finished_at = started_at + duration_ms`.
   - Hoạt họa mũi tên và hạt năng lượng **CHỈ ĐƯỢC PHÉP CHUYỂN ĐỘNG** trên các luồng đang thực sự hoạt động (`RUNNING` hoặc `ACTIVE`), hoặc khi người dùng chủ động rê chuột (`Hover`) để soi đường dẫn (3 gliding chevrons + 3 neon photon particles lõi trắng `#ffffff` ở 60 FPS).
   - Khi một dự án hoặc tác vụ ở trạng thái `COMPLETED`, `IDLE`, hoặc `STANDBY`, toàn bộ đường nối và node liên quan **phải ở trạng thái tĩnh lặng 100% (Zero Fake Motion)**: 0 mũi tên động, 0 hạt photon (HUD hiển thị đúng 0 hạt), 0 vòng xung nhịp sóng, duy nhất 1 resting chevron tĩnh tại $t=0.5$ (`alpha = 0.85` x `0.45` = `0.3825`).
   - Vòng phát xung sóng năng lượng (`Pulsing Rings`) chỉ phát xung từ các node đang `RUNNING` thực tế.
3. **Mũi Tên Động Thay Thế Hoàn Toàn Mũi Tên Tĩnh (Dynamic Chevrons vs Static Symbols)**:
   - Tắt hoàn toàn đầu mũi tên tam giác tĩnh của ECharts (`edgeSymbol: ['none', 'none']`) để triệt tiêu hiện tượng đầu mũi tên bị đứng im gây hiểu lầm.
   - Toàn bộ mũi tên trên đồ thị là mũi tên chevron di động 60 FPS với nhân phản quang màu trắng tâm điểm (`#ffffff`) và viền neon phát sáng qua lớp GPU-accelerated canvas overlay (`#topo-flow-overlay`).
4. **Cơ Chế Khắc Phục Lỗi "Mũi Tên Đứng Im, Chỉ Sáng Lên Khi Sử Dụng" & Hiện Tượng "Luồng 1 Nơi, Mũi Tên 1 Nơi Khi Zoom"**:
    - **Truy cập đúng Data Model**: Trong ECharts Graph, `edge.data` là `undefined` (phải truy cập qua `series.getOption().links[edge.dataIndex]` hoặc `edge.getModel().option`), nếu truy cập sai sẽ khiến `isRunning` luôn bằng `false` và bỏ qua toàn bộ lệnh vẽ chevrons/particles.
    - **Đồng bộ ma trận tọa độ sống khi Zoom / Pan / Roam**: Hàm `series.coordinateSystem.dataToPoint()` của ECharts chỉ tính toán ma trận ban đầu nhưng không phản ứng kịp thời với các thao tác tương tác roam/zoom động. Để triệt tiêu 100% hiện tượng "luồng một nơi, mũi tên một nơi" khi người dùng cuộn chuột (+Zoom, -Zoom) hoặc kéo di chuyển (Pan), thuật toán phải bóc tách trực tiếp phần tử ZRender đồ họa sống: `edge.getGraphicEl().transformCoordToGlobal(shape.x, shape.y)`. Cơ chế này tự động tích hợp ma trận biến đổi affine tổng hợp của canvas, bảo đảm độ lệch tọa độ giữa đường cong Bezier và mũi tên luôn bằng 0px (`diff = [0, 0]`).
    - **Quy chuẩn thiết kế Mũi tên Thanh mảnh, Khí động học (Slender Needle Chevron)**:
      - Loại bỏ hình dạng mũi tên thô to, bè rộng; áp dụng hình dạng kim vi mạch thanh thoát: đỉnh nhọn vát tới `size`, góc mở cánh hẹp `±size * 0.35` (thay vì `0.55 - 0.6`), điểm thắt đáy `size * 0.45`.
      - Nhân kim quang trắng siêu mảnh tâm điểm (`size * 0.18`) cùng viền neon phát sáng tinh tế (`shadowBlur: 5 - 8px`).
      - Kích thước mũi tên thu nhỏ từ `8.5 - 11px` xuống `5.2 - 6.8px`; bán kính hạt photon thu nhỏ từ `4.8px` xuống `2.0 - 2.8px`.
    - **Quy chuẩn quan sát trung thực**: TokenMonitor là daemon giám sát đang chạy liên tục (`Status: "ACTIVE"`), luôn phát luồng mũi tên chevron và hạt photon 60 FPS cùng sóng xung nhịp tại Orchestrator/Hub. MCREDIT ở trạng thái nghỉ (`STANDBY`/`COMPLETED`), giữ nguyên tĩnh lặng khi ở chế độ xem "Tất Cả", và chỉ kích hoạt luồng di động khi người dùng chọn lọc riêng MCREDIT hoặc rê chuột (`Hover`) để soi đường.
5. **Quy Chuẩn Căn Chỉnh Nhãn Đường Nối (Edge Labels on Curves Alignment & Toggle Modes)**:
   - **Chuẩn hóa thuộc tính `edgeLabel`**: Trong cấu hình ECharts Graph series và từng liên kết trong `links`, bắt buộc sử dụng thuộc tính chuẩn `edgeLabel` (thay vì `label`), thiết lập `position: 'middle'` để ECharts tự động neo nhãn bám sát chính xác vào vị trí trung điểm của đường cong Bezier.
   - **Đồng bộ hóa tuyệt đối với `transformCoordToGlobal` trên Canvas Overlay**: Lớp `#topo-flow-overlay` bóc tách hình học đường nối (`lineShape`), tính toán vector pháp tuyến cong từ `curveness` nếu `cpx1` chưa được khởi tạo, và chuyển đổi tọa độ qua `transformTarget.transformCoordToGlobal(...)`. Tọa độ trung điểm nhãn `(lx, ly)` được tính từ công thức Bezier bậc 2 tại $t=0.5$ (`getTopoBezierPoint(p0, p1, cp, 0.5)`), loại bỏ hoàn toàn độ trôi lệch khi người dùng Zoom / Pan / Roam.
   - **Hai Chế Độ Xem Nhãn Linh Hoạt**:
     * `🏷️ Gọn Gàng` (Smart mode): Ẩn nhãn tĩnh để đồ thị thoáng đãng; khi rê chuột (hover) vào node hoặc đường nối, nhãn số liệu (tokens, calls, loại luồng) lập tức xuất hiện nổi bật dưới dạng Pill Badge viền neon tại đúng trung điểm đường cong.
     * `📑 Hiện Tất Cả` (All mode): Hiển thị nhãn tĩnh bám sát đường cong trên toàn bộ các liên kết, và làm nổi bật (glowing emphasis) khi hover.
6. **Điều Phối Bảng Điều Khiển Theo AI Provider (`syncAgentFleetControlsForProvider`) & Dynamic Type-Hints**:
   - Khi chuyển đổi sang tab OpenAI Codex hoặc Anthropic Claude: Hệ thống tự động ẩn các nút Dual View, Concurrency Only, Lifecycle Gantt (vốn thuộc về dữ liệu Antigravity), và tự động chuyển sang chế độ Topology Graph tương ứng (`/api/openai/graph`, `/api/claude/graph`). Khi quay lại Google Antigravity, toàn bộ các nút điều khiển được khôi phục nguyên trạng.
   - Dynamic Type-Hints tự động cập nhật nội dung tooltip kỹ thuật cho từng thành phần Header (Account Badge, Quota Pill, Reset Expiry, Status, Workspace CWD, Sync button) phù hợp với cơ chế telemetry và thư mục log của từng nhà cung cấp.

7. **Quy Chuẩn Endpoint Tổng Hợp Đa LLM (`GET /api/projects/leaderboard`)**:
   - **Query Parameters**:
     * `range`: `today`, `24h` (hoặc `1d`), `7d`, `30d` (hoặc `month`), `all` (mặc định: `30d`).
     * `sort`: `tokens` (mặc định), `cost`, `activity`.
   - **Zero Mock Data Contract**: 100% tính toán động từ CSDL SQLite (`token_usage_logs`, `agent_fleet_telemetry`), file session Codex (`~/.codex/sessions`), và Claude projects (`~/.claude/projects`). Nếu mốc thời gian không có phát sinh, trả về 0 trung thực.
   - **Cơ cấu DTO**:
     * `kpis`: GrandTotalTokens, GrandTotalCostUSD, GrandTotalSavingsUSD, GrandTotalCalls, GrandTotalTasks, GrandTotalActivity, TotalProjectsCount, ActiveProjectsCount, OverallCacheHitPercent, TopConsumerProject, TopConsumerTokens, TopConsumerPercent, TopActiveProject, TopActiveCount, GoogleTotalTokens/Cost/Percent, OpenAITotalTokens/Cost/Percent, ClaudeTotalTokens/Cost/Percent.
     * `projects[]`: Rank, ProjectID, ProjectName, Workspace, Status (`RUNNING`, `COMPLETED`, `STANDBY`), TotalTokens, PromptTokens, OutputTokens, CachedTokens, ThinkingTokens, EstimatedCostUSD, EstimatedSavingsUSD, TotalCalls, AgentTasks, TotalActivity, CacheHitPercent, TokenSharePercent, CostSharePercent, GoogleBreakdown, OpenAIBreakdown, ClaudeBreakdown.
     * Cho mỗi project: `GoogleBreakdown.Percentage + OpenAIBreakdown.Percentage + ClaudeBreakdown.Percentage == 100.0%` (khi TotalTokens > 0).

8. **Quy Chuẩn Tự Động Focus Dự Án Đang Chạy & Responsive Auto-Fit**:
   - **Auto-Focus (`updateTopologyProjectSelect` & `loadAgentFleetData`)**: Khi chưa có lựa chọn thủ công (`!userHasManuallyChosenTopologyProject`), hệ thống tự động quét danh sách dự án, nhận diện dự án có `status === 'ACTIVE' || status === 'RUNNING'` (ví dụ: `TokenMonitor (GoLangDev)`), tự động chọn option trên dropdown và refetch `/api/agents/graph?project=...` để focus ngay vào cụm dự án đang thực thi. Cờ `userHasManuallyChosenTopologyProject` được bật thành `true` khi người dùng tự tay chọn dropdown và chỉ reset khi chuyển tab Provider.
   - **Responsive Auto-Fit (`calculateTopologyAutoFit`)**: Hàm tính toán bounding box $[minX, maxX, minY, maxY]$ của tập node, căn giữa chính xác tại $[midX, midY] = [\text{round}((minX+maxX)/2), \text{round}((minY+maxY)/2)]$, bổ sung đệm an toàn $padX = 160\text{px}, padY = 150\text{px}$, co giãn tỷ lệ tối ưu theo khung nhìn ($92\%$ width, $84\%$ height), clamp trong khoảng $[0.38, 1.40]$.
   - **Anti-Drift Storage Contract**: Tuyệt đối không lưu tọa độ `center` vào `localStorage` (chỉ lưu `agent_fleet_graph_zoom`) để loại bỏ hoàn toàn nguy cơ lệch tọa độ ra ngoài màn hình khi tải lại trang sau khi pan/roam.

9. **Quy Chuẩn Bố Cục Đồ Thị, Cách Ly Tọa Độ & Lọc Dự Án 1 Giờ (Topology Layouts, Coordinate Decoupling & 1-Hour Active Filter Invariants)**:
   - **3 Chế Độ Bố Cục Chuẩn Hóa**:
     * 📌 **Cố Định (`pinned`)**: Kiến trúc 4 tầng phân cấp kim tự tháp (Level 0 Root 64px, Level 1 Project Hubs 56px, Level 2 Orchestrator 48px, Level 3 Subagents 36-46px) với tọa độ tính toán sẵn từ Golang backend ($canvasCX=1500, stepX=780$). Toàn bộ node mang `fixed: true`.
     * 🧲 **Tự Do (`force`)**: Mô phỏng đàn hồi hạt theo **Quy luật tương tác vật lý động 4 tầng (4-Tier Force Physics Scaling Law)** ($N > 40 \implies \text{repulsion} = 2200, \text{edgeLength} = [180, 350], \text{gravity} = 0.03$; $N > 25 \implies 1800, [150, 300], 0.04$; $N > 12 \implies 1200, [120, 250], 0.06$; $N \le 12 \implies 800, [100, 200], 0.06$). Khởi tạo đối xứng qua `initLayout: 'circular'` và hệ số ma sát `friction: 0.65` nhanh chóng đạt cân bằng tĩnh, triệt tiêu rung lắc kéo dài.
     * ⭕ **Vòng Tròn (`circular`)**: Phân bổ đối xứng vòng tròn đồng tâm, `rotateLabel: true`.
   - **Quy Tắc Cách Ly Tọa Độ Tuyệt Đối (Absolute Coordinate Decoupling Protocol)**:
     * Trong các chế độ động (`force` và `circular`), bắt buộc truyền `x: undefined, y: undefined, fixed: false` cho toàn bộ các node (kể cả Root Controller và Project Hubs). Nghiêm cấm neo tọa độ tĩnh vì sẽ biến node thành mỏ neo lệch tâm kéo văng toàn bộ đồ thị vào góc màn hình ("lào vào góc").
   - **Phân Rã Độc Lập Camera Theo Bố Cục (Layout-Specific Camera Decoupling)**:
     * Chế độ Force / Circular: Bắt buộc camera thiết lập `effectiveCenter: ['50%', '50%']` và `effectiveZoom: 0.85`, tuyệt đối không dùng chung `fitConfig` của Pinned layout.
     * Chế độ Pinned: Sử dụng `fitConfig.center` $[midX, midY]$ và `fitConfig.zoom` (hoặc `savedGraphZoom`).
   - **Quy Chuẩn Lọc Dự Án Hoạt Động Trong Vòng 1 Giờ (1-Hour Active Project Filtering Protocol)**:
     * `oneHourAgo = now.Add(-75 * time.Minute)` (cửa sổ 60 phút kèm 15 phút đệm an toàn).
     * Điều kiện nạp: `p.IsRunning || (!p.LatestActivity.IsZero() && p.LatestActivity.After(oneHourAgo))`.
     * Safe Fallback: Nếu không có dự án nào thỏa mãn trong 75m, giữ lại duy nhất 1 dự án gần nhất (`mostRecentProject`), triệt tiêu 100% nguy cơ màn hình trắng (Zero-Blank Graph Guard).
     * Dropdown Override: Khi người dùng chọn 1 dự án cụ thể từ dropdown, bỏ qua quy tắc 1 giờ và nạp đầy đủ dự án đó.
   - **Triệt Tiêu Nhiễu Thị Giác & Trùng Lặp Nhãn (Zero Visual Redundancy Protocol)**:
     * Loại bỏ hoàn toàn huy hiệu chữ tĩnh `⚡ RUNNING` pill đè dưới chân node. Trạng thái hoạt động được chỉ báo qua vầng sóng xung nhịp radar lan tỏa (`Active Ripple Rings`) và 3 photon chuyển động 60 FPS.
     * Tinh gọn kích thước nốt trong Force mode (Root 44px, Project 34px, Orch 28px, Subagent 22px) và đường nối thanh mảnh (`width: 0.8 - 1.8px`, `curveness: 0.2`, `opacity: 0.4 - 0.8`).

### Documentation Standards Compliance
- Must strictly comply with all 6 standards in `C:\Users\EthanPham\.gemini\config\skills\archify\references\documentation-standards.md`:
  1. Standard 1: Root Placement Contract (`docs/`)
  2. Standard 2: Master Documentation Portal Contract (`docs/index.html` with inlined `DOCS_DATA`)
  3. Standard 3: Wiring & Communication Protocols (Numbered 1-6 arrows with exact file names)
  4. Standard 4: Database ERD Standard (5 tables, CASCADE, 7 indexes, 6 PRAGMAs, DDL)
  5. Standard 5: Security Compliance Contract (`mode=ro`, zero passwords, 100% offline)
  6. Standard 6: Viewer UX Contract (Focus/Trace mode, background reset, Ctrl+F5)

## Code Layout
- `config/`: Configuration structs and loader (`config.go`) - Ground Truth
- `storage/`: Database DDL (5 tables, 7 indexes), connection handling, detector, auto-backup engine, and repository with TTL auto-sweep (`db.go`, `detector.go`, `backup.go`, `repository.go`) - Ground Truth
- `collector/`: Antigravity tailer, OpenAI/Codex read-only session monitor, subagent role classifier, async ring buffer, proxy, and models (`tailer.go`, `codex_monitor.go`, `buffer.go`, `proxy.go`, `models.go`) - Ground Truth
- `web/`: HTTP server (20 `/api/*` endpoints, 24 routes total), embedded static dashboard with 5 views, Topology Network Graph and Type-Hint tooltip engine (`handler.go`, `static/`) - Ground Truth
- `main.go`: Application entrypoint, dependency wiring, background tickers - Ground Truth
- `config.yaml`: Runtime configuration - Ground Truth
- `docs/`: Technical specifications, ERD, architecture diagrams, runbooks, and master portal (`index.html`) - 100% SYNCHRONIZED
