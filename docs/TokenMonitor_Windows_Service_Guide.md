# TokenMonitor — Hướng Dẫn Cấu Hình Windows Service

> **Phiên bản**: 1.0 — Cập nhật: 2026-09-18
> **Tác giả**: Pham Ethan
> **Mục tiêu**: Cấu hình `token_monitor.exe` chạy nền (Background Service) trên Windows, tự khởi động cùng hệ điều hành, hiển thị trong tab **Services** của Task Manager.

---

## Mục Lục

- [1. Tổng Quan & Yêu Cầu Tiên Quyết](#1-tổng-quan--yêu-cầu-tiên-quyết)
- [2. Phương Án 1: NSSM (Khuyên Dùng)](#2-phương-án-1-nssm-khuyên-dùng)
  - [2.1. Cài đặt NSSM](#21-cài-đặt-nssm)
  - [2.2. Đăng ký Service](#22-đăng-ký-service)
  - [2.3. Khởi động & Kiểm tra](#23-khởi-động--kiểm-tra)
  - [2.4. Các lệnh quản trị hàng ngày](#24-các-lệnh-quản-trị-hàng-ngày)
- [3. Phương Án 2: Windows Task Scheduler (Native)](#3-phương-án-2-windows-task-scheduler-native)
  - [3.1. Cấu hình bằng PowerShell](#31-cấu-hình-bằng-powershell)
  - [3.2. Cấu hình bằng giao diện GUI](#32-cấu-hình-bằng-giao-diện-gui)
- [4. Nghiệm Thu & Xác Nhận](#4-nghiệm-thu--xác-nhận)
- [5. Xử Lý Sự Cố](#5-xử-lý-sự-cố)
- [6. Cập Nhật Phiên Bản Mới](#6-cập-nhật-phiên-bản-mới)

---

## 1. Tổng Quan & Yêu Cầu Tiên Quyết

### Tại sao cần chạy dưới dạng Service?

| Tiêu chí | Chạy thủ công (Terminal) | Chạy dạng Service |
| :--- | :--- | :--- |
| Tự khởi động cùng Windows | ❌ Phải mở terminal mỗi lần | ✅ Tự động 100% |
| Hiển thị trong Task Manager → Services | ❌ Không | ✅ Có (NSSM) |
| Tự restart khi crash | ❌ Phải theo dõi thủ công | ✅ Tự động sau 5 giây |
| Ghi log ra file riêng | ❌ Mất khi đóng terminal | ✅ Ghi vào `logs/` |
| Ẩn cửa sổ đen (Console) | ❌ Luôn hiện | ✅ Chạy hoàn toàn nền |

### Yêu cầu hệ thống

- **OS**: Windows 10 / 11 / Server 2019+
- **Quyền**: Phải chạy PowerShell với **Run as Administrator**
- **File nhị phân**: `token_monitor.exe` đã biên dịch thành công
- **File cấu hình**: `config.yaml` nằm cùng thư mục gốc dự án

### Cấu trúc thư mục quan trọng

```
E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\
├── token_monitor.exe          ← File nhị phân chính
├── config.yaml                ← File cấu hình
├── data\
│   └── token_monitor.db       ← Cơ sở dữ liệu SQLite (WAL mode)
└── logs\                      ← Thư mục log service (tạo mới)
    ├── service.log            ← Log stdout
    └── service_err.log        ← Log stderr
```

> **⚠️ LƯU Ý QUAN TRỌNG**: Không thể dùng `sc.exe create` trực tiếp cho file Go console app. Windows SCM yêu cầu service phải phản hồi bắt tay `SERVICE_RUNNING` — file `token_monitor.exe` là ứng dụng console nên sẽ gặp lỗi **Error 1053** nếu đăng ký trực tiếp. Phải dùng **NSSM** (Service Wrapper) hoặc **Task Scheduler**.

---

## 2. Phương Án 1: NSSM (Khuyên Dùng)

**NSSM** (*Non-Sucking Service Manager*) là công cụ tiêu chuẩn công nghiệp để bọc bất kỳ ứng dụng console nào (Go, Node.js, Python, Java...) thành Windows Service chính thức.

- **Trang chủ**: [nssm.cc](https://nssm.cc)
- **Giấy phép**: Public Domain (Miễn phí hoàn toàn)

### 2.1. Cài đặt NSSM

Mở **PowerShell (Run as Administrator)**, chọn 1 trong 2 cách:

**Cách 1 — Qua Winget (Nhanh nhất):**

```powershell
winget install NSSM.NSSM
```

**Cách 2 — Tải thủ công:**

1. Truy cập: [nssm.cc/download](https://nssm.cc/download)
2. Tải bản mới nhất (file `.zip`)
3. Giải nén, copy file `win64\nssm.exe` vào `C:\Windows\System32\`

**Xác nhận cài đặt:**

```powershell
nssm version
# Kết quả mong đợi: NSSM 2.24-101-g897c7ad ...
```

---

### 2.2. Đăng ký Service

Chạy lần lượt các lệnh sau trong **PowerShell (Administrator)**:

```powershell
# ── Biến cấu hình ──
$WORK_DIR = "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"
$EXE_PATH = "$WORK_DIR\token_monitor.exe"
$LOG_DIR  = "$WORK_DIR\logs"

# Tạo thư mục logs nếu chưa có
if (!(Test-Path $LOG_DIR)) {
    New-Item -ItemType Directory -Path $LOG_DIR -Force
}

# ── 1. Đăng ký Windows Service ──
nssm install TokenMonitor $EXE_PATH

# ── 2. Thư mục làm việc (BẮT BUỘC — để đọc config.yaml và data/) ──
nssm set TokenMonitor AppDirectory $WORK_DIR

# ── 3. Tham số khởi động ──
nssm set TokenMonitor AppParameters "-config config.yaml"

# ── 4. Tên hiển thị và mô tả ──
nssm set TokenMonitor DisplayName "TokenMonitor Daemon"
nssm set TokenMonitor Description "TokenMonitor - AI Agent Observability & FinOps Dashboard (Port 9090)"

# ── 5. Chế độ khởi động tự động cùng Windows ──
nssm set TokenMonitor Start SERVICE_AUTO_START

# ── 6. Chuyển hướng log ra file ──
nssm set TokenMonitor AppStdout "$LOG_DIR\service.log"
nssm set TokenMonitor AppStderr "$LOG_DIR\service_err.log"

# ── 7. Tự động xoay vòng log khi đạt 10MB ──
nssm set TokenMonitor AppRotateFiles 1
nssm set TokenMonitor AppRotateBytes 10485760

# ── 8. Tự restart sau 5 giây nếu app bị tắt đột ngột ──
nssm set TokenMonitor AppRestartDelay 5000
```

> **💡 MẸO**: Nếu muốn dùng giao diện đồ họa (GUI) thay vì dòng lệnh:
> ```powershell
> nssm edit TokenMonitor
> ```
> Cửa sổ GUI sẽ mở ra với đầy đủ các tab: *Application*, *Details*, *Log on*, *I/O*, v.v.

---

### 2.3. Khởi động & Kiểm tra

```powershell
# Tắt process cũ nếu đang chạy thủ công
Get-Process -Name token_monitor -ErrorAction SilentlyContinue | Stop-Process -Force

# Khởi động service
nssm start TokenMonitor

# Kiểm tra trạng thái
nssm status TokenMonitor
# ✅ Kết quả mong đợi: SERVICE_RUNNING
```

Sau khi chạy thành công, mở **Task Manager → tab Services** sẽ thấy dòng:

| Name | PID | Description |
| :--- | :--- | :--- |
| **TokenMonitor** | *<số PID>* | TokenMonitor - AI Agent Observability & FinOps Dashboard (Port 9090) |

---

### 2.4. Các lệnh quản trị hàng ngày

| Thao tác | Lệnh PowerShell (Administrator) |
| :--- | :--- |
| **Khởi động** | `nssm start TokenMonitor` |
| **Tạm dừng** | `nssm stop TokenMonitor` |
| **Khởi động lại** | `nssm restart TokenMonitor` |
| **Xem trạng thái** | `nssm status TokenMonitor` |
| **Chỉnh sửa cấu hình (GUI)** | `nssm edit TokenMonitor` |
| **Xem log stdout** | `Get-Content "$WORK_DIR\logs\service.log" -Tail 50` |
| **Xem log stderr** | `Get-Content "$WORK_DIR\logs\service_err.log" -Tail 50` |
| **Theo dõi log realtime** | `Get-Content "$WORK_DIR\logs\service.log" -Wait -Tail 20` |
| **Gỡ bỏ hoàn toàn service** | `nssm remove TokenMonitor confirm` |

> **⚠️ Lưu ý**: Anh cũng có thể Start/Stop/Restart trực tiếp bằng **chuột phải** lên dòng `TokenMonitor` trong tab Services của Task Manager.

---

## 3. Phương Án 2: Windows Task Scheduler (Native)

Nếu không muốn cài thêm NSSM, có thể dùng **Task Scheduler** có sẵn trong Windows. Hạn chế duy nhất: task **không hiển thị** trong tab Services của Task Manager, nhưng vẫn chạy nền 100% và tự khởi động cùng Windows.

### 3.1. Cấu hình bằng PowerShell

Mở **PowerShell (Run as Administrator)** và dán đoạn lệnh sau:

```powershell
$action = New-ScheduledTaskAction `
    -Execute "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\token_monitor.exe" `
    -Argument "-config config.yaml" `
    -WorkingDirectory "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"

# Kích hoạt khi đăng nhập máy tính
$trigger = New-ScheduledTaskTrigger -AtLogOn

# Thiết lập: không giới hạn thời gian chạy, tự restart nếu lỗi
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit 0 `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1)

# Đăng ký tác vụ
Register-ScheduledTask `
    -TaskName "TokenMonitorDaemon" `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -RunLevel Highest `
    -Description "TokenMonitor Background Daemon on Port 9090"

# Kích hoạt chạy ngay lập tức
Start-ScheduledTask -TaskName "TokenMonitorDaemon"
```

### 3.2. Cấu hình bằng giao diện GUI

1. Nhấn `Win + R`, gõ `taskschd.msc` và Enter
2. Cột bên phải, bấm **Create Task...**
3. **Tab General**:
   - Name: `TokenMonitorDaemon`
   - Description: `TokenMonitor Background Daemon on Port 9090`
   - Tích ☑ **Run with highest privileges**
   - Tích ☑ **Run whether user is logged on or not** *(để ẩn cửa sổ đen)*
4. **Tab Triggers**:
   - Bấm **New...** → Begin the task: **At log on** → OK
5. **Tab Actions**:
   - Bấm **New...**
   - Program/script: `E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\token_monitor.exe`
   - Add arguments: `-config config.yaml`
   - Start in: `E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor`
   - Bấm OK
6. **Tab Settings**:
   - **Bỏ tích** dòng *"Stop the task if it runs longer than..."*
   - Tích ☑ *"If the running task does not end when requested, force it to stop"*
   - Mục *"If the task fails, restart every"*: chọn **1 minute**, tối đa **3 lần**
7. Bấm **OK** để lưu

### Quản trị Task Scheduler

| Thao tác | Lệnh PowerShell |
| :--- | :--- |
| **Kích hoạt** | `Start-ScheduledTask -TaskName "TokenMonitorDaemon"` |
| **Tạm dừng** | `Stop-ScheduledTask -TaskName "TokenMonitorDaemon"` |
| **Xem trạng thái** | `Get-ScheduledTask -TaskName "TokenMonitorDaemon" \| Select-Object State` |
| **Gỡ bỏ** | `Unregister-ScheduledTask -TaskName "TokenMonitorDaemon" -Confirm:$false` |

---

## 4. Nghiệm Thu & Xác Nhận

Sau khi cấu hình xong (bằng NSSM hoặc Task Scheduler):

### Bước 1: Kiểm tra service đang chạy

```powershell
# Kiểm tra port 9090 đang lắng nghe
Get-NetTCPConnection -LocalPort 9090 -ErrorAction SilentlyContinue | Select-Object LocalPort, State, OwningProcess
```

### Bước 2: Truy cập Dashboard

Mở trình duyệt → **http://localhost:9090**
- Giao diện Dashboard & Topology Graph hiển thị bình thường = ✅ Thành công

### Bước 3: Kiểm tra sau Restart Windows

1. Khởi động lại máy tính (Restart Windows)
2. **Không mở** bất kỳ terminal/PowerShell nào
3. Mở trình duyệt → **http://localhost:9090**
4. Dashboard tải tức thì và dữ liệu token vẫn được ghi nhận = ✅ Service hoạt động hoàn hảo

### Bước 4: Xác nhận trong Task Manager (chỉ NSSM)

Mở **Task Manager → tab Services** → Tìm dòng **TokenMonitor** → Trạng thái **Running**

---

## 5. Xử Lý Sự Cố

### Lỗi 1: Port 9090 bị chiếm bởi process khác

```powershell
# Tìm process đang chiếm port 9090
Get-NetTCPConnection -LocalPort 9090 | Select-Object OwningProcess
Get-Process -Id <PID> | Select-Object ProcessName, Path

# Tắt process đó
Stop-Process -Id <PID> -Force

# Khởi động lại service
nssm restart TokenMonitor
```

### Lỗi 2: Service không khởi động — kiểm tra log

```powershell
# Đọc log lỗi
Get-Content "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\logs\service_err.log" -Tail 30
```

### Lỗi 3: NSSM báo "Service already exists"

```powershell
# Gỡ bỏ service cũ trước
nssm remove TokenMonitor confirm

# Đăng ký lại
nssm install TokenMonitor "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor\token_monitor.exe"
# ... (chạy lại các lệnh nssm set ở mục 2.2)
```

### Lỗi 4: Task Scheduler — task chạy rồi tắt ngay

Nguyên nhân thường gặp: thiếu trường **Start in** (Working Directory).
- Mở Task Scheduler → tìm task `TokenMonitorDaemon` → Properties → tab Actions → Edit
- Điền đúng ô **Start in**: `E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor`

---

## 6. Cập Nhật Phiên Bản Mới

Khi biên dịch lại `token_monitor.exe` (ví dụ sau khi sửa code và `go build`):

```powershell
# 1. Dừng service
nssm stop TokenMonitor

# 2. Biên dịch lại (trong thư mục dự án)
Set-Location "E:\GoogleDrive\WorkSpace\Code\ProjectGolang\GoLangDev\TokenMonitor"
go build -o token_monitor.exe .

# 3. Khởi động lại service
nssm start TokenMonitor

# 4. Xác nhận
nssm status TokenMonitor
# ✅ SERVICE_RUNNING
```

> **💡 MẸO**: Không cần đăng ký lại service (không cần `nssm install` lại). Chỉ cần Stop → Build → Start là đủ vì NSSM trỏ trực tiếp đến file `token_monitor.exe` tại vị trí cố định.

---

## So Sánh Hai Phương Án

| Tiêu chí | NSSM (Phương án 1) | Task Scheduler (Phương án 2) |
| :--- | :--- | :--- |
| Hiển thị trong Task Manager → Services | ✅ **Có** | ❌ Không |
| Cần cài thêm tool | ✅ Cần cài NSSM (~300KB) | ❌ 100% Native |
| Tự khởi động cùng Windows | ✅ | ✅ |
| Tự restart khi crash | ✅ (sau 5 giây) | ✅ (sau 1 phút, tối đa 3 lần) |
| Ghi log ra file riêng | ✅ Tự động | ❌ Phải tự cấu hình thêm |
| Xoay vòng log (Log rotation) | ✅ Tự động (10MB) | ❌ Không có |
| Start/Stop bằng chuột (Task Manager) | ✅ | ❌ Phải vào Task Scheduler |
| Độ chuyên nghiệp | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |

> **Khuyến nghị**: Dùng **NSSM (Phương án 1)** để có trải nghiệm quản trị chuyên nghiệp nhất, hiển thị đúng trong tab Services của Task Manager như yêu cầu.
