/**
 * Module tiện ích cho giao diện và tương tác
 */

/**
 * Cập nhật trạng thái kết nối
 * @param {string} status
 * @param {string} text
 */
function updateConnectionStatus(status, text) {
  const statusElement = document.getElementById("connection-status");
  const statusText = document.getElementById("status-text");
  
  statusElement.className = `connection-status status-${status}`;
  statusText.textContent = text;
}

/**
 * Hiển thị thông báo trên giao diện
 * @param {string} message
 * @param {string} type
 */
function showNotification(message, type = 'info') {
  const existingNotifications = document.querySelectorAll('.notification');
  existingNotifications.forEach(notification => notification.remove());
  
  const notification = document.createElement('div');
  notification.className = `notification ${type}`;
  notification.textContent = message;
  
  document.body.appendChild(notification);
  
  setTimeout(() => {
    if (notification.parentNode) {
      notification.remove();
    }
  }, 3000);
}

/**
 * Hiển thị chỉ báo đang nhập
 */
function showTypingIndicator() {
  const indicator = document.getElementById("typing-indicator");
  if (indicator) {
    indicator.style.display = "block";
  }
}

/**
 * Ẩn chỉ báo đang nhập
 */
function hideTypingIndicator() {
  const indicator = document.getElementById("typing-indicator");
  if (indicator) {
    indicator.style.display = "none";
  }
}

/**
 * Xử lý sự kiện khi người dùng đang nhập
 */
function handleTyping() {
  const now = Date.now();
  if (now - lastTypingTime > 1000) {
    showTypingIndicator();
    lastTypingTime = now;
  }
  
  if (typingTimeout) {
    clearTimeout(typingTimeout);
  }
  
  typingTimeout = setTimeout(() => {
    hideTypingIndicator();
  }, 2000);
}

/**
 * Xử lý phím Enter để gửi tin nhắn
 * @param {Event} event
 */
function handleKeyPress(event) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault();
    if (!document.getElementById("send-button").disabled) {
      sendMessage(event);
    }
  }
}

/**
 * Xử lý phím Enter để tham gia chat
 * @param {Event} event
 */
function handleNameKeyPress(event) {
  if (event.key === 'Enter') {
    event.preventDefault();
    if (!document.getElementById("join-button").disabled) {
      onJoin();
    }
  }
}

/**
 * Ngăn chặn form submit trên toàn trang
 */
function preventFormSubmit(event) {
  event.preventDefault();
  return false;
}

document.addEventListener("DOMContentLoaded", () => {
  // Gắn event listeners cho các button
  const joinBtn = document.getElementById("join-button");
  const sendBtn = document.getElementById("send-button");
  const disconnectBtn = document.getElementById("disconnect-button");
  
  if (joinBtn) {
    joinBtn.addEventListener("click", (e) => {
      e.preventDefault();
      onJoin();
    });
  }
  
  if (sendBtn) {
    sendBtn.addEventListener("click", (e) => {
      e.preventDefault();
      sendMessage(e);
    });
  }
  
  if (disconnectBtn) {
    disconnectBtn.addEventListener("click", (e) => {
      e.preventDefault();
      onDisconnect();
    });
  }
  
  // Gắn event cho input fields
  const messageInput = document.getElementById("message");
  const nameInput = document.getElementById("name");
  
  if (messageInput) {
    messageInput.addEventListener("keypress", handleKeyPress);
    messageInput.addEventListener("input", handleTyping);
  }
  
  if (nameInput) {
    nameInput.addEventListener("keypress", handleNameKeyPress);
  }
  
  // Ngăn chặn mọi form submit trên trang
  document.addEventListener('submit', preventFormSubmit);
  
  const welcomeTime = document.getElementById("welcome-time");
  if (welcomeTime) {
    welcomeTime.textContent = new Date().toLocaleTimeString();
  }
});

let typingTimeout = null;
let lastTypingTime = 0;