# Rà soát Codex Monitor — 2026-09-15

Tab OpenAI / Codex trước sửa chưa phản ánh chính xác usage cục bộ. Lỗi quan trọng nhất là cộng cả `token_usage_record` và `event_msg.token_count` cho cùng một cập nhật. Đã đối chiếu cấu trúc và bộ đếm trong 3 file session gần nhất trên máy; không đưa nội dung hội thoại hoặc thông tin xác thực vào báo cáo.

## Các thay đổi

| Vấn đề | Cách xử lý |
| --- | --- |
| Cùng usage được ghi ở hai dạng record | Dùng chung baseline `thread_token_usage` / `total_token_usage`; lấy chênh lệch từng thành phần. Khử replay `response_id`. |
| Chỉ có cumulative total hoặc mất cập nhật trung gian | Không yêu cầu `last_token_usage`; lấy chênh lệch tổng đã quan sát. |
| Bộ đếm giảm sau reset/compaction | Đổi baseline, bỏ chênh lệch âm và báo `PARTIAL`. Không coi toàn bộ snapshot thấp hơn là usage mới. |
| Cộng chồng cached/reasoning vào tổng biểu đồ | Stacked chart dùng uncached input + cached input + non-reasoning output + reasoning. |
| Gán mọi bucket cho model đầu tiên | Trả `model_time_series` theo model thực tế ghi trong `turn_context`. |
| Đồ thị tự tạo 5 công cụ, chia token đều và giả định concurrency 16 | Chỉ dựng workspace/session quan sát được; hiển thị số session có log gần đây. |
| Hai thư mục trùng tên bị gộp trong đồ thị | Dùng mã băm đường dẫn để phân nhóm; chỉ xuất tên thư mục và mã băm, không xuất đường dẫn đầy đủ. |
| Giới hạn 50 dòng làm mất session trong graph/tổng hợp dự án | Có đường tổng hợp không giới hạn số dòng giao diện. |
| Bản sao file cùng session bị cộng lại | Chọn bản có mtime mới nhất theo session ID, thứ tự ổn định khi mtime bằng nhau. |
| Input/cache/output theo dự án bị phân bổ theo tỷ lệ chung | Dùng bộ đếm riêng của session; không suy từ tỷ lệ tổng. |
| File lỗi bị bỏ qua âm thầm / chạm max_files | Báo `PARTIAL`, số file phát hiện, cờ giới hạn và lỗi tổng quát; không xuất đường dẫn lỗi qua API. |
| Đang ghi dở dòng JSONL | Chờ dòng hoàn thiện; kiểm thử append và truncate. |
| Bỏ qua CODEX_HOME | Ưu tiên `sessions_dir`, tiếp đến `$CODEX_HOME/sessions`, cuối cùng `~/.codex/sessions`. |
| Mặc định Plus, quota 5h/7d, 100% tool success hoặc billing cố định | Hiển thị UNKNOWN/N/A khi thiếu dữ liệu; quota có timestamp và session nguồn, đánh dấu cũ; cửa sổ lấy từ log. |

Giữ cache theo kích thước/mtime để không parse lại file không đổi. Bỏ bước giải mã payload không liên quan và vòng lặp thứ hai để tính cache theo session. File thay đổi vẫn được parse lại toàn bộ; chưa triển khai tail theo offset.

## Phạm vi và giới hạn

- Đây là **usage của các session cục bộ đã quét**, không phải danh sách người dùng, số tài khoản, usage toàn tổ chức, hoặc tổng của mọi thiết bị. Không quét Windows profile của người khác.
- Không tự quét `archived_sessions`, log đã xóa, cloud tasks hoặc các thư mục khác ngoài nguồn cấu hình. `all` vẫn chịu giới hạn `max_files`.
- `model_calls` giữ tên API cũ nhưng mang nghĩa **cập nhật usage có token**; một delta có thể gộp nhiều request. Timestamp/model của delta là nơi ghi nhận, không khôi phục được chính xác các request bị thiếu.
- Log fork có lịch sử kế thừa nhưng session ID khác có thể chứa usage trùng. Khử bản sao cùng ID không giải quyết được mọi dạng fork hoặc log bị cắt/rewrite. Khi bộ đếm reset, cách xử lý bảo thủ có thể bỏ sót usage; đã báo `PARTIAL`.
- `ACTIVE` là log trong 2 phút và chưa thấy kết thúc turn; không chứng minh tiến trình đang chạy. `COMPLETED` là sự kiện kết thúc turn quan sát được, không có nghĩa session sẽ không được mở lại.
- Tool invocation được đếm từ loại record. Không giải mã output dạng văn bản để suy đoán thành công; chưa có kết quả cấu trúc thì tỷ lệ là N/A. Khi file có invocation records, dùng nguồn đó thay cho item-completed để tránh đếm hai lần.
- Quota là snapshot của session, có thể thuộc tài khoản đăng nhập trước đây; không xác minh tài khoản hiện tại. Chỉ chọn nhóm quota `codex`/legacy, không trộn nhóm model khác. Ngưỡng “cũ” 2 phút là quy tắc hiển thị của monitor, không phải quy định của OpenAI.
- Chi phí trong tab Codex là N/A. Bảng FinOps chung vẫn dùng ước tính của repository theo model và đơn giá trong mã, không phải hóa đơn; chưa giải quyết định giá theo từng request khi một session đổi model. Quy tắc hợp nhất dự án liên provider vẫn có thể gộp thư mục cùng tên.
- JSONL là định dạng triển khai cục bộ, không được coi là API ổn định cho mọi phiên bản Codex.

## Quyền riêng tư và tài liệu chính thức

Collector mở log bằng chế độ đọc, không truy cập `auth.json` hoặc credential store, không đăng nhập/refresh token và không thêm request mạng. Nội dung prompt, lệnh, phản hồi không được lưu trong DTO hoặc database bởi collector này; các byte JSONL có đi qua bộ đọc để trích metadata. Dashboard vẫn chứa tên workspace, model, session ID và thời gian hoạt động.

Cấu hình hiện tại bind dashboard vào `127.0.0.1`, proxy tắt. Phạm vi này phù hợp với việc theo dõi log trên máy do chủ máy cho phép; không bao gồm cơ chế vượt quyền hoặc thu thập bí mật thông tin đăng nhập. Đây là rà soát kỹ thuật, không phải chứng nhận pháp lý cho mọi cách triển khai theo dõi nhân viên. Việc mở dashboard ra mạng hoặc tập trung dữ liệu nhiều người cần thiết kế quyền truy cập và phạm vi đồng ý riêng.

Tài liệu chính thức đã đọc:

- [Codex authentication](https://developers.openai.com/codex/auth): `CODEX_HOME`, các credential store và yêu cầu bảo vệ `auth.json` như mật khẩu.
- [Codex app-server](https://developers.openai.com/codex/app-server): giao diện chính thức có `account/read`, `account/rateLimits/read`, `account/usage/read` và `thread/tokenUsage/updated`. Tài liệu này không xác nhận raw JSONL là giao diện ổn định hoặc cho phép suy người dùng từ session. Bản sửa hiện tại không khởi chạy app-server hay truy vấn tài khoản.

## Kiểm thử

- `go test ./...` — đạt.
- `go vet ./...` — đạt.
- `node tests_codex_audit.js` — đạt: cú pháp script, thiếu/cũ metadata, stacked token và model attribution.
- `git diff --check` — đạt.

Kiểm thử hồi quy bao phủ hai dạng record theo cả hai thứ tự, response replay, cumulative-only/gap/reset, phân tách model, kết thúc turn, tool output chưa biết, nội dung không lọt DTO, bản sao session, giới hạn file/dòng, graph không tự sinh node, CODEX_HOME, nhóm quota và lỗi API không lộ đường dẫn. Fixture kiểm thử chi phí cũ được bổ sung bộ đếm session thực tế; giữ nguyên kết quả chi phí mong đợi.

Chưa xác nhận bố cục bằng trình duyệt tương tác. Chưa khởi động lại ứng dụng đang chạy; thay đổi nằm trong source và cần build/restart để áp dụng vào tiến trình hiện tại.
