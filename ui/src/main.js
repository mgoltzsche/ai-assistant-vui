import './style.css'
import { MicVAD } from "@ricky0123/vad-web"
import { wavBlobFromFloat32Array } from './audio.js'

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

const player = new Audio();
player.src = `/channels/default/audio?t=${Math.floor(Date.now() / 1000)}`
player.preload = 'none'
player.load(); // prevent caching
player.play();
