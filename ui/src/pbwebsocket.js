import { chat } from "./gen/types.js";

export class PbWebsocket {
  constructor({ url, player } = {}) {
    this.url = url;
    this.player = player;
    this.ws = null;
    this.reconnectDelay = 1000;
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

      this.ws.onopen = () => {
        console.log("WebSocket connected");
      };

      this.ws.onmessage = (event) => {
        const msg = chat.Message.decode(new Uint8Array(event.data));
        if (msg.audioMessage) {
        	  this.player.playRiffWave(msg.audioMessage);
        }
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

  send(data) {
    if (!(data instanceof Uint8Array)) {
      throw new Error("send(buf) expects an ArrayBuffer but got "+data);
    }
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      const msg = chat.Message.create({'audioMessage': data});
      this.ws.send(chat.Message.encode(msg).finish());
    } else {
      console.warn("WebSocket is not open. Unable to send PCM data.");
    }
  }

  stop() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    if (this.player) {
    	  this.player.stop();
	  this.player = null;
    }
  }
}

