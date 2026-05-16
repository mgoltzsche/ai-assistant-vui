import './style.css'
import { MicVAD } from "@ricky0123/vad-web"
import { wavBlobFromFloat32Array } from './audioconversion.js'
import { RealTimeAudioPlayer } from './audioplayer.js'
import { PCMWebsocket } from './pcmwebsocket.js'

try {
  if (!navigator.mediaDevices) {
    throw 'Please use the app via HTTPS in order to enable microphone support!'
  }

  const player = new RealTimeAudioPlayer()
  const websocket = new PCMWebsocket({
	url: `wss://${window.location.host}/channels/default/audio?buffer-ms=50`,
	player: player
  })
  const myvad = await MicVAD.new({
    onSpeechEnd: async function(audioSamples) {
      console.log("voice detected")
      /*fetch('/channels/default/audio', {
        method: 'POST',
        body: wavBlobFromFloat32Array(audioSamples, 16000)
      });*/
      websocket.send(await wavBlobFromFloat32Array(audioSamples, 16000).bytes())
    },
  });

  player.start()
  websocket.start()
  myvad.start()
} catch(e) {
  const errorEl = document.querySelector('#error')
  errorEl.innerHTML = e.toString()
}
