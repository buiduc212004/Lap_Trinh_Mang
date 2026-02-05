# MODULE CLIENT

> 📘 Phần Client của project: giao diện web (HTML/CSS/JavaScript) chịu trách nhiệm tương tác với người dùng — gửi/nhận tin nhắn, upload/download file và vẽ chia sẻ qua WebTransport.

---

## 🎯 MỤC TIÊU

Client chịu trách nhiệm chính:
- Cung cấp giao diện cho người dùng nhập tên để tham gia chat và gửi/nhận tin nhắn.
- Hiển thị danh sách người online và danh sách file có thể tải về.
- Upload và download file theo cơ chế multi-stream (client-side chunking).
- Hỗ trợ vẽ trên canvas và gửi bản vẽ tới phiên chat.

---

## ⚙️ CÔNG NGHỆ SỬ DỤNG

| Thành phần | Công nghệ |
|:-----------|:----------|
| Frontend | Vanilla HTML / CSS / JavaScript |
| Thư viện UI | Bulma (CSS), Font Awesome (icons) |
| Giao thức | WebTransport API — secure context (HTTPS) required |

---

## 🚀 HƯỚNG DẪN CHẠY

### Mở giao diện chat

```powershell
cd source\client\ui
start index.html
```

### Lưu ý trước khi chạy

1) Đảm bảo server đã chạy trước khi thử các tính năng WebTransport (xem hướng dẫn server trong `source/server/README.md`).

2) Phục vụ tĩnh thư mục `source/client/ui` bằng một static HTTPS server (trang cần secure context để WebTransport hoạt động).

---

## 📦 CẤU TRÚC
```
client/
├── connection.js      # Quản lý kết nối WebTransport, đọc datagrams và incoming streams
├── drawing.js         # Canvas drawing, gửi ảnh PNG qua stream
├── file.js            # Upload/download file với multi-stream, chunking
├── message.js         # Gửi/nhận tin nhắn qua streams
├── README.md          # (this file)
├── ui.js              # DOM updates, Join/Disconnect, hiển thị online list và messages
├── utils.js           # Helper UI (notifications, keyboard handlers, status)
└── ui/
    ├── index.html     # Trang giao diện chính
    └── style.css      # Style cho giao diện
```

---

## 💡 SỬ DỤNG

1) Mở trang `index.html` qua HTTPS như hướng dẫn ở trên.
2) Nhập tên rồi bấm **Join Chat** để kết nối tới server (endpoint được cấu hình trong `connection.js`).
3) Gửi tin nhắn, xem danh sách người online và file có sẵn trong sidebar.
4) Upload file: chọn file → Upload (client sẽ thực hiện chunking và upload nhiều stream song song).
5) Download file: nhấn nút download bên cạnh file trong list.
6) Vẽ: bấm "Draw" để mở canvas → vẽ → Send Drawing.

---

## 📝 GHI CHÚ

- Trang client cần được phục vụ trong secure context (HTTPS) để WebTransport hoạt động.
- `index.html` hiện tham chiếu tới script `../client/*.js` — khi serve `source/client/ui` bằng static server, đường dẫn này sẽ tải đúng các file JS trong `source/client`.
- Nếu nút **Join Chat** không hoạt động: mở DevTools → Console để kiểm tra lỗi (404, uncaught exceptions).