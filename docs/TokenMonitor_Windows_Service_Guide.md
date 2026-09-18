# TokenMonitor — Hướng Dẫn Chạy Dưới Dạng Windows Service

> **Phiên bản**: 2.0 — Cập nhật: 2026-09-18
> **Tác giả**: Pham Ethan
> **Mục tiêu**: Chạy `token_monitor.exe` như một Windows Service chính thức — tự động bật cùng máy tính, ẩn hoàn toàn cửa sổ đen (console), hiển thị trong tab **Services** của Task Manager, không cần cài thêm bất kỳ phần mềm nào.

---

## Mục Lục

- [1. Tổng Quan](#1-tổng-quan)
- [2. Phương Án 1: Native Windows Service (Khuyên Dùng — Không Cần Cài Gì)](#2-phương-án-1-native-windows-service-khuyên-dùng--không-cần-cài-gì)
  - [2.1. Nguyên lý hoạt động](#21-nguyên-lý-hoạt-động)
  - [2.2. Cài đặt & Đăng ký Service](#22-cài-đặt--đăng-ký-service)
  - [2.3. Quản lý Service hàng ngày](#23-quản-lý-service-hàng-ngày)
  - [2.4. Cập nhật phiên bản mới](#24-cập-nhật-phiên-bản-mới)
  - [2.5. Gỡ bỏ Service](#25-gỡ-bỏ-service)
- [3. Phương Án 2: NSSM (Service Wrapper)](#3-phương-án-2-nssm-service-wrapper)
- [4. Phương Án 3: Windows Task Scheduler (100% Native, không vào tab Services)](#4-phương-án-3-windows-task-scheduler-100-native-không-vào-tab-services)
- [5. Nghiệm Thu & Xác Nhận](#5-nghiệm-thu--xác-nhận)
- [6. Xử Lý Sự Cố](#6-xử-lý-sự-cố)
- [7. So Sánh Ba Phương Án](#7-so-sánh-ba-phương-án)

---

## 1. Tổng Quan

### Tại sao cần chạy dưới dạng Service?

| Tiêu chí | Chạy thủ công (double-click / terminal) | Chạy dạng Service |
|:---|:---|:---|
| Tự bật cùng Windows | ❌ Phải mở tay mỗi lần | ✅ Tự động 100% |
| Ẩn cửa sổ đen (console) | ❌ Luôn hiện cửa sổ đen | ✅ Chạy hoàn toàn nền |
| Hiển thị trong tab Services | ❌ Không | ✅ Có |
| Tự restart khi crash | ❌ Phải theo dõi thủ công | ✅ Tự động |
| Graceful shutdown an toàn | ✅ Ctrl+C | ✅ Stop từ Task Manager |

### Yêu cầu

- **OS**: Windows 10 / 11 / Windows Server 2019+
- **Quyền**: Phải mở PowerShell bằng **Run as Administrator**
- **File nhị phân**: `token_monitor.exe` đã biên dịch thành công
- **File cấu hình**: `config.yaml` nằm cùng thư mục gốc dự án

### Cấu trúc thư mục

```
E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\
├── token_monitor.exe       ← File nhị phân chính (đã tích hợp Windows Service)
├── config.yaml             ← File cấu hình
├── data\
│   └── token_monitor.db    ← SQLite database (WAL mode)
└── logs\                   ← (Tùy chọn) Thư mục log
```

---

## 2. Phương Án 1: Native Windows Service (Khuyên Dùng — Không Cần Cài Gì)

Đây là phương án tốt nhất. `token_monitor.exe` đã được tích hợp sẵn giao thức giao tiếp Windows SCM (*Service Control Manager*) thông qua thư viện `golang.org/x/sys/windows/svc`. Không cần cài thêm bất kỳ phần mềm nào.

### 2.1. Nguyên lý hoạt động

```
token_monitor.exe -service install
       │
       ▼
Windows SCM ◄──────────────────────────────────────────────┐
(Services)                                                  │
       │ sc start TokenMonitor                              │
       ▼                                                    │
token_monitor.exe ←──── bắt tay SERVICE_RUNNING ────────────┘
   (chạy nền)
       │
       ├── LocalTailer (quét transcript Antigravity)
       ├── CodexMonitor (quét session OpenAI)
       ├── ClaudeMonitor (quét session Claude)
       └── Web Dashboard :9090

Task Manager → tab Services → TokenMonitor [Running]
```

Khi SCM gửi lệnh Stop (từ chuột phải trong Task Manager hoặc `sc stop`), `token_monitor.exe` thực hiện **Graceful Shutdown** — lưu dữ liệu, đóng database an toàn, backup cuối cùng — trước khi thoát.

### 2.2. Cài đặt & Đăng ký Service

> ⚠️ **Phải mở PowerShell bằng Run as Administrator**

```powershell
# Bước 1: Vào thư mục dự án
cd "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"

# Bước 2: Đăng ký TokenMonitor thành Windows Service (chỉ làm 1 lần duy nhất)
.\token_monitor.exe -service install
```

Kết quả thành công:
```
[INFO] ✅ Service "TokenMonitor" đã được đăng ký thành công
[INFO] Chạy lệnh sau để bật service:
[INFO]   sc start TokenMonitor
```

```powershell
# Bước 3: Khởi động service
sc.exe start TokenMonitor

# Bước 4: Xác nhận đang chạy
sc.exe query TokenMonitor
```

Kết quả mong đợi:
```
SERVICE_NAME: TokenMonitor
        TYPE               : 10  WIN32_OWN_PROCESS
        STATE              : 4  RUNNING
        WIN32_EXIT_CODE    : 0
        WAIT_HINT          : 0x0
```

**Kiểm tra trực quan**: Mở Task Manager → tab **Services** → thấy dòng `TokenMonitor` với trạng thái **Running**. Không có cửa sổ đen nào xuất hiện.

#### Cấu hình Service trong Registry (tự động — không cần làm thêm gì)

Lệnh `-service install` tự động ghi vào Registry:

| Tham số | Giá trị |
|:---|:---|
| **Tên service** | `TokenMonitor` |
| **Tên hiển thị** | `TokenMonitor Daemon` |
| **Mô tả** | `TokenMonitor - AI Agent Observability & FinOps Dashboard (Port 9090)` |
| **Kiểu khởi động** | `Automatic` (tự bật cùng Windows) |
| **File thực thi** | Đường dẫn tuyệt đối đến `token_monitor.exe` |
| **Tham số** | `-config <đường dẫn tuyệt đối đến config.yaml>` |

### 2.3. Quản lý Service hàng ngày

#### Bằng lệnh PowerShell (không cần Administrator)

| Thao tác | Lệnh |
|:---|:---|
| **Khởi động** | `sc.exe start TokenMonitor` |
| **Tạm dừng** | `sc.exe stop TokenMonitor` |
| **Kiểm tra trạng thái** | `sc.exe query TokenMonitor` |
| **Xem chi tiết cấu hình** | `sc.exe qc TokenMonitor` |

#### Bằng chuột (Task Manager)

1. Mở **Task Manager** → tab **Services**
2. Tìm dòng `TokenMonitor`
3. **Chuột phải** → chọn **Start** / **Stop** / **Restart**

#### Bằng Services Panel (services.msc)

```powershell
# Mở cửa sổ Services
services.msc
```

Tìm `TokenMonitor Daemon` trong danh sách → có thể cấu hình thêm Recovery (tự restart khi crash), Startup type, v.v.

### 2.4. Cập nhật phiên bản mới

Khi biên dịch lại `token_monitor.exe` sau khi sửa code:

```powershell
# Bước 1: Dừng service
sc.exe stop TokenMonitor

# Bước 2: Biên dịch lại
cd "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"
go build -o token_monitor.exe .

# Bước 3: Khởi động lại service
sc.exe start TokenMonitor

# Bước 4: Xác nhận
sc.exe query TokenMonitor
```

> ✅ **Không cần** `install` lại service. NSSM trỏ thẳng đến file `.exe` — chỉ cần Stop → Build → Start.

### 2.5. Gỡ bỏ Service

```powershell
# Bước 1: Dừng service (nếu đang chạy)
sc.exe stop TokenMonitor

# Bước 2: Gỡ bỏ hoàn toàn
.\token_monitor.exe -service uninstall
```

Kết quả:
```
[INFO] ✅ Service "TokenMonitor" đã được gỡ bỏ hoàn toàn
```

---

## 3. Phương Án 2: NSSM (Service Wrapper)

Dùng khi không thể biên dịch lại binary (ví dụ chỉ có file `.exe` cũ chưa tích hợp `-service install`).

> **NSSM** (*Non-Sucking Service Manager*) — [nssm.cc](https://nssm.cc) — bọc bất kỳ console app nào thành Windows Service.

### Cài đặt NSSM

```powershell
winget install NSSM.NSSM
```

### Đăng ký Service

Mở **PowerShell (Administrator)**:

```powershell
$D = "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"
New-Item -ItemType Directory -Path "$D\logs" -Force

nssm install TokenMonitor "$D\token_monitor.exe"
nssm set TokenMonitor AppDirectory        $D
nssm set TokenMonitor AppParameters       "-config config.yaml"
nssm set TokenMonitor DisplayName         "TokenMonitor Daemon"
nssm set TokenMonitor Description         "TokenMonitor - AI Agent Observability & FinOps Dashboard (Port 9090)"
nssm set TokenMonitor Start               SERVICE_AUTO_START
nssm set TokenMonitor AppStdout           "$D\logs\service.log"
nssm set TokenMonitor AppStderr           "$D\logs\service_err.log"
nssm set TokenMonitor AppRotateFiles      1
nssm set TokenMonitor AppRotateBytes      10485760
nssm set TokenMonitor AppRestartDelay     5000

nssm start TokenMonitor
nssm status TokenMonitor   # → SERVICE_RUNNING
```

### Lệnh quản trị NSSM

| Thao tác | Lệnh |
|:---|:---|
| Khởi động | `nssm start TokenMonitor` |
| Dừng | `nssm stop TokenMonitor` |
| Khởi động lại | `nssm restart TokenMonitor` |
| Trạng thái | `nssm status TokenMonitor` |
| Chỉnh sửa GUI | `nssm edit TokenMonitor` |
| Xem log realtime | `Get-Content "$D\logs\service.log" -Wait -Tail 20` |
| Gỡ bỏ hoàn toàn | `nssm remove TokenMonitor confirm` |

---

## 4. Phương Án 3: Windows Task Scheduler (100% Native, không vào tab Services)

Dùng khi không muốn cài bất kỳ gì và không cần hiển thị trong tab Services.

> ⚠️ **Hạn chế**: Task Scheduler **không hiển thị** trong tab Services của Task Manager.

### Cấu hình bằng PowerShell (Administrator)

```powershell
$D = "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"

$action = New-ScheduledTaskAction `
    -Execute "$D\token_monitor.exe" `
    -Argument "-config config.yaml" `
    -WorkingDirectory $D

$trigger = New-ScheduledTaskTrigger -AtLogOn

$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit 0 `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask `
    -TaskName "TokenMonitorDaemon" `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -RunLevel Highest `
    -Description "TokenMonitor Background Daemon on Port 9090"

Start-ScheduledTask -TaskName "TokenMonitorDaemon"
```

### Lệnh quản trị Task Scheduler

| Thao tác | Lệnh |
|:---|:---|
| Khởi động | `Start-ScheduledTask -TaskName "TokenMonitorDaemon"` |
| Dừng | `Stop-ScheduledTask -TaskName "TokenMonitorDaemon"` |
| Trạng thái | `Get-ScheduledTask -TaskName "TokenMonitorDaemon" \| Select State` |
| Gỡ bỏ | `Unregister-ScheduledTask -TaskName "TokenMonitorDaemon" -Confirm:$false` |

---

## 5. Nghiệm Thu & Xác Nhận

### Bước 1 — Kiểm tra port 9090 đang lắng nghe

```powershell
Get-NetTCPConnection -LocalPort 9090 | Select-Object LocalPort, State, OwningProcess
# ✅ State: Listen
```

### Bước 2 — Truy cập Dashboard

Mở trình duyệt → **http://localhost:9090**
- Dashboard và Topology Graph hiển thị bình thường = ✅ Thành công

### Bước 3 — Kiểm tra sau Restart Windows

1. Khởi động lại máy tính
2. **Không mở** bất kỳ terminal nào
3. Mở trình duyệt → **http://localhost:9090**
4. Dashboard tải và dữ liệu token vẫn đang được thu thập = ✅ Service hoạt động hoàn hảo

### Bước 4 — Xem trong Task Manager

Mở **Task Manager → tab Services** → Tìm dòng **`TokenMonitor`**:

| Name | PID | Description | Status |
|:---|:---|:---|:---|
| `TokenMonitor` | *số PID* | TokenMonitor - AI Agent Observability & FinOps Dashboard (Port 9090) | **Running** |

---

## 6. Xử Lý Sự Cố

### Lỗi: "Access Denied" khi install/uninstall

```
Nguyên nhân: Chưa chạy PowerShell với quyền Administrator
Giải pháp:   Nhấn phải vào PowerShell → "Run as administrator"
```

### Lỗi: Port 9090 đã bị chiếm

```powershell
# Tìm process đang chiếm port
Get-NetTCPConnection -LocalPort 9090 | Select-Object OwningProcess
Get-Process -Id <PID_ở_trên> | Select-Object Id, ProcessName, Path

# Nếu là tiến trình token_monitor.exe cũ đang chạy thủ công
Stop-Process -Id <PID> -Force

# Bật lại service
sc.exe start TokenMonitor
```

### Lỗi: Service dừng ngay sau khi Start

```powershell
# Kiểm tra Windows Event Log
Get-EventLog -LogName Application -Source TokenMonitor -Newest 10 |
    Select-Object TimeGenerated, Message | Format-List
```

```powershell
# Hoặc thử chạy thủ công để xem lỗi trực tiếp
cd "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"
.\token_monitor.exe -config config.yaml
# Xem thông báo lỗi hiển thị trong cửa sổ đen
```

### Lỗi: "Service already exists" khi install

```powershell
# Gỡ service cũ trước
sc.exe stop TokenMonitor
.\token_monitor.exe -service uninstall

# Đăng ký lại
.\token_monitor.exe -service install
sc.exe start TokenMonitor
```

### Lỗi: config.yaml không tìm thấy khi chạy Service

```
Nguyên nhân: SCM khởi động từ thư mục hệ thống (C:\Windows\System32),
             không phải thư mục dự án. Hàm installService() trong winsvc.go
             đã xử lý bằng cách ghi đường dẫn tuyệt đối vào Registry.
Kiểm tra:    sc.exe qc TokenMonitor
             → BINARY_PATH_NAME phải chứa -config <đường dẫn tuyệt đối>
```

---

## 7. So Sánh Ba Phương Án

| Tiêu chí | ⭐ Phương án 1<br>Native Go Service | Phương án 2<br>NSSM | Phương án 3<br>Task Scheduler |
|:---|:---:|:---:|:---:|
| Cần cài thêm phần mềm | ❌ Không | ✅ Cần NSSM | ❌ Không |
| Hiển thị trong tab Services | ✅ | ✅ | ❌ |
| Tự khởi động cùng Windows | ✅ | ✅ | ✅ |
| Ẩn cửa sổ đen hoàn toàn | ✅ | ✅ | ✅ |
| Graceful Shutdown (lưu DB) | ✅ | ✅ | ⚠️ (kill ngay) |
| Ghi log ra file riêng | ⚠️ Windows Event Log | ✅ file .log | ❌ |
| Tự restart khi crash | ⚠️ Cần cấu hình thêm | ✅ AppRestartDelay | ✅ RestartCount |
| Start/Stop bằng chuột | ✅ Task Manager | ✅ Task Manager | ❌ Task Scheduler |
| Độ chuyên nghiệp | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |

> **Khuyến nghị**: Dùng **Phương án 1** (Native Go Service) vì không cần cài gì thêm, tích hợp trực tiếp vào binary, Graceful Shutdown đúng chuẩn, và hiển thị đầy đủ trong tab Services như một service hệ thống Windows thực thụ.

---

## Phụ Lục: Kiến Trúc Kỹ Thuật Phương Án 1

### Các file liên quan

| File | Vai trò |
|:---|:---|
| [`winsvc.go`](../winsvc.go) | Windows Service handler — chỉ compile trên Windows (`//go:build windows`). Implements `svc.Handler`, xử lý bắt tay SCM, nhận lệnh Stop/Shutdown |
| [`main.go`](../main.go) | Entry point — nhận flag `-service install/uninstall`, phát hiện tự động khi đang chạy dưới SCM qua `isWindowsService()` |

### Luồng khởi động khi chạy dưới SCM

```
Windows Boot
    → SCM đọc Registry → tìm "TokenMonitor"
    → Chạy: token_monitor.exe -config C:\...\config.yaml
    → main() → isWindowsService() = true
    → runAsWindowsService() → svc.Run("TokenMonitor", handler)
    → handler.Execute() → gửi SERVICE_RUNNING về SCM
    → LocalTailer, CodexMonitor, ClaudeMonitor, Web Dashboard :9090 khởi động
    → Chờ lệnh Stop từ SCM (hoặc SIGTERM)
    → Graceful Shutdown: lưu DB, backup, đóng HTTP server
    → Báo SERVICE_STOPPED về SCM → thoát
```

### Thư viện sử dụng

```
golang.org/x/sys/windows/svc        — Windows Service lifecycle (Start/Stop/Shutdown)
golang.org/x/sys/windows/svc/mgr    — Kết nối và thao tác với Windows SCM
golang.org/x/sys/windows/svc/eventlog — Ghi log vào Windows Event Viewer
```

Thư viện này đã có sẵn trong `go.mod` của dự án (`golang.org/x/sys v0.47.0`) — không cần `go get` thêm.
