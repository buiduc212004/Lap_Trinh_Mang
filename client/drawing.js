/**
 * Module xử lý chức năng vẽ và chia sẻ
 */

let isDrawing = false;
let lastX = 0;
let lastY = 0;
let currentColor = '#000000';
let lineWidth = 3;
let drawingData = []; // Lưu tất cả các nét vẽ

/**
 * Mở modal vẽ
 */
function openDrawingModal() {
  const modal = document.getElementById('drawing-modal');
  const canvas = document.getElementById('drawing-canvas');
  const ctx = canvas.getContext('2d');
  
  // Reset canvas
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  drawingData = [];
  
  modal.classList.add('is-active');
  
  // Set default color
  currentColor = '#000000';
  document.getElementById('color-black').checked = true;
}

/**
 * Đóng modal vẽ
 */
function closeDrawingModal() {
  const modal = document.getElementById('drawing-modal');
  modal.classList.remove('is-active');
  drawingData = [];
}

/**
 * Xóa toàn bộ canvas
 */
function clearCanvas() {
  const canvas = document.getElementById('drawing-canvas');
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  drawingData = [];
}

/**
 * Thay đổi màu vẽ
 */
function changeColor(color) {
  currentColor = color;
  lineWidth = 3; // Vẽ bình thường
}

/**
 * Chọn chế độ tẩy
 */
function selectEraser() {
  currentColor = '#ffffff';
  lineWidth = 20; // Tẩy với nét rộng hơn
  document.getElementById('color-eraser').checked = true;
}

/**
 * Gửi bản vẽ lên server
 */
async function sendDrawing() {
  if (!transport) {
    showNotification('Not connected to server!', 'error');
    return;
  }

  if (drawingData.length === 0) {
    showNotification('Canvas is empty!', 'error');
    return;
  }

  const canvas = document.getElementById('drawing-canvas');
  
  try {
    // Chuyển canvas thành blob
    const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/png'));
    const arrayBuffer = await blob.arrayBuffer();
    const uint8Array = new Uint8Array(arrayBuffer);

    // Tạo stream để gửi
    const stream = await transport.createBidirectionalStream();
    const writer = stream.writable.getWriter();
    const encoder = new TextEncoder();

    // Gửi header
    const header = JSON.stringify({
      op: "drawing",
      size: uint8Array.length,
      format: "png"
    }) + "\n";
    await writer.write(encoder.encode(header));

    // Gửi dữ liệu ảnh
    await writer.write(uint8Array);
    await writer.close();

    // Đọc response
    const reader = stream.readable.getReader();
    const decoder = new TextDecoder();
    let response = '';
    
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      response += decoder.decode(value, { stream: true });
    }

    const result = JSON.parse(response.trim());
    
    if (result.status === "ok") {
      showNotification('Drawing sent successfully!', 'success');
      closeDrawingModal();
    } else {
      throw new Error(result.error || "Failed to send drawing");
    }

  } catch (error) {
    console.error("Send drawing error:", error);
    showNotification(`Failed to send drawing: ${error.message}`, 'error');
  }
}

/**
 * Hiển thị bản vẽ nhận được trong chat
 */
function displayReceivedDrawing(sender, imageData) {
  const messageDiv = document.createElement("div");
  messageDiv.className = "message drawing-message";
  
  if (sender === name) {
    messageDiv.classList.add("own");
  } else {
    messageDiv.classList.add("other");
  }

  const messageBubble = document.createElement("div");
  messageBubble.className = "message-bubble drawing-bubble";

  // Message info
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

  // Drawing image
  const drawingContainer = document.createElement("div");
  drawingContainer.className = "drawing-container";

  const img = document.createElement("img");
  img.className = "drawing-image";
  img.src = `data:image/png;base64,${imageData}`;
  img.alt = "Drawing";
  
  // Click để xem full size
  img.onclick = () => {
    const modal = document.createElement('div');
    modal.className = 'image-modal';
    modal.innerHTML = `
      <div class="image-modal-content">
        <span class="image-modal-close">&times;</span>
        <img src="${img.src}" alt="Drawing Full Size">
      </div>
    `;
    document.body.appendChild(modal);
    
    modal.querySelector('.image-modal-close').onclick = () => {
      modal.remove();
    };
    
    modal.onclick = (e) => {
      if (e.target === modal) {
        modal.remove();
      }
    };
  };

  drawingContainer.appendChild(img);
  messageBubble.appendChild(drawingContainer);
  messageDiv.appendChild(messageBubble);

  const messages = document.getElementById("messages");
  messages.appendChild(messageDiv);
  messages.scrollTop = messages.scrollHeight;
}

/**
 * Khởi tạo canvas và event listeners
 */
function initializeDrawingCanvas() {
  const canvas = document.getElementById('drawing-canvas');
  const ctx = canvas.getContext('2d');
  
  // Set canvas size
  canvas.width = 600;
  canvas.height = 400;
  
  // Fill white background
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  
  // Drawing settings
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';

  // Mouse events
  canvas.addEventListener('mousedown', startDrawing);
  canvas.addEventListener('mousemove', draw);
  canvas.addEventListener('mouseup', stopDrawing);
  canvas.addEventListener('mouseout', stopDrawing);

  // Touch events for mobile
  canvas.addEventListener('touchstart', handleTouchStart, { passive: false });
  canvas.addEventListener('touchmove', handleTouchMove, { passive: false });
  canvas.addEventListener('touchend', stopDrawing);
}

function startDrawing(e) {
  isDrawing = true;
  const rect = e.target.getBoundingClientRect();
  lastX = e.clientX - rect.left;
  lastY = e.clientY - rect.top;
}

function draw(e) {
  if (!isDrawing) return;
  
  const canvas = document.getElementById('drawing-canvas');
  const ctx = canvas.getContext('2d');
  const rect = canvas.getBoundingClientRect();
  
  const currentX = e.clientX - rect.left;
  const currentY = e.clientY - rect.top;

  ctx.strokeStyle = currentColor;
  ctx.lineWidth = lineWidth;
  
  ctx.beginPath();
  ctx.moveTo(lastX, lastY);
  ctx.lineTo(currentX, currentY);
  ctx.stroke();

  // Lưu nét vẽ
  drawingData.push({
    x1: lastX,
    y1: lastY,
    x2: currentX,
    y2: currentY,
    color: currentColor,
    width: lineWidth
  });

  lastX = currentX;
  lastY = currentY;
}

function stopDrawing() {
  isDrawing = false;
}

function handleTouchStart(e) {
  e.preventDefault();
  const touch = e.touches[0];
  const rect = e.target.getBoundingClientRect();
  lastX = touch.clientX - rect.left;
  lastY = touch.clientY - rect.top;
  isDrawing = true;
}

function handleTouchMove(e) {
  e.preventDefault();
  if (!isDrawing) return;
  
  const touch = e.touches[0];
  const canvas = document.getElementById('drawing-canvas');
  const ctx = canvas.getContext('2d');
  const rect = canvas.getBoundingClientRect();
  
  const currentX = touch.clientX - rect.left;
  const currentY = touch.clientY - rect.top;

  ctx.strokeStyle = currentColor;
  ctx.lineWidth = lineWidth;
  
  ctx.beginPath();
  ctx.moveTo(lastX, lastY);
  ctx.lineTo(currentX, currentY);
  ctx.stroke();

  drawingData.push({
    x1: lastX,
    y1: lastY,
    x2: currentX,
    y2: currentY,
    color: currentColor,
    width: lineWidth
  });

  lastX = currentX;
  lastY = currentY;
}

// Initialize khi DOM loaded
document.addEventListener('DOMContentLoaded', () => {
  initializeDrawingCanvas();
});