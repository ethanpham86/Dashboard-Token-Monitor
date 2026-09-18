package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tokenmonitor/collector"
	"tokenmonitor/config"
	"tokenmonitor/storage"
	"tokenmonitor/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Đường dẫn tới file cấu hình YAML")
	// -service install   → đăng ký Windows Service (chạy với quyền Admin)
	// -service uninstall → gỡ bỏ Windows Service
	// (bỏ trống)         → chạy bình thường như console app
	serviceCmd := flag.String("service", "", "Lệnh quản lý Windows Service: install | uninstall")
	flag.Parse()

	// ── Xử lý lệnh install / uninstall ──────────────────────────────────────
	switch *serviceCmd {
	case "install":
		// Cần đường dẫn tuyệt đối để SCM khởi động đúng config
		absConfig := *configPath
		if !strings.HasPrefix(absConfig, string(os.PathSeparator)) && len(absConfig) > 1 && absConfig[1] != ':' {
			if exe, err := os.Executable(); err == nil {
				absConfig = strings.TrimSuffix(exe, "token_monitor.exe") + *configPath
			}
		}
		if err := installService(absConfig); err != nil {
			log.Fatalf("[FATAL] Không thể cài đặt service: %v", err)
		}
		return
	case "uninstall":
		if err := uninstallService(); err != nil {
			log.Fatalf("[FATAL] Không thể gỡ service: %v", err)
		}
		return
	}


	log.Println("==================================================================")
	log.Println("  🚀 KHỞI ĐỘNG TOKENMONITOR - OBSERVABILITY & FINOPS DAEMON")
	log.Println("==================================================================")

	// 1. Nạp file cấu hình
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[FATAL] Không thể nạp cấu hình: %v", err)
	}
	log.Printf("[INFO] Đã nạp cấu hình từ %s", *configPath)

	// 2. Khởi tạo Storage (SQLite)
	store, err := storage.NewStorage(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Không thể khởi tạo SQLite storage: %v", err)
	}
	defer store.Close()
	log.Printf("[INFO] SQLite Storage đã sẵn sàng tại: %s (WAL Mode)", cfg.Database.SQLitePath)

	log.Println("[INFO] Chế độ giám sát dữ liệu thực tế: 100% dữ liệu được thu thập từ Antigravity transcripts (Zero Hard-code).")

	// 3. Khởi tạo Ring Buffer không khóa
	buf := collector.NewAsyncBuffer(store, 1000, 100, 1*time.Second)
	buf.Start()
	defer buf.Stop()
	log.Println("[INFO] In-Memory Ring Buffer đã khởi chạy.")

	// 3b. Khởi chạy định kỳ Rollup 5 phút (Station 4)
	rollupInterval := time.Duration(cfg.Database.RollupIntervalSeconds) * time.Second
	if rollupInterval <= 0 {
		rollupInterval = 5 * time.Minute
	}
	rollupTicker := time.NewTicker(rollupInterval)
	rollupDone := make(chan struct{})
	go func() {
		// Chạy 1 lần ngay sau khi khởi động
		if err := store.RollupHourlyMetrics(); err != nil {
			log.Printf("[WARN] Khởi tạo RollupHourlyMetrics thất bại: %v", err)
		} else {
			log.Println("[INFO] Đã thực hiện tổng hợp Rollup ban đầu thành công.")
		}
		for {
			select {
			case <-rollupDone:
				return
			case <-rollupTicker.C:
				if err := store.RollupHourlyMetrics(); err != nil {
					log.Printf("[ERROR] Lỗi thực hiện định kỳ Rollup 5 phút: %v", err)
				}
			}
		}
	}()

	// 3c. Khởi chạy định kỳ Auto-Backup cho SQLite (WAL mode snapshot)
	var backupTicker *time.Ticker
	backupDone := make(chan struct{})
	if cfg.Backup.Enabled {
		backupInterval := time.Duration(cfg.Backup.IntervalMinutes) * time.Minute
		if backupInterval <= 0 {
			backupInterval = 60 * time.Minute
		}
		backupTicker = time.NewTicker(backupInterval)
		log.Printf("[INFO] 💾 Auto-Backup định kỳ đã kích hoạt: Chu kỳ %v, Thư mục: %s (Giữ tối đa: %d bản)",
			backupInterval, cfg.Backup.BackupDir, cfg.Backup.MaxKeep)

		go func() {
			// Thực hiện 1 lần sau khi khởi động 5 giây
			select {
			case <-backupDone:
				return
			case <-time.After(5 * time.Second):
				if _, err := store.PerformAutoBackup(); err != nil {
					log.Printf("[WARN] Khởi tạo Auto-Backup ban đầu thất bại: %v", err)
				}
			}

			for {
				select {
				case <-backupDone:
					return
				case <-backupTicker.C:
					if _, err := store.PerformAutoBackup(); err != nil {
						log.Printf("[ERROR] Lỗi thực hiện định kỳ Auto-Backup: %v", err)
					}
				}
			}
		}()
	}

	// 4. Khởi chạy Reverse Proxy (nếu bật)
	var proxyServer *http.Server
	if cfg.Proxy.Enabled {
		proxyHandler, err := collector.NewProxyServer(cfg, buf)
		if err != nil {
			log.Fatalf("[FATAL] Lỗi khởi tạo Proxy: %v", err)
		}

		proxyAddr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Proxy.ListenPort)
		proxyServer = &http.Server{
			Addr:         proxyAddr,
			Handler:      proxyHandler,
			ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
			WriteTimeout: time.Duration(cfg.Proxy.UpstreamTimeoutSeconds) * time.Second,
		}

		go func() {
			log.Printf("[INFO] ⚡ Proxy Interceptor đang lắng nghe tại: http://%s -> Upstream: %s", proxyAddr, cfg.Proxy.UpstreamTarget)
			if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[ERROR] Proxy Server gặp lỗi: %v", err)
			}
		}()
	}

	// 5. Khởi chạy Local Log Tailer (Quét trực tiếp phiên chat Antigravity IDE)
	var tailer *collector.LocalTailer
	if cfg.LocalTailer.Enabled {
		tailer = collector.NewLocalTailer(cfg, buf)
		tailer.Start()
		defer tailer.Stop()
	}

	// 5b. Theo dõi OpenAI/Codex bằng log session cục bộ, không đọc credential.
	var codexMonitor *collector.CodexMonitor
	if cfg.OpenAIMonitor.Enabled {
		codexMonitor = collector.NewCodexMonitor(cfg.OpenAIMonitor)
		codexMonitor.Start()
		defer codexMonitor.Stop()
	}

	// 5c. Theo dõi Anthropic Claude Code bằng log session cục bộ, không đọc .credentials.json.
	var claudeMonitor *collector.ClaudeMonitor
	if cfg.ClaudeMonitor.Enabled {
		claudeMonitor = collector.NewClaudeMonitor(cfg.ClaudeMonitor)
		claudeMonitor.Start()
		defer claudeMonitor.Stop()
	}

	// 6. Khởi chạy Web Dashboard & API Server
	webApp := web.NewServer(store, buf, tailer, codexMonitor)
	if claudeMonitor != nil {
		webApp.SetClaudeMonitor(claudeMonitor)
	}
	dashAddr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.DashboardPort)
	dashServer := &http.Server{
		Addr:         dashAddr,
		Handler:      webApp.Routes(),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
	}

	go func() {
		log.Printf("[INFO] 🌐 Dashboard Web UI đang mở tại: http://%s", dashAddr)
		if err := dashServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if strings.Contains(err.Error(), "bind") {
				log.Fatalf("[FATAL] Cổng %s đã bị chiếm dụng! Bạn có thể đang bật sẵn 1 tiến trình TokenMonitor khác trên máy.", dashAddr)
			}
			log.Fatalf("[FATAL] Dashboard Web Server gặp lỗi: %v", err)
		}
	}()

	// 7. Lắng nghe tín hiệu dừng — hỗ trợ cả Windows Service (SCM) và console (Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Khi chạy dưới Windows SCM, nhận stop signal từ SCM thay vì OS signal
	if isWindowsService() {
		log.Println("[INFO] 🪟 Chạy dưới dạng Windows Service — chờ lệnh Stop từ SCM...")
		svcStopCh, err := runAsWindowsService()
		if err != nil {
			log.Fatalf("[FATAL] Không thể khởi động Windows Service handler: %v", err)
		}
		select {
		case <-svcStopCh:
			log.Println("[INFO] Nhận lệnh Stop từ Windows SCM, đang Graceful Shutdown...")
		case sig := <-quit:
			log.Printf("[INFO] Nhận tín hiệu OS (%s), đang Graceful Shutdown...", sig)
		}
	} else {
		sig := <-quit
		log.Printf("[INFO] Nhận tín hiệu dừng (%s), đang thực hiện Graceful Shutdown...", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if tailer != nil {
		tailer.Stop()
	}
	if codexMonitor != nil {
		codexMonitor.Stop()
	}
	if claudeMonitor != nil {
		claudeMonitor.Stop()
	}
	if proxyServer != nil {
		_ = proxyServer.Shutdown(ctx)
	}
	_ = dashServer.Shutdown(ctx)
	rollupTicker.Stop()
	close(rollupDone)
	buf.Stop()

	if backupTicker != nil {
		backupTicker.Stop()
		close(backupDone)
		log.Println("[INFO] 💾 Đang thực hiện bản sao lưu an toàn cuối cùng trước khi tắt...")
		if _, err := store.PerformAutoBackup(); err != nil {
			log.Printf("[WARN] Không thể tạo backup cuối cùng: %v", err)
		}
	}

	log.Println("[INFO] TokenMonitor đã dừng an toàn và lưu toàn bộ dữ liệu.")
}
