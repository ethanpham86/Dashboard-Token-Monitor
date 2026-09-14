package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CreateBackup thực hiện sao lưu nguyên tử (atomic snapshot) database SQLite sang destPath
// sử dụng tính năng native VACUUM INTO của SQLite.
// Điểm mạnh:
// 1. Tự động merge toàn bộ dữ liệu từ Write-Ahead Log (.db-wal) vào file đích.
// 2. Không khóa tiến trình ghi (Non-blocking) đối với các request đang chạy.
// 3. File sinh ra là một file .db hoàn chỉnh, độc lập, không cần kèm theo file -wal hay -shm.
func (s *Storage) CreateBackup(destPath string) error {
	if destPath == "" {
		return fmt.Errorf("đường dẫn file backup không được để trống")
	}

	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("không thể tạo thư mục backup %s: %w", destDir, err)
	}

	// SQLite VACUUM INTO sẽ báo lỗi nếu file đích đã tồn tại sẵn, do đó xóa file cũ nếu có
	if _, err := os.Stat(destPath); err == nil {
		if err := os.Remove(destPath); err != nil {
			return fmt.Errorf("không thể dọn dẹp file đích cũ %s: %w", destPath, err)
		}
	}

	// Chuẩn hóa đường dẫn dạng forward slash cho SQLite engine
	cleanPath := filepath.ToSlash(filepath.Clean(destPath))
	escapedPath := strings.ReplaceAll(cleanPath, "'", "''")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	query := fmt.Sprintf("VACUUM INTO '%s';", escapedPath)
	if _, err := s.DB.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("lỗi thực thi VACUUM INTO: %w", err)
	}

	return nil
}

// PerformAutoBackup thực thi chu trình sao lưu tự động dựa trên cấu hình:
// 1. Tạo file snapshot timestamp: token_monitor_backup_YYYYMMDD_HHMMSS.db
// 2. Cập nhật bản sao nhanh token_monitor.db trong thư mục backup
// 3. Dọn dẹp các bản backup cũ vượt quá giới hạn MaxKeep
func (s *Storage) PerformAutoBackup() (string, error) {
	if !s.cfg.Backup.Enabled {
		return "", nil
	}

	backupDir := s.cfg.Backup.BackupDir
	if backupDir == "" {
		backupDir = "./data/backup"
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("không thể tạo thư mục backup %s: %w", backupDir, err)
	}

	// Đặt tên theo ngày (YYYYMMDD): mỗi ngày duy trì đúng 1 file duy nhất, cập nhật liên tục trong ngày
	timestamp := time.Now().Format("20060102")
	destName := fmt.Sprintf("token_monitor_backup_%s.db", timestamp)
	destPath := filepath.Join(backupDir, destName)

	start := time.Now()
	if err := s.CreateBackup(destPath); err != nil {
		return "", fmt.Errorf("sao lưu thất bại sang %s: %w", destPath, err)
	}
	duration := time.Since(start)

	fileInfo, err := os.Stat(destPath)
	var fileSize int64
	if err == nil {
		fileSize = fileInfo.Size()
	}

	log.Printf("[INFO] 💾 Auto-Backup hoàn tất thành công: %s (Dung lượng: %d bytes, Thời gian: %v)",
		destPath, fileSize, duration)

	// Đồng bộ bản sao token_monitor.db (chuẩn hóa phục hồi nhanh)
	latestPath := filepath.Join(backupDir, "token_monitor.db")
	if err := copyFileAtomic(destPath, latestPath); err != nil {
		log.Printf("[WARN] Không thể cập nhật file token_monitor.db trong backup: %v", err)
	}

	// Dọn dẹp bản backup cũ theo retention policy (MaxKeep)
	if s.cfg.Backup.MaxKeep > 0 {
		if err := s.PruneOldBackups(backupDir, s.cfg.Backup.MaxKeep); err != nil {
			log.Printf("[WARN] Lỗi dọn dẹp backup cũ: %v", err)
		}
	}

	return destPath, nil
}

// PruneOldBackups quét thư mục backup và xóa các file dạng token_monitor_backup_*.db
// chỉ giữ lại số lượng bản mới nhất bằng maxKeep.
func (s *Storage) PruneOldBackups(backupDir string, maxKeep int) error {
	if maxKeep <= 0 {
		return nil
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("không thể đọc thư mục backup %s: %w", backupDir, err)
	}

	var backupFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "token_monitor_backup_") && strings.HasSuffix(name, ".db") {
			backupFiles = append(backupFiles, filepath.Join(backupDir, name))
		}
	}

	// Tên file có định dạng YYYYMMDD_HHMMSS nên sort theo thứ tự alphabet sẽ đúng thứ tự thời gian tăng dần
	sort.Strings(backupFiles)

	if len(backupFiles) > maxKeep {
		excess := len(backupFiles) - maxKeep
		for i := 0; i < excess; i++ {
			toDelete := backupFiles[i]
			if err := os.Remove(toDelete); err != nil {
				log.Printf("[WARN] Không thể xóa file backup cũ %s: %v", toDelete, err)
			} else {
				log.Printf("[INFO] 🗑️ Đã xoay vòng dọn dẹp bản backup cũ: %s", filepath.Base(toDelete))
			}
		}
	}

	return nil
}

// copyFileAtomic sao chép file an toàn qua file tạm rồi đổi tên
func copyFileAtomic(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	tmpDst := dst + ".tmp"
	dstFile, err := os.Create(tmpDst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		_ = os.Remove(tmpDst)
		return err
	}
	if err := dstFile.Close(); err != nil {
		_ = os.Remove(tmpDst)
		return err
	}

	return os.Rename(tmpDst, dst)
}
