// This player allows to play a raw PCM audio stream from a given URL.
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
	}

	async start(url) {
		const response = await fetch(url, {
			headers: {
				'Accept': 'audio/x-raw',
				'X-Buffer-Duration-Ms': this.BUFFER_DURATION * 1000
			}
		});
		const reader = response.body.getReader();

		const BYTES_PER_SAMPLE = 2;
		const SAMPLES_PER_CHUNK = this.SAMPLE_RATE * this.BUFFER_DURATION;
		const CHUNK_SIZE = SAMPLES_PER_CHUNK * BYTES_PER_SAMPLE;

		let leftover = new Uint8Array(0);

		if (!this.schedulerRunning) this.schedulerLoop();

		while (true) {
			if (this.getQueueDuration() > this.MAX_BUFFERED_SEC) {
				await this.waitUntil(() => this.getQueueDuration() < this.MAX_BUFFERED_SEC * 0.8);
			}

			const { value, done } = await reader.read();
			if (done) break;

			const combined = new Uint8Array(leftover.length + value.length);
			combined.set(leftover);
			combined.set(value, leftover.length);

			const totalSamples = Math.floor(combined.length / BYTES_PER_SAMPLE);
			const usableSamples = totalSamples - (totalSamples % SAMPLES_PER_CHUNK);
			const usableBytes = usableSamples * BYTES_PER_SAMPLE;

			for (let offset = 0; offset < usableBytes; offset += CHUNK_SIZE) {
				const chunk = combined.slice(offset, offset + CHUNK_SIZE);
				const buffer = this.decodePCM(chunk);
				this.playbackQueue.push(buffer);
			}

			leftover = combined.slice(usableBytes);
		}
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
}
