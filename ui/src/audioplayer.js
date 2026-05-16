// This player allows to play a raw PCM audio chunks.
// The latency from chunk submission to actual playback is very low.
// This is achieved by using a short buffer and keeping the audio device continuously active, playing silence if there's no chunk available.
export class RealTimeAudioPlayer {
  constructor({ sampleRate = 16000, channels = 1, bufferDuration = 0.05 } = {}) {
    this.SAMPLE_RATE = sampleRate;
    this.CHANNELS = channels;
    this.BUFFER_DURATION = bufferDuration;

    this.audioCtx = new AudioContext();
    this.startTime = this.audioCtx.currentTime;
    this.playbackQueue = [];
    this.schedulerRunning = false;
  }

  start() {
    if (!this.schedulerRunning) {
    	  this.schedulerRunning = true;
      this.schedulerLoop();
    }
  }

  playChunk(chunk) {
	const buffer = this.decodePCM(chunk);
    this.playbackQueue.push(buffer);
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
    if (this.schedulerRunning) {
      setTimeout(() => this.schedulerLoop(), 10);
    }
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

  stop() {
	this.schedulerRunning = false;
    this.playbackQueue = [];
  }
}
