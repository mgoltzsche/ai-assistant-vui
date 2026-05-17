export function riffWaveFromFloat32Array(samples, sampleRate) {
	// See https://devtails.xyz/@adam/how-to-write-a-wav-file-in-javascript
	const numChannels = 1;
	const bytesPerSample = 2 * numChannels;
	const bytesPerSecond = sampleRate * bytesPerSample;
	const dataLength = samples.length * bytesPerSample;
	const headerLength = 44;
	const fileLength = dataLength + headerLength;
	const bufferData = new Uint8Array(fileLength);
	const dataView = new DataView(bufferData.buffer);
	const writer = createWriter(dataView);

	// Generate header
	writer.string("RIFF");
	// File Size
	writer.uint32(fileLength);
	writer.string("WAVE");

	writer.string("fmt ");
	// Chunk Size
	writer.uint32(16);
	// Format Tag
	writer.uint16(1);
	// Number Channels
	writer.uint16(numChannels);
	// Sample Rate
	writer.uint32(sampleRate);
	// Bytes Per Second
	writer.uint32(bytesPerSecond);
	// Bytes Per Sample
	writer.uint16(bytesPerSample);
	// Bits Per Sample
	writer.uint16(bytesPerSample * 8);
	writer.string("data");
	writer.uint32(dataLength);

	// Copy samples
	for (let i = 0; i < samples.length; i++) {
		writer.pcm16s(samples[i]);
	}

	return new Uint8Array(dataView.buffer)
}

function createWriter(dataView) {
  let pos = 0;

  return {
    string(val) {
      for (let i = 0; i < val.length; i++) {
        dataView.setUint8(pos++, val.charCodeAt(i));
      }
    },
    uint16(val) {
      dataView.setUint16(pos, val, true);
      pos += 2;
    },
    uint32(val) {
      dataView.setUint32(pos, val, true);
      pos += 4;
    },
    pcm16s: function(value) {
      value = Math.round(value * 32768);
      value = Math.max(-32768, Math.min(value, 32767));
      dataView.setInt16(pos, value, true);
      pos += 2;
    },
  }
}

/**
 * Parse a RIFF/WAVE file from a Uint8Array or ArrayBuffer.
 *
 * Supports:
 * - PCM
 * - IEEE float
 * - mono/stereo/multichannel
 *
 * Returns:
 * {
 *   audioFormat,
 *   numChannels,
 *   sampleRate,
 *   byteRate,
 *   blockAlign,
 *   bitsPerSample,
 *   dataOffset,
 *   dataSize,
 *   samples
 * }
 */
export function parseWaveFile(bytes) {
  const view = new DataView(
    bytes.buffer,
    bytes.byteOffset,
    bytes.byteLength
  );

  function readString(offset, length) {
    let s = "";
    for (let i = 0; i < length; i++) {
      s += String.fromCharCode(view.getUint8(offset + i));
    }
    return s;
  }

  // RIFF header
  const riff = readString(0, 4);
  if (riff !== "RIFF") {
    throw new Error("Invalid RIFF header");
  }

  const wave = readString(8, 4);
  if (wave !== "WAVE") {
    throw new Error("Invalid WAVE header");
  }

  let offset = 12;

  let fmt = null;
  let dataOffset = null;
  let dataSize = null;

  // Iterate chunks
  while (offset < view.byteLength) {
    const chunkId = readString(offset, 4);
    const chunkSize = view.getUint32(offset + 4, true);

    if (chunkId === "fmt ") {
      fmt = {
        audioFormat: view.getUint16(offset + 8, true),
        numChannels: view.getUint16(offset + 10, true),
        sampleRate: view.getUint32(offset + 12, true),
        byteRate: view.getUint32(offset + 16, true),
        blockAlign: view.getUint16(offset + 20, true),
        bitsPerSample: view.getUint16(offset + 22, true),
      };
    } else if (chunkId === "data") {
      dataOffset = offset + 8;
      dataSize = chunkSize;
    }

    // Chunks are word aligned
    offset += 8 + chunkSize + (chunkSize % 2);
  }

  if (!fmt) {
    throw new Error("Missing fmt chunk");
  }

  if (dataOffset == null) {
    throw new Error("Missing data chunk");
  }

  const {
    audioFormat,
    numChannels,
    sampleRate,
    byteRate,
    blockAlign,
    bitsPerSample,
  } = fmt;

  const bytesPerSample = bitsPerSample / 8;
  const totalSamples = dataSize / bytesPerSample;

  let samples;

  // PCM integer
  if (audioFormat === 1) {
    switch (bitsPerSample) {
      case 8: {
        samples = new Uint8Array(bytes.buffer, bytes.byteOffset + dataOffset, totalSamples);
        break;
      }

      case 16: {
        samples = new Int16Array(totalSamples);
        for (let i = 0; i < totalSamples; i++) {
          samples[i] = view.getInt16(dataOffset + i * 2, true);
        }
        break;
      }

      case 24: {
        samples = new Int32Array(totalSamples);

        for (let i = 0; i < totalSamples; i++) {
          const pos = dataOffset + i * 3;

          let value =
            (view.getUint8(pos)) |
            (view.getUint8(pos + 1) << 8) |
            (view.getUint8(pos + 2) << 16);

          // sign extend
          if (value & 0x800000) {
            value |= 0xff000000;
          }

          samples[i] = value;
        }

        break;
      }

      case 32: {
        samples = new Int32Array(totalSamples);

        for (let i = 0; i < totalSamples; i++) {
          samples[i] = view.getInt32(dataOffset + i * 4, true);
        }

        break;
      }

      default:
        throw new Error(`Unsupported PCM bit depth: ${bitsPerSample}`);
    }
  }

  // IEEE float
  else if (audioFormat === 3) {
    if (bitsPerSample !== 32) {
      throw new Error("Unsupported float bit depth");
    }

    samples = new Float32Array(totalSamples);

    for (let i = 0; i < totalSamples; i++) {
      samples[i] = view.getFloat32(dataOffset + i * 4, true);
    }
  } else {
    throw new Error(`Unsupported audio format: ${audioFormat}`);
  }

  return {
    audioFormat,
    numChannels,
    sampleRate,
    byteRate,
    blockAlign,
    bitsPerSample,
    dataOffset,
    dataSize,
    samples,
  };
}
