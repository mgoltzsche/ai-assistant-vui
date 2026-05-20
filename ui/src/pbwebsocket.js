import { chat } from "./gen/types.js";

export class PbWebsocket {
  constructor(url) {
    this.url = url;
    this.ws = null;
    this.reconnectDelay = 1000;
    this._onmessage = (msg) => {}
  }

  set onmessage(handler) {
	this._onmessage = handler;
  }

  start() {
    this.connect();
  }

  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return; // Prevent concurrent reconnect attempts
    }

    try {
      console.log(`connecting WebSocket to ${this.url}`)

      this.ws = new WebSocket(this.url);
      this.ws.binaryType = 'arraybuffer';
      this.ws.onmessage = this._onmessage;
      this.ws.onmessage = (event) => {
        const msg = chat.Message.decode(new Uint8Array(event.data));
        console.log('received websocket message:', msg);
        this._onmessage(msg);
      };

      this.ws.onopen = () => {
        console.log("WebSocket connected");
      };

      this.ws.onerror = (err) => {
        console.warn("WebSocket error", err);
        this.reconnect();
      };

      this.ws.onclose = () => {
        console.warn("WebSocket closed, reconnecting");
        this.reconnect();
      };

    } catch (err) {
      console.warn("WebSocket connection failed", err);
      this.reconnect();
    }
  }

  reconnect() {
    setTimeout(() => this.connect(), this.reconnectDelay);
  }

  send(msg) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(chat.Message.encode(msg).finish());
    } else {
      console.warn("WebSocket is not open. Unable to send message.");
    }
  }

  stop() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}

