import './style.css'
import { MicVAD } from "@ricky0123/vad-web"
import { riffWaveFromFloat32Array } from './riffwave.js'
import { RealTimeAudioPlayer } from './audioplayer.js'
import { PbWebsocket } from './pbwebsocket.js'

try {
  if (!navigator.mediaDevices) {
    throw 'Please use the app via HTTPS in order to enable microphone support!'
  }

  const player = new RealTimeAudioPlayer()
  const websocket = new PbWebsocket({
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
      websocket.send(riffWaveFromFloat32Array(audioSamples, 16000))
    },
  });

  player.start()
  websocket.start()
  myvad.start()
} catch(e) {
  const errorEl = document.querySelector('#error')
  errorEl.innerHTML = e.toString()
}
