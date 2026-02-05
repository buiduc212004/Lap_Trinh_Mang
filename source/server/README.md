# MODULE SERVER

> 📘 Server Go sử dụng WebTransport để cung cấp chat, multi-stream file upload/download và các tính năng chia sẻ (drawing, file list, online list).

---

## 🎯 MỤC TIÊU

Server chịu trách nhiệm:
- Tiếp nhận kết nối WebTransport từ client (endpoint `/chat`).
- Quản lý session/clients, phân phối tin nhắn chat qua persistent stream.
- Nhận file upload theo multi-stream (ghép các chunk trên server) và lưu vào thư mục `uploads/`.
- Gửi datagrams (ví dụ: danh sách users online hoặc file list) tới client.

---

## ⚙️ CÔNG NGHỆ SỬ DỤNG

| Thành phần | Công nghệ |
|:-----------|:----------|
| Ngôn ngữ | Go (go1.24) |
| WebTransport | github.com/quic-go/quic-go, github.com/quic-go/webtransport-go |
| TLS | self-signed certs for local dev (localhost.pem / localhost-key.pem) |

---

## 🚀 HƯỚNG DẪN CHẠY

### Yêu cầu

- go 1.24

### Tạo chứng chỉ TLS cho localhost:

```powershell
# localhost.pem và localhost-key.pem
cd source\server
mkcert -install
mkcert localhost
```

### Chạy server:

```powershell
# Vẫn trong source\server
go build

# Đã được build thành file .exe, tiến hành chạy
start .\source.exe
```

- Server mặc định lắng nghe trên port `:4433`. Khi khởi động lần đầu `main.go` sẽ tạo thư mục `uploads/` nếu chưa tồn tại.

---

## 🔗 Endpoint & Giao thức (tóm tắt)

- `/chat` — endpoint WebTransport API.
- Client mở `new WebTransport('https://localhost:4433/chat?name=...')` (xem `source/client/connection.js`).

Truyền thông chính giữa client/server trong project:
- Tin nhắn chat: client gửi JSON `{type: 'chat', name, message}` qua unidirectional stream; server phát lại trên persistent stream.
- Datagrams: server gửi danh sách online và file list dưới dạng datagram JSON `{type: 'online', clients: [...]}` hoặc `{type: 'file_list', files: [...]}`.
- File upload: client chia file thành NUM_STREAMS chunks, gửi từng chunk qua bidirectional streams; server nhận chunks, lưu tạm và merge khi đầy đủ.
- Drawing: client gửi header + binary PNG qua bidirectional stream; server trả JSON status.

---

## 📦 CẤU TRÚC
```
server/
├── uploads/                # Thư mục đích để lưu file upload - Được sinh ra khi chạy các lệnh
├── client.go               # Cấu trúc đại diện cho một client kết nối
├── config.go               # Các hằng cấu hình (CHUNK_SIZE, NUM_STREAMS) và buffer pool
├── drawing_handler.go      # Xử lý bản vẽ: nhận dữ liệu PNG, lưu hoặc chuyển tiếp bản vẽ tới các client
├── file_handler.go         # Xử lý up/download file: nhận upload theo các chunk, lưu tạm, ghép các chunk và phục vụ file
├── go.mod                  # Định nghĩa Go module
├── go.sum                  # Checksum của dependencies
├── localhost.pem           # TLS cert (dev) - Được sinh ra khi chạy các lệnh
├── localhost-key.pem       # TLS key (dev) - Được sinh ra khi chạy các lệnh
├── main.go                 # Entrypoint, khởi tạo server và handler cho /chat
├── server.go               # Xử lý logic phiên, stream và file
├── session_handler.go      # Quản lý phiên: theo dõi các client đang kết nối, cấp ID phiên, phát tin nhắn đến client
├── source.exe              # Build artifact (binary) - Được sinh ra khi chạy các lệnh
└── README.md               # (this file)
```

---

## 🧪 TEST

- Thư mục `uploads/`: `main.go` sẽ tạo `uploads/` với mode `0755` khi khởi động. Kiểm tra quyền nếu không thể ghi file.

- Kiểm tra logs: server in thông tin khi khởi động (chunk size, num streams). Kiểm tra output console để biết trạng thái.

---

## 📝 GHI CHÚ

- Đảm bảo client được phục vụ trên secure context (HTTPS) và tin cậy cert dev khi cần thử WebTransport trên trình duyệt.
