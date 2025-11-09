/**
 * Module xử lý kết nối WebTransport
 */

const baseUrl = "https://localhost:4433/chat";
let transport = null;
let name = "";
let isConnecting = false;

async function connect() {
  if (transport) return transport;
  if (isConnecting) return;

  isConnecting = true;
  updateConnectionStatus('connecting', 'Connecting...');

  try {
    transport = new WebTransport(`${baseUrl}?name=${encodeURIComponent(name)}`);
    await transport.ready;
    console.log("Connected to server");
    
    updateConnectionStatus('connected', 'Connected');
    showNotification('Successfully connected to chat!', 'success');

    transport.closed.then(() => {
      console.log("Connection closed.");
      updateUIOnDisconnect();
      showNotification('Disconnected from server', 'error');
    });

    handleIncomingStreams(); 
    readDatagrams();

    return transport;
  } catch (error) {
    console.error("Connection failed:", error);
    updateConnectionStatus('disconnected', 'Connection Failed');
    showNotification('Failed to connect to server', 'error');
    transport = null;
    throw error;
  } finally {
    isConnecting = false;
  }
}

async function handleIncomingStreams() {
  const reader = transport.incomingUnidirectionalStreams.getReader();
  
  const { value: persistentStream, done } = await reader.read();
  
  if (done) {
    console.log("No incoming stream received. Connection may be closing.");
    return;
  }
  
  readContinuousMessages(persistentStream); 
}

/**
 * Đọc datagram từ server (online list và file list)
 */
async function readDatagrams() {
  const datagramReader = transport.datagrams.readable.getReader();
  while (true) {
    const { value, done } = await datagramReader.read();
    if (done) break;
    try {
      const text = new TextDecoder().decode(value);
      console.log("Received datagram:", text);
      
      const msg = JSON.parse(text);
      
      if (msg.type === "online") {
        console.log("Online list update:", msg.clients);
        updateOnlineList(msg.clients);
      } else if (msg.type === "file_list") {
        console.log("File list update:", msg.files);
        updateAvailableFiles(msg.files);
      } else {
        console.log("Unknown datagram type:", msg.type);
      }
    } catch (err) {
      console.error("Invalid datagram:", err);
    }
  }
}