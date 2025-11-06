/**
 * Module xử lý tin nhắn
 */

async function sendMessage(event) {
  // Ngăn chặn form submit nếu được gọi từ event
  if (event) {
    event.preventDefault();
  }
  
  const msgInput = document.getElementById("message");
  const message = msgInput.value.trim();
  if (!message || !transport) return;

  try {
    const encoder = new TextEncoder();
    const stream = await transport.createUnidirectionalStream(); 
    const writer = stream.getWriter();
    await writer.write(encoder.encode(JSON.stringify({ type: "chat", name, message })));
    await writer.close();
    msgInput.value = "";
    console.log("Sent message:", message);
    hideTypingIndicator();
  } catch (error) {
    console.error("Failed to send message:", error);
    showNotification('Failed to send message', 'error');
  }
}

/**
 * Đọc tin nhắn liên tục từ Stream vĩnh viễn
 */
async function readContinuousMessages(stream) {
    const decoder = new TextDecoderStream("utf-8");
    const reader = stream.pipeThrough(decoder).getReader();

    while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        
        try {
            const msg = JSON.parse(value);
            
            if (msg.type === "chat") {
                addMessageElement(msg.name, msg.message);
            } else if (msg.type === "system") {
                addMessageElement("SYSTEM", msg.message);
            } else if (msg.type === "file") {
                // Hiển thị thông báo file mới
                addFileNotification(msg.name, msg.filename, msg.size);
            }
        } catch(e) {
            console.error("Failed to parse JSON from continuous stream:", e, "Data received:", value);
        }
    }
    console.log("Persistent message stream closed.");
}

/**
 * Thêm thông báo file vào chat
 */
function addFileNotification(sender, filename, size) {
  const messageDiv = document.createElement("div");
  messageDiv.className = "message file-notification";

  const messageBubble = document.createElement("div");
  messageBubble.className = "message-bubble file-bubble";

  const fileIcon = document.createElement("div");
  fileIcon.className = "file-notification-icon";
  fileIcon.innerHTML = '<i class="fas fa-file-upload"></i>';

  const fileContent = document.createElement("div");
  fileContent.className = "file-notification-content";

  const fileTitle = document.createElement("div");
  fileTitle.className = "file-notification-title";
  fileTitle.innerHTML = `<strong>${sender}</strong> shared a file`;

  const fileInfo = document.createElement("div");
  fileInfo.className = "file-notification-info";
  fileInfo.innerHTML = `
    <i class="fas fa-file"></i>
    <span>${filename}</span>
    <span class="file-size">(${formatFileSize(size)})</span>
  `;

  const downloadBtn = document.createElement("button");
  downloadBtn.className = "file-notification-btn";
  downloadBtn.innerHTML = '<i class="fas fa-download"></i> Download';
  downloadBtn.onclick = () => downloadFile(filename);

  fileContent.appendChild(fileTitle);
  fileContent.appendChild(fileInfo);
  fileContent.appendChild(downloadBtn);

  messageBubble.appendChild(fileIcon);
  messageBubble.appendChild(fileContent);
  messageDiv.appendChild(messageBubble);

  const messages = document.getElementById("messages");
  messages.appendChild(messageDiv);
  messages.scrollTop = messages.scrollHeight;
}