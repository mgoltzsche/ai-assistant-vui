import './style.css'
import { MicVAD } from "@ricky0123/vad-web"
import { wavBlobFromFloat32Array } from './audioconversion.js'
import { PCMStreamPlayer } from './player.js'

try {
	if (!navigator.mediaDevices) {
		throw 'Please use the app via HTTPS in order to enable microphone support!'
	}

	const myvad = await MicVAD.new({
		onSpeechEnd: (audioSamples) => {
			console.log("voice detected")
			fetch('/channels/default/audio', {
				method: 'POST',
				body: wavBlobFromFloat32Array(audioSamples, 16000)
			});
		},
	})
	myvad.start()

	const player = new PCMStreamPlayer()

	player.start(`wss://${window.location.host}/channels/default/audio?buffer-ms=50`);
} catch(e) {
	const errorEl = document.querySelector('#error');
	errorEl.innerHTML = e.toString()
}
