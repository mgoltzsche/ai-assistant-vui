/*export function float32ToInt8(float32Array) {
	const int8Array = new Int8Array(float32Array.length);
	for (let i = 0; i < float32Array.length; i++) {
		let sample = Math.max(-1, Math.min(1, float32Array[i]));
		int8Array[i] = Math.round(sample * 127);
	}
	return int8Array;
}*/

export function wavBlobFromFloat32Array(samples, sampleRate) {
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

	// HEADER
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

	for (let i = 0; i < samples.length; i++) {
		writer.pcm16s(samples[i]);
	}

	return new Blob([dataView.buffer], { type: 'application/octet-stream' });
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
