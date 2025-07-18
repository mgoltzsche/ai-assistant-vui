import './style.css'
import { MicVAD } from "@ricky0123/vad-web"
import { wavBlobFromFloat32Array } from './audioconversion.js'
//import { float32ToInt8 } from './audioconversion.js'
import { PCMStreamPlayer } from './player.js'

try {
	if (!navigator.mediaDevices) {
		throw 'Please use the app via HTTPS in order to enable microphone support!'
	}

	const player = new PCMStreamPlayer()
	const myvad = await MicVAD.new({
		onSpeechEnd: async function(audioSamples) {
			console.log("voice detected")
			/*fetch('/channels/default/audio', {
				method: 'POST',
				body: wavBlobFromFloat32Array(audioSamples, 16000)
			});*/
			player.send(await wavBlobFromFloat32Array(audioSamples).bytes())
		},
	})
	player.start(`wss://${window.location.host}/channels/default/audio?buffer-ms=50`);
	myvad.start()
} catch(e) {
	const errorEl = document.querySelector('#error');
	errorEl.innerHTML = e.toString()
}
