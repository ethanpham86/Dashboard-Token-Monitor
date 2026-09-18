//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const winSvcName = "TokenMonitor"

// windowsService implements svc.Handler so Windows SCM can control us.
type windowsService struct {
	stopCh chan struct{}
}

// Execute is called by the SCM when the service starts.
// It must send cmdsAccepted then block until a Stop/Shutdown command arrives.
func (ws *windowsService) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	status <- svc.Status{State: svc.StartPending}
	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		c := <-req
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			close(ws.stopCh)
			return false, 0
		default:
			log.Printf("[WARN] WindowsService: unexpected control request #%d", c)
		}
	}
}

// isWindowsService returns true when the process is being run by the SCM.
func isWindowsService() bool {
	inService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return inService
}

// runAsWindowsService hands control to the SCM and returns a stopCh that is
// closed when SCM sends Stop/Shutdown. The caller must block on stopCh.
func runAsWindowsService() (stopCh chan struct{}, err error) {
	// Chuyển working directory về thư mục chứa .exe (quan trọng khi SCM khởi động)
	exePath, err := os.Executable()
	if err == nil {
		_ = os.Chdir(filepath.Dir(exePath))
	}

	// Ghi log vào Windows Event Log
	elog, err := eventlog.Open(winSvcName)
	if err != nil {
		// Nếu chưa có event source thì bỏ qua — service vẫn chạy được
		elog = nil
	}
	if elog != nil {
		defer elog.Close()
		_ = elog.Info(1, fmt.Sprintf("%s service starting", winSvcName))
	}

	stopCh = make(chan struct{})
	ws := &windowsService{stopCh: stopCh}

	go func() {
		if err := svc.Run(winSvcName, ws); err != nil {
			log.Printf("[ERROR] WindowsService Run error: %v", err)
		}
	}()

	return stopCh, nil
}

// ── Tiện ích install / uninstall / start / stop ──────────────────────────────

// installService đăng ký binary hiện tại với Windows SCM.
func installService(configPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("kết nối SCM thất bại: %w", err)
	}
	defer m.Disconnect()

	// Kiểm tra nếu service đã tồn tại
	s, err := m.OpenService(winSvcName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %q đã tồn tại — hãy chạy -service uninstall trước", winSvcName)
	}

	// Tạo event log source
	_ = eventlog.InstallAsEventCreate(winSvcName, eventlog.Error|eventlog.Warning|eventlog.Info)

	s, err = m.CreateService(
		winSvcName,
		exePath,
		mgr.Config{
			StartType:   mgr.StartAutomatic,
			DisplayName: "TokenMonitor Daemon",
			Description: "TokenMonitor - AI Agent Observability & FinOps Dashboard (Port 9090)",
		},
		"-config", configPath,
	)
	if err != nil {
		return fmt.Errorf("tạo service thất bại: %w", err)
	}
	defer s.Close()

	log.Printf("[INFO] ✅ Service %q đã được đăng ký thành công", winSvcName)
	log.Printf("[INFO] Chạy lệnh sau để bật service:")
	log.Printf("[INFO]   sc start %s", winSvcName)
	log.Printf("[INFO] Hoặc vào Services → chuột phải %q → Start", winSvcName)
	return nil
}

// uninstallService gỡ bỏ service khỏi Windows SCM.
func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("kết nối SCM thất bại: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(winSvcName)
	if err != nil {
		return fmt.Errorf("không tìm thấy service %q: %w", winSvcName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("xóa service thất bại: %w", err)
	}
	_ = eventlog.Remove(winSvcName)
	log.Printf("[INFO] ✅ Service %q đã được gỡ bỏ hoàn toàn", winSvcName)
	return nil
}
