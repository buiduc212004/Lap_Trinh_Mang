/**
 * Module xử lý gửi và nhận file với multi-stream (parallel transfers - optimized)
 */

let availableFiles = [];
const NUM_STREAMS = 8; // Số stream song song
const CHUNK_SIZE = 256 * 1024; // 256KB cho mỗi lần gửi (client-side chunking)

/**
 * Tính toán SHA-256 Hash của file
 */
async function calculateFileHash(file) {
  const buffer = await file.arrayBuffer();
  const hashBuffer = await crypto.subtle.digest('SHA-256', buffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
}

/**
 * Upload file lên server với multi-stream (parallel upload - optimized)
 */
async function uploadFile(file) {
  if (!transport) {
    showNotification('Not connected to server!', 'error');
    return;
  }

  if (file.size > 100 * 1024 * 1024) {
    showNotification('File too large! Maximum 100MB', 'error');
    return;
  }

  const uploadBtn = document.getElementById('file-upload-btn');
  const progressContainer = document.getElementById('upload-progress');
  const progressBar = document.getElementById('upload-progress-bar');
  const progressText = document.getElementById('upload-progress-text');

  try {
    uploadBtn.style.display = 'none';
    progressContainer.style.display = 'block';
    progressText.textContent = 'Calculating hash...';
    progressBar.style.width = '10%';

    const fileHash = await calculateFileHash(file);

    progressText.textContent = `Uploading ${file.name} with ${NUM_STREAMS} streams...`;
    progressBar.style.width = '20%';

    // Chia file thành NUM_STREAMS chunks
    const chunkSize = Math.ceil(file.size / NUM_STREAMS);
    const chunks = [];
    for (let i = 0; i < NUM_STREAMS; i++) {
      const start = i * chunkSize;
      const end = Math.min(start + chunkSize, file.size);
      chunks.push({ index: i, start, end, blob: file.slice(start, end) });
    }

    let uploadedBytes = 0;
    const startTime = performance.now();
    const bytesLock = { value: 0 };
    const speedSamples = [];
    let lastUpdate = startTime;

    // Upload các chunks song song
    const uploadPromises = chunks.map(async (chunk) => {
      const stream = await transport.createBidirectionalStream();
      const writer = stream.writable.getWriter();
      const encoder = new TextEncoder();

      // Gửi header
      const header = JSON.stringify({
        op: "upload",
        filename: file.name,
        size: chunk.end - chunk.start,
        chunk_index: chunk.index,
        chunk_start: chunk.start,
        chunk_end: chunk.end
      }) + "\n";
      await writer.write(encoder.encode(header));

      // Stream chunk data với buffer lớn hơn
      const reader = chunk.blob.stream().getReader();
      const buffer = [];
      let bufferSize = 0;
      const BUFFER_THRESHOLD = CHUNK_SIZE * 4; // 1MB buffer

      while (true) {
        const { value, done } = await reader.read();
        if (done) {
          // Flush remaining buffer
          if (buffer.length > 0) {
            const merged = new Uint8Array(bufferSize);
            let offset = 0;
            for (const chunk of buffer) {
              merged.set(chunk, offset);
              offset += chunk.length;
            }
            await writer.write(merged);
            bytesLock.value += merged.length;
          }
          break;
        }

        buffer.push(value);
        bufferSize += value.length;

        // Flush buffer khi đạt threshold
        if (bufferSize >= BUFFER_THRESHOLD) {
          const merged = new Uint8Array(bufferSize);
          let offset = 0;
          for (const chunk of buffer) {
            merged.set(chunk, offset);
            offset += chunk.length;
          }
          await writer.write(merged);
          bytesLock.value += merged.length;

          buffer.length = 0;
          bufferSize = 0;

          // Update progress (throttled)
          const now = performance.now();
          if (now - lastUpdate >= 100) { // Update mỗi 100ms
            const percent = 20 + (bytesLock.value / file.size) * 70;
            const elapsed = (now - startTime) / 1000;
            const speed = (bytesLock.value / (1024 * 1024)) / (elapsed || 1);

            // Lưu speed samples để tính trung bình mượt hơn
            speedSamples.push(speed);
            if (speedSamples.length > 10) speedSamples.shift();
            const avgSpeed = speedSamples.reduce((a, b) => a + b, 0) / speedSamples.length;

            progressBar.style.width = `${percent.toFixed(1)}%`;
            progressText.textContent = `Uploading... ${avgSpeed.toFixed(2)} MB/s (${NUM_STREAMS} streams)`;
            lastUpdate = now;
          }
        }
      }

      await writer.close();

      // Đọc response
      const result = await readJSONResponse(stream.readable);
      if (result.status !== "ok") {
        throw new Error(`Chunk ${chunk.index} failed: ${result.error}`);
      }

      console.log(`Chunk ${chunk.index} uploaded successfully`);
      return result;
    });

    // Đợi tất cả chunks upload xong
    await Promise.all(uploadPromises);

    progressBar.style.width = '90%';
    progressText.textContent = 'Merging chunks on server...';

    // Gửi yêu cầu merge
    const mergeStream = await transport.createBidirectionalStream();
    const mergeWriter = mergeStream.writable.getWriter();
    const encoder = new TextEncoder();

    const mergeHeader = JSON.stringify({
      op: "merge",
      filename: file.name,
      hash: fileHash
    }) + "\n";
    await mergeWriter.write(encoder.encode(mergeHeader));
    await mergeWriter.close();

    const mergeResult = await readJSONResponse(mergeStream.readable);

    if (mergeResult.status === "ok") {
      progressBar.style.width = '100%';
      const totalTime = (performance.now() - startTime) / 1000;
      const avgSpeed = (file.size / (1024 * 1024)) / totalTime;
      progressText.textContent = `Upload successful! (${avgSpeed.toFixed(2)} MB/s)`;
      showNotification(`File uploaded: ${file.name}`, 'success');
    } else {
      throw new Error(`Merge failed: ${mergeResult.error}`);
    }

    setTimeout(() => {
      progressContainer.style.opacity = 0;
      setTimeout(() => {
        progressContainer.style.display = 'none';
        progressContainer.style.opacity = 1;
        progressBar.style.width = '0%';
      }, 400);
    }, 1200);

  } catch (e) {
    progressContainer.style.display = 'none';
    progressBar.style.width = '0%';
    showNotification(`Upload failed: ${e.message}`, 'error');
    console.error("Upload error:", e);
  } finally {
    uploadBtn.disabled = false;
    uploadBtn.dataset.uploading = 'false';
  }
}

/**
 * Download file từ server với multi-stream (parallel download - optimized)
 */
async function downloadFile(filename) {
  if (!transport) {
    showNotification('Not connected to server!', 'error');
    return;
  }

  try {
    showNotification(`Downloading ${filename} with ${NUM_STREAMS} streams...`, 'info');

    const bar = document.getElementById('upload-progress-bar');
    const text = document.getElementById('upload-progress-text');
    const container = document.getElementById('upload-progress');
    container.style.display = 'block';
    bar.style.width = '0%';
    text.textContent = 'Getting file info...';

    // 1. Lấy metadata file (size, num_streams)
    const metaStream = await transport.createBidirectionalStream();
    const metaWriter = metaStream.writable.getWriter();
    const encoder = new TextEncoder();

    const metaHeader = JSON.stringify({
      op: "download",
      filename: filename,
      chunk_index: -1 // Request metadata
    }) + "\n";
    await metaWriter.write(encoder.encode(metaHeader));
    await metaWriter.close();

    const metadata = await readJSONResponse(metaStream.readable);
    if (metadata.status !== "ok") {
      throw new Error(metadata.error || "Failed to get file metadata");
    }

    const fileSize = metadata.size;
    const numStreams = metadata.num_streams || NUM_STREAMS;
    console.log(`Downloading ${filename}: ${fileSize} bytes with ${numStreams} streams`);

    text.textContent = `Downloading with ${numStreams} streams...`;
    bar.style.width = '10%';

    // 2. Chia file thành chunks
    const chunkSize = Math.ceil(fileSize / numStreams);
    const chunks = [];
    for (let i = 0; i < numStreams; i++) {
      const start = i * chunkSize;
      const end = Math.min(start + chunkSize, fileSize);
      chunks.push({ index: i, start, end, data: [] });
    }

    let receivedBytes = 0;
    const startTime = performance.now();
    const bytesLock = { value: 0 };
    const speedSamples = [];
    let lastUpdate = startTime;

    // 3. Download các chunks song song
    const downloadPromises = chunks.map(async (chunk) => {
      const stream = await transport.createBidirectionalStream();
      const writer = stream.writable.getWriter();

      // Gửi request cho chunk cụ thể
      const header = JSON.stringify({
        op: "download",
        filename: filename,
        chunk_index: chunk.index,
        chunk_start: chunk.start,
        chunk_end: chunk.end
      }) + "\n";
      await writer.write(encoder.encode(header));
      await writer.close();

      // Đọc data với buffering
      const reader = stream.readable.getReader();
      const buffer = [];
      let bufferSize = 0;
      const BUFFER_THRESHOLD = 1024 * 1024; // 1MB buffer

      while (true) {
        const { value, done } = await reader.read();
        if (done) {

          if (buffer.length > 0) {
            const merged = new Uint8Array(bufferSize);
            let offset = 0;
            for (const part of buffer) {
              merged.set(part, offset);
              offset += part.length;
            }
            chunk.data.push(merged);
          }
          break;
        }

        buffer.push(value);
        bufferSize += value.length;
        bytesLock.value += value.length;

        // Merge buffer khi đạt threshold
        if (bufferSize >= BUFFER_THRESHOLD) {
          const merged = new Uint8Array(bufferSize);
          let offset = 0;
          for (const part of buffer) {
            merged.set(part, offset);
            offset += part.length;
          }
          chunk.data.push(merged);
          buffer.length = 0;
          bufferSize = 0;
        }

        // Update progress (throttled)
        const now = performance.now();
        if (now - lastUpdate >= 100) { // Update mỗi 100ms
          const percent = 10 + (bytesLock.value / fileSize) * 85;
          const elapsed = (now - startTime) / 1000;
          const speed = (bytesLock.value / (1024 * 1024)) / (elapsed || 1);

          speedSamples.push(speed);
          if (speedSamples.length > 10) speedSamples.shift();
          const avgSpeed = speedSamples.reduce((a, b) => a + b, 0) / speedSamples.length;

          bar.style.width = `${percent.toFixed(1)}%`;
          text.textContent = `Downloading... ${avgSpeed.toFixed(2)} MB/s (${numStreams} streams)`;
          lastUpdate = now;
        }
      }

      console.log(`Chunk ${chunk.index} downloaded: ${chunk.data.reduce((sum, arr) => sum + arr.length, 0)} bytes`);
    });

    await Promise.all(downloadPromises);

    bar.style.width = '95%';
    text.textContent = 'Merging chunks...';

    // 4. Merge chunks theo thứ tự
    const sortedChunks = chunks.sort((a, b) => a.index - b.index);
    const allChunks = [];
    for (const chunk of sortedChunks) {
      allChunks.push(...chunk.data);
    }

    // 5. Tạo blob và download
    const blob = new Blob(allChunks);
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = filename;
    document.body.appendChild(a);
    a.click();

    setTimeout(() => URL.revokeObjectURL(a.href), 500);
    document.body.removeChild(a);

    const duration = (performance.now() - startTime) / 1000;
    const avgSpeed = (fileSize / (1024 * 1024)) / duration;

    bar.style.width = '100%';
    text.textContent = `Download complete! (${avgSpeed.toFixed(2)} MB/s)`;
    showNotification(`Downloaded: ${filename} (${avgSpeed.toFixed(2)} MB/s)`, 'success');

    setTimeout(() => {
      container.style.opacity = 0;
      setTimeout(() => {
        container.style.display = 'none';
        container.style.opacity = 1;
        bar.style.width = '0%';
      }, 400);
    }, 1000);

  } catch (e) {
    showNotification(`Download failed: ${e.message}`, 'error');
    console.error("Download error:", e);
  }
}

/**
 * Đọc JSON response từ stream
 */
async function readJSONResponse(readable) {
  const reader = readable.getReader();
  const decoder = new TextDecoder();
  let jsonBuffer = '';

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    jsonBuffer += decoder.decode(value, { stream: true });

    let newlineIndex;
    while ((newlineIndex = jsonBuffer.indexOf('\n')) !== -1) {
      const line = jsonBuffer.slice(0, newlineIndex);
      try {
        return JSON.parse(line);
      } catch (err) {
        console.warn("Invalid JSON line, skipping:", line);
      }
      jsonBuffer = jsonBuffer.slice(newlineIndex + 1);
    }
  }

  throw new Error("No valid JSON response from server");
}

/**
 * UI Functions
 */

function updateAvailableFiles(files) {
  console.log("Received file list:", files);
  if (Array.isArray(files)) {
    availableFiles = files.map(f => typeof f === 'string' ? { name: f, size: 0 } : f);
  } else {
    availableFiles = [];
  }
  console.log("Processed file list:", availableFiles);
  renderFileList();
}

function renderFileList() {
  const fileListEl = document.getElementById('available-files-list');
  if (!fileListEl) return;

  if (availableFiles.length === 0) {
    fileListEl.innerHTML = `
      <div class="has-text-centered has-text-grey" style="padding: 2rem;">
        <i class="fas fa-folder-open fa-2x" style="opacity: 0.3;"></i>
        <p style="margin-top: 10px;">No files available</p>
      </div>`;
    return;
  }

  fileListEl.innerHTML = '';
  availableFiles.forEach(file => {
    const fileDiv = document.createElement('div');
    fileDiv.className = 'file-item';

    const fileIcon = document.createElement('div');
    fileIcon.className = 'file-icon';
    fileIcon.innerHTML = getFileIcon(file.name);

    const fileInfo = document.createElement('div');
    fileInfo.className = 'file-info';

    const fileName = document.createElement('div');
    fileName.className = 'file-name';
    fileName.textContent = file.name;
    fileName.title = file.name;

    const fileSize = document.createElement('div');
    fileSize.className = 'file-size';
    fileSize.textContent = formatFileSize(file.size);

    fileInfo.appendChild(fileName);
    fileInfo.appendChild(fileSize);

    const downloadBtn = document.createElement('button');
    downloadBtn.className = 'file-download-btn';
    downloadBtn.innerHTML = '<i class="fas fa-download"></i>';
    downloadBtn.title = 'Download';
    downloadBtn.onclick = () => downloadFile(file.name);

    fileDiv.appendChild(fileIcon);
    fileDiv.appendChild(fileInfo);
    fileDiv.appendChild(downloadBtn);
    fileListEl.appendChild(fileDiv);
  });
}

function getFileIcon(filename) {
  const ext = filename.split('.').pop().toLowerCase();
  const map = {
    pdf: 'fa-file-pdf', doc: 'fa-file-word', docx: 'fa-file-word',
    xls: 'fa-file-excel', xlsx: 'fa-file-excel',
    ppt: 'fa-file-powerpoint', pptx: 'fa-file-powerpoint',
    jpg: 'fa-file-image', jpeg: 'fa-file-image', png: 'fa-file-image', gif: 'fa-file-image',
    zip: 'fa-file-archive', rar: 'fa-file-archive',
    txt: 'fa-file-alt', mp3: 'fa-file-audio', mp4: 'fa-file-video'
  };
  const icon = map[ext] || 'fa-file';
  return `<i class="fas ${icon}"></i>`;
}

function formatFileSize(bytes) {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

function handleFileSelect() {
  const fileInput = document.getElementById('file-input');
  const file = fileInput.files[0];
  if (!file) return;

  const selectedFileInfo = document.getElementById('selected-file-info');
  selectedFileInfo.innerHTML = `
    <div class="selected-file">
      <i class="fas fa-file"></i>
      <span>${file.name} (${formatFileSize(file.size)})</span>
      <button onclick="clearFileSelection()" class="clear-btn">
        <i class="fas fa-times"></i>
      </button>
    </div>`;
  selectedFileInfo.style.display = 'block';
  
  const uploadBtn = document.getElementById('file-upload-btn');
  uploadBtn.style.display = 'inline-flex';
}

function clearFileSelection() {
  const fileInput = document.getElementById('file-input');
  fileInput.value = '';
  
  const selectedFileInfo = document.getElementById('selected-file-info');
  selectedFileInfo.style.display = 'none';
  selectedFileInfo.innerHTML = '';
  
  const uploadBtn = document.getElementById('file-upload-btn');
  uploadBtn.style.display = 'none';
}

async function handleFileUpload(event) {
  if (event) {
    event.preventDefault();
    event.stopPropagation();
  }
  
  const fileInput = document.getElementById('file-input');
  const file = fileInput.files[0];
  
  if (!file) {
    showNotification('Please select a file first!', 'error');
    return false;
  }

  await uploadFile(file);
  clearFileSelection();
  return false;
}