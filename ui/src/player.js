// This player allows to play a raw PCM audio stream from a given websocket URL.
// Once started, it plays with very low latency, given that the server sends
// sufficient zero samples after each speech to fill its small buffer,
// making it play immmediately.
export class PCMStreamPlayer {
	constructor({ sampleRate = 16000, channels = 1, bufferDuration = 0.05, maxBufferedSec = 2 } = {}) {
		this.SAMPLE_RATE = sampleRate;
		this.CHANNELS = channels;
		this.BUFFER_DURATION = bufferDuration;
		this.MAX_BUFFERED_SEC = maxBufferedSec;

		this.audioCtx = new AudioContext();
		this.startTime = this.audioCtx.currentTime;
		this.playbackQueue = [];
		this.schedulerRunning = false;
		this.ws = null;
		this.url = null;
		this.reconnectDelay = 1000;
		this.leftover = new Uint8Array(0);
	}

	start(url) {
		this.url = url;
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
				if (!this.schedulerRunning) this.schedulerLoop();
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
					const buffer = this.decodePCM(chunk);
					this.playbackQueue.push(buffer);
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

	decodePCM(pcmChunk) {
		const samples = new Int16Array(pcmChunk.buffer);
		const audioBuffer = this.audioCtx.createBuffer(this.CHANNELS, samples.length, this.SAMPLE_RATE);
		const floatBuffer = audioBuffer.getChannelData(0);

		for (let i = 0; i < samples.length; i++) {
			floatBuffer[i] = samples[i] / 32768;
		}
		return audioBuffer;
	}

	schedulerLoop() {
		this.schedulerRunning = true;
		const SCHEDULE_AHEAD_SEC = 0.5;
		const MIN_BUFFER_SEC = 0.2;

		const timeAhead = this.startTime - this.audioCtx.currentTime;

		if (timeAhead < SCHEDULE_AHEAD_SEC) {
			if (this.playbackQueue.length > 0) {
				const buffer = this.playbackQueue.shift();
				this.playBuffer(buffer);
			} else if (timeAhead < MIN_BUFFER_SEC) {
				const silence = this.createSilenceBuffer(this.BUFFER_DURATION);
				this.playBuffer(silence);
			}
		}
		setTimeout(() => this.schedulerLoop(), 10);
	}

	playBuffer(buffer) {
		const source = this.audioCtx.createBufferSource();
		source.buffer = buffer;
		source.connect(this.audioCtx.destination);

		const playAt = Math.max(this.audioCtx.currentTime, this.startTime);
		source.start(playAt);
		this.startTime = playAt + buffer.duration;
	}

	createSilenceBuffer(duration) {
		const samples = duration * this.SAMPLE_RATE;
		return this.audioCtx.createBuffer(this.CHANNELS, samples, this.SAMPLE_RATE);
	}

	getQueueDuration() {
		return this.playbackQueue.reduce((sum, buf) => sum + buf.duration, 0);
	}

	waitUntil(predicate) {
		return new Promise(resolve => {
			const check = () => predicate() ? resolve() : setTimeout(check, 50);
			check();
		});
	}

	send(buf) {
		if (!(buf instanceof ArrayBuffer || ArrayBuffer.isView(buf))) {
			throw new Error("send(buf) expects an ArrayBuffer or TypedArray but got "+buf);
		}
		if (this.ws && this.ws.readyState === WebSocket.OPEN) {
			const data = buf instanceof ArrayBuffer ? buf : buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
			this.ws.send(data);
		} else {
			console.warn("WebSocket is not open. Unable to send PCM data.");
		}
	}
}
