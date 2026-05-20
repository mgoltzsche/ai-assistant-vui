import { chat } from "./gen/types.js";

let chatHistory = [];

const messageInput = document.getElementById('message-input');
const sendButton = document.getElementById('send-button');
const chatHistoryElement = document.getElementById('chat-history');

function addMessageToChat(role, text) {
  const messageDiv = document.createElement('div');
  messageDiv.className = `message ${role}`;
  messageDiv.innerHTML = `<strong>${role}:</strong> ${text}`;
  chatHistoryElement.appendChild(messageDiv);
  chatHistoryElement.scrollTop = chatHistoryElement.scrollHeight;
}

function sendTextMessage(text, websocket) {
  if (!text.trim()) return;

  websocket.send({'textMessage': text});

  messageInput.value = '';
}

function setupUIEventHandlers(websocket) {
  sendButton.addEventListener('click', () => {
    sendTextMessage(messageInput.value, websocket);
  });
  messageInput.addEventListener('keypress', (e) => {
    if (e.key === 'Enter') {
      sendTextMessage(messageInput.value, websocket);
    }
  });
}

function setupWebsocketMessageHandling(websocket, player) {
  websocket.onmessage = (msg) => {
    if (msg.role === 1 && msg.audioMessage) {
    	  player.playRiffWave(msg.audioMessage);
    }
    if (msg.textMessage) {
      addMessageToChat(chat.Role[msg.role], msg.textMessage);
    }
  };
}

export function setupUI(websocket, player) {
  setupUIEventHandlers(websocket);
  setupWebsocketMessageHandling(websocket, player);
}
