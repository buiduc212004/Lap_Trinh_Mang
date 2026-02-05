/**
 * Module xử lý giao diện người dùng
 */

/**
 * Cập nhật danh sách người dùng online
 * @param {Array} list - Danh sách tên người dùng online
 */
function updateOnlineList(list) {
  const onlineList = document.getElementById("online-list");
  const userCount = document.getElementById("user-count");
  
  if (list.length === 0) {
    onlineList.innerHTML = `
      <div class="has-text-centered has-text-grey">
        <i class="fas fa-user-friends fa-2x" style="opacity: 0.3;"></i>
        <p style="margin-top: 10px;">No users online</p>
      </div>
    `;
    userCount.textContent = "0";
    return;
  }

  onlineList.innerHTML = "";
  list.forEach((userName) => {
    const userDiv = document.createElement("div");
    userDiv.className = "online-user";
    
    const avatar = document.createElement("div");
    avatar.className = "user-avatar";
    avatar.textContent = userName.charAt(0).toUpperCase();
    
    const userInfo = document.createElement("div");
    userInfo.className = "user-info";
    
    const nameSpan = document.createElement("div");
    nameSpan.className = "user-name";
    nameSpan.textContent = userName;
    
    const statusSpan = document.createElement("div");
    statusSpan.className = "user-status";
    statusSpan.textContent = "Online";
    
    userInfo.appendChild(nameSpan);
    userInfo.appendChild(statusSpan);
    
    userDiv.appendChild(avatar);
    userDiv.appendChild(userInfo);
    onlineList.appendChild(userDiv);
  });
  
  userCount.textContent = list.length.toString();
  console.log("Online clients:", list);
}

/**
 * Thêm một tin nhắn vào khung chat
 * @param {string} sender - Tên người gửi
 * @param {string} message - Nội dung tin nhắn
 */
function addMessageElement(sender, message) {
  const messageDiv = document.createElement("div");
  messageDiv.className = "message";
  
  if (sender === "SYSTEM") {
    messageDiv.classList.add("system");
  } else if (sender === name) {
    messageDiv.classList.add("own");
  } else {
    messageDiv.classList.add("other");
  }

  const messageBubble = document.createElement("div");
  messageBubble.className = "message-bubble";

  if (sender !== "SYSTEM") {
    const messageInfo = document.createElement("div");
    messageInfo.className = "message-info";

    const senderSpan = document.createElement("span");
    senderSpan.className = "message-sender";
    senderSpan.textContent = sender;

    const timeSpan = document.createElement("span");
    timeSpan.className = "message-time";
    timeSpan.textContent = new Date().toLocaleTimeString();

    messageInfo.appendChild(senderSpan);
    messageInfo.appendChild(timeSpan);
    messageBubble.appendChild(messageInfo);
  }

  const messageText = document.createElement("div");
  messageText.className = "message-text";
  messageText.textContent = message;

  messageBubble.appendChild(messageText);
  messageDiv.appendChild(messageBubble);

  const messages = document.getElementById("messages");
  messages.appendChild(messageDiv);
  messages.scrollTop = messages.scrollHeight;
}

/**
 * Xử lý khi người dùng nhấn Join
 * @returns {Promise<void>}
 */
async function onJoin() {
  name = document.getElementById("name").value.trim();
  if (!name) {
    showNotification('Please enter your name!', 'error');
    return;
  }

  if (name.length < 2) {
    showNotification('Name must be at least 2 characters!', 'error');
    return;
  }

  try {
    await connect();

    document.getElementById("name").disabled = true;
    document.getElementById("send-button").disabled = false;
    document.getElementById("message").disabled = false;
    document.getElementById("join-button").disabled = true;
    document.getElementById("disconnect-button").disabled = false;

    const welcomeTime = document.getElementById("welcome-time");
    if (welcomeTime) {
      welcomeTime.textContent = new Date().toLocaleTimeString();
    }
  } catch (error) {
    console.error("Join failed:", error);
    showNotification('Failed to join chat. Please try again.', 'error');
  }
}

/**
 * Xử lý khi người dùng nhấn Disconnect
 * @returns {Promise<void>}
 */
async function onDisconnect() {
  if (transport) {
    await transport.close();
    updateUIOnDisconnect();
    addMessageElement("SYSTEM", "Disconnected from server");
  }
}

/**
 * Cập nhật giao diện khi ngắt kết nối
 */
function updateUIOnDisconnect() {
  document.getElementById("name").disabled = false;
  document.getElementById("send-button").disabled = true;
  document.getElementById("message").disabled = true;
  document.getElementById("join-button").disabled = false;
  document.getElementById("disconnect-button").disabled = true;
  
  updateOnlineList([]);
  updateConnectionStatus('disconnected', 'Disconnected');
  transport = null;
  isConnecting = false;
}