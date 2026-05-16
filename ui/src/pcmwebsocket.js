export class PCMWebsocket {
  constructor({ url, player, sampleRate = 16000, channels = 1, bufferDuration = 0.05, maxBufferedSec = 2 } = {}) {
    this.SAMPLE_RATE = sampleRate;
    this.CHANNELS = channels;
    this.BUFFER_DURATION = bufferDuration;
    this.MAX_BUFFERED_SEC = maxBufferedSec;

    this.ws = null;
    this.url = url;
    this.player = player;
    this.reconnectDelay = 1000;
    this.leftover = new Uint8Array(0);
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

      const BYTES_PER_SAMPLE = 2;
      const SAMPLES_PER_CHUNK = this.SAMPLE_RATE * this.BUFFER_DURATION;
      const CHUNK_SIZE = SAMPLES_PER_CHUNK * BYTES_PER_SAMPLE;

      this.ws.onopen = () => {
        console.log("WebSocket connected");
      };

      this.ws.onmessage = (event) => {
        const value = new Uint8Array(event.data);

        const combined = new Uint8Array(this.leftover.length + value.length);
        combined.set(this.leftover);
        combined.set(value, this.leftover.length);

        const totalSamples = Math.floor(combined.length / BYTES_PER_SAMPLE);
        const usableSamples = totalSamples - (totalSamples % SAMPLES_PER_CHUNK);
        const usableBytes = usableSamples * BYTES_PER_SAMPLE;

        for (let offset = 0; offset < usableBytes; offset += CHUNK_SIZE) {
          const chunk = combined.slice(offset, offset + CHUNK_SIZE);
          this.player.playChunk(chunk);
        }

        this.leftover = combined.slice(usableBytes);
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
    if (!(data instanceof ArrayBuffer || ArrayBuffer.isView(data))) {
      throw new Error("send(buf) expects an ArrayBuffer or TypedArray but got "+data);
    }
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      const buf = data instanceof ArrayBuffer ? data : data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength);
      this.ws.send(buf);
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
