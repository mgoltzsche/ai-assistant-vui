import './style.css'
import { MicVAD } from "@ricky0123/vad-web"
import { wavBlobFromFloat32Array } from './audioconversion.js'
import { PCMStreamPlayer } from './player.js'

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

player.start(`ws://${window.location.host}/channels/default/audio?buffer-ms=50`);
