package storage

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite_detector", &sqlite.Driver{})
}

type AntigravityAuthPayload struct {
	Name                        string `json:"name"`
	Email                       string `json:"email"`
	APIKey                      string `json:"apiKey"`
	UserStatusProtoBinaryBase64 string `json:"userStatusProtoBinaryBase64"`
}

func getAntigravityStateDB() (*sql.DB, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "AppData", "Roaming")
	}

	// Các đường dẫn tiềm năng trên các phiên bản Antigravity IDE (ưu tiên "Antigravity IDE" chuẩn Windows)
	candidates := []string{
		os.Getenv("ANTIGRAVITY_STATE_DB"),
		filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
		filepath.Join(appData, "Antigravity", "User", "globalStorage", "state.vscdb"),
	}

	var dbPath string
	for _, c := range candidates {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				dbPath = c
				break
			}
		}
	}

	if dbPath == "" {
		return nil, fmt.Errorf("không tìm thấy file state.vscdb tại các đường dẫn Antigravity")
	}

	// Mở read-only để không can thiệp hay khóa file CSDL của IDE
	db, err := sql.Open("sqlite_detector", fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(dbPath)))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db, nil
}

// DetectActiveAntigravityAccount tự động đọc phiên đăng nhập thực tế của Antigravity IDE
// Tuân thủ 100% nguyên tắc an toàn: mode=ro, zero mật khẩu Gmail, 100% offline nội bộ.
func DetectActiveAntigravityAccount() (name, email, plan string, err error) {
	db, err := getAntigravityStateDB()
	if err != nil {
		return "", "", "", err
	}
	defer db.Close()

	decodeB64 := func(s string) ([]byte, error) {
		s = strings.TrimSpace(s)
		b, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			return b, nil
		}
		return base64.RawStdEncoding.DecodeString(s)
	}

	// 1. Chế độ 1 (Primary): Kiểm tra key antigravityAuthStatus (JSON format)
	var valJSON string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'antigravityAuthStatus'").Scan(&valJSON)
	if err == nil && strings.TrimSpace(valJSON) != "" {
		var payload AntigravityAuthPayload
		if err := json.Unmarshal([]byte(valJSON), &payload); err == nil {
			name = payload.Name
			email = payload.Email
			plan = "Google AI Pro"

			if payload.UserStatusProtoBinaryBase64 != "" {
				if protoBytes, err := decodeB64(payload.UserStatusProtoBinaryBase64); err == nil {
					protoStr := string(protoBytes)
					if strings.Contains(protoStr, "Google AI Ultra") || strings.Contains(protoStr, "g1-ultra-tier") {
						plan = "Google AI Ultra (20X Ultra Tier)"
					} else if strings.Contains(protoStr, "Pro") {
						plan = "Google AI Pro"
					}
				}
			}
			return name, email, plan, nil
		}
	}

	// 2. Chế độ 2 (Secondary): Kiểm tra key thực tế trên Antigravity IDE: antigravityUnifiedStateSync.userStatus
	var rawStatus string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'antigravityUnifiedStateSync.userStatus'").Scan(&rawStatus)
	if err != nil {
		return "", "", "", fmt.Errorf("không tìm thấy key auth trong ItemTable (thử cả antigravityAuthStatus và antigravityUnifiedStateSync.userStatus): %w", err)
	}

	rawBytes, err := decodeB64(rawStatus)
	if err != nil {
		return "", "", "", fmt.Errorf("lỗi giải mã base64 userStatus: %w", err)
	}

	emailRe := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

	// Tìm nested base64 protobuf stream trong chuỗi byte (hỗ trợ dịch chuyển 0-3 byte padding/tag)
	nestedB64Re := regexp.MustCompile(`[A-Za-z0-9+/=]{40,}`)
	protoBytes := rawBytes
	allNested := nestedB64Re.FindAll(rawBytes, -1)
	foundNested := false
	for _, nestedMatch := range allNested {
		strMatch := string(nestedMatch)
		for trim := 0; trim < 4; trim++ {
			if len(strMatch) > trim {
				candidate := strMatch[trim:]
				if len(candidate)%4 == 1 {
					candidate = candidate[:len(candidate)-1]
				}
				if decoded, err := decodeB64(candidate); err == nil && len(decoded) > 0 {
					if emailRe.Match(decoded) {
						protoBytes = decoded
						foundNested = true
						break
					}
				}
			}
		}
		if foundNested {
			break
		}
	}

	protoStr := string(protoBytes)

	// Bóc tách Email
	if emailMatch := emailRe.FindString(protoStr); emailMatch != "" {
		email = emailMatch
	}

	// Bóc tách Plan & Tier
	plan = "Google AI Pro"
	if strings.Contains(protoStr, "Google AI Ultra") || strings.Contains(protoStr, "g1-ultra-tier") {
		plan = "Google AI Ultra (20X Ultra Tier)"
	} else if strings.Contains(protoStr, "Pro") {
		plan = "Google AI Pro"
	}

	// Bóc tách Name (chuỗi ký tự in trước email trong protobuf)
	if email != "" {
		idx := strings.Index(protoStr, email)
		if idx > 0 {
			prefix := protoStr[:idx]
			wordsRe := regexp.MustCompile(`[\x20-\x7E]{2,}`)
			matches := wordsRe.FindAllString(prefix, -1)
			if len(matches) > 0 {
				candidate := strings.TrimSpace(matches[len(matches)-1])
				candidate = strings.TrimPrefix(candidate, "User:")
				candidate = strings.TrimPrefix(candidate, "User")
				candidate = strings.Trim(candidate, ": ")
				name = candidate
			}
		}
	}

	return name, email, plan, nil
}

// DetectInstallationUUID trích xuất mã UUID cài đặt từ key 'storage.serviceMachineId' trong ItemTable của state.vscdb
func DetectInstallationUUID() (string, error) {
	db, err := getAntigravityStateDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	var uuidVal string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'storage.serviceMachineId'").Scan(&uuidVal)
	if err != nil {
		return "", fmt.Errorf("không tìm thấy key storage.serviceMachineId: %w", err)
	}
	uuidVal = strings.TrimSpace(uuidVal)
	if uuidVal == "" {
		return "", fmt.Errorf("key storage.serviceMachineId có giá trị rỗng")
	}
	return uuidVal, nil
}

func decodeVarint(buf []byte) (uint64, int) {
	var val uint64
	var shift uint
	for i, b := range buf {
		if i >= 10 {
			return 0, -1
		}
		val |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return val, i + 1
		}
		shift += 7
	}
	return 0, -1
}

func extractExpiryFromProtobuf(data []byte) time.Time {
	offset := 0
	for offset < len(data) {
		tag, n := decodeVarint(data[offset:])
		if n <= 0 {
			break
		}
		offset += n
		field := tag >> 3
		wire := tag & 7
		switch wire {
		case 0: // varint
			v, vn := decodeVarint(data[offset:])
			if vn <= 0 {
				return time.Time{}
			}
			offset += vn
			// Plausible Unix timestamp (2020 to 2050: 1577836800 to 2524608000)
			if v >= 1577836800 && v <= 2524608000 {
				return time.Unix(int64(v), 0)
			}
		case 1: // 64-bit
			offset += 8
		case 2: // length-delimited
			length, ln := decodeVarint(data[offset:])
			if ln <= 0 {
				return time.Time{}
			}
			offset += ln
			if offset+int(length) > len(data) {
				return time.Time{}
			}
			subBytes := data[offset : offset+int(length)]
			offset += int(length)

			// Field 4 contains token expiry info submessage (\x08 varint)
			if field == 4 {
				if len(subBytes) > 1 && subBytes[0] == 0x08 {
					ts, _ := decodeVarint(subBytes[1:])
					if ts >= 1577836800 && ts <= 2524608000 {
						return time.Unix(int64(ts), 0)
					}
				}
				if subExp := extractExpiryFromProtobuf(subBytes); !subExp.IsZero() {
					return subExp
				}
			}
		case 5: // 32-bit
			offset += 4
		default:
			return time.Time{}
		}
	}
	return time.Time{}
}

// DetectOAuthTokenExpiry trích xuất thời điểm hết hạn OAuth token từ key 'antigravityUnifiedStateSync.oauthToken' trong state.vscdb
func DetectOAuthTokenExpiry() (time.Time, error) {
	db, err := getAntigravityStateDB()
	if err != nil {
		return time.Time{}, err
	}
	defer db.Close()

	var rawVal string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'antigravityUnifiedStateSync.oauthToken'").Scan(&rawVal)
	if err != nil {
		return time.Time{}, fmt.Errorf("không tìm thấy key antigravityUnifiedStateSync.oauthToken: %w", err)
	}

	decodeB64 := func(s string) ([]byte, error) {
		s = strings.TrimSpace(s)
		b, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			return b, nil
		}
		return base64.RawStdEncoding.DecodeString(s)
	}

	rawBytes, err := decodeB64(rawVal)
	if err != nil {
		return time.Time{}, fmt.Errorf("lỗi giải mã base64 oauthToken: %w", err)
	}

	nestedB64Re := regexp.MustCompile(`[A-Za-z0-9+/=]{40,}`)
	allNested := nestedB64Re.FindAll(rawBytes, -1)
	for _, nestedMatch := range allNested {
		strMatch := string(nestedMatch)
		for trim := 0; trim < 4; trim++ {
			if len(strMatch) > trim {
				candidate := strMatch[trim:]
				if len(candidate)%4 == 1 {
					candidate = candidate[:len(candidate)-1]
				}
				if decoded, err := decodeB64(candidate); err == nil && len(decoded) > 0 {
					if exp := extractExpiryFromProtobuf(decoded); !exp.IsZero() {
						return exp, nil
					}
				}
			}
		}
	}

	return time.Time{}, fmt.Errorf("không tìm thấy timestamp trong oauthToken")
}
