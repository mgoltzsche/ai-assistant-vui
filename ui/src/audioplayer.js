import { parseWaveFile } from './riffwave.js'

// This player allows to play a raw PCM audio chunks.
// The latency from chunk submission to actual playback is very low.
// This is achieved by using a short buffer and keeping the audio device continuously active, playing silence if there's no chunk available.
export class RealTimeAudioPlayer {
  constructor({ sampleRate = 16000, channels = 1, bufferDuration = 0.05 } = {}) {
    this.CHANNELS = channels;
    this.BUFFER_DURATION = bufferDuration;

    this.audioCtx = new AudioContext({sampleRate: sampleRate});
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

  playRiffWave(bytes) {
    // The browser is natively resampling the provided samples to AudioContext.sampleRate
    const wav = parseWaveFile(bytes)

    for (let buffer of audioBufferChunksFromSamples(wav.samples, wav.sampleRate, wav.numChannels, this.audioCtx, this.BUFFER_DURATION)) {
      this.playbackQueue.push(buffer);
    }
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
	const sampleRate = this.audioCtx.sampleRate;
    const samples = duration * sampleRate;
    return this.audioCtx.createBuffer(this.CHANNELS, samples, sampleRate);
  }

  stop() {
	this.schedulerRunning = false;
    this.playbackQueue = [];
  }
}

function* audioBufferChunksFromSamples(samples, sampleRate, numChannels, audioContext, bufferDurationSeconds) {
  const framesPerBuffer = Math.floor(sampleRate * bufferDurationSeconds);
  const samplesPerBuffer = framesPerBuffer * numChannels;

  let offset = 0;

  while (offset < samples.length) {
    const remainingSamples = samples.length - offset;
    const currentSamples = Math.min(samplesPerBuffer, remainingSamples);
    const currentFrames = Math.floor(currentSamples / numChannels);
    const audioBuffer = audioContext.createBuffer(numChannels, currentFrames, sampleRate);

    for (let channel = 0; channel < numChannels; channel++) {
      const channelData = audioBuffer.getChannelData(channel);

      for (let frame = 0; frame < currentFrames; frame++) {
        const sampleIndex = offset + frame * numChannels + channel;

        let sample = samples[sampleIndex];

        // Convert integer PCM -> float [-1, 1]
        if (samples instanceof Int16Array) {
          sample /= 32768;
        } else if (samples instanceof Int32Array) {
          sample /= 2147483648;
        } else if (samples instanceof Uint8Array) {
          sample = (sample - 128) / 128;
        }

        channelData[frame] = sample;
      }
    }

    yield audioBuffer;

    offset += currentSamples;
  }
}
