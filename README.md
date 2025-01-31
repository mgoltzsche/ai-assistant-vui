# ai-assistant-vui

A voice user interface (VUI) to interact with an AI assistant.

The VUI runs as a client of an OpenAI-compatible API (which may be served by [LocalAI](https://github.com/mudler/LocalAI)).
[silero-vad](https://github.com/snakers4/silero-vad) is built into the client for voice activity detection (VAD).
For chat completion, speech-to-text (STT) and text-to-speech (TTS) capabilities the client leverages the API server.
In order to detect whether the AI assistant is addressed, a wake word can be configured.
Though, wake word support is implemented by matching the STT (whisper) output string against the wake word, requiring all voice communication to be STT processed, at least for now.

## Build

```sh
make
```

## Run

1. Start the [LocalAI](https://github.com/mudler/LocalAI) API server (LLM server):
```sh
make run-localai
```

2. Browse the LocalAI web GUI at [http://127.0.0.1:8080/browse/](http://127.0.0.1:8080/browse/) and search and install the models you want to use, e.g. `whisper-1` (STT), `llama-3-sauerkrautlm-8b-instruct` (chat) and `voice-en-us-amy-low` (TTS) or `voice-de-kerstin-low` (TTS).

3. Run the VUI (within another terminal):
```sh
make run-vui INPUT_DEVICE="KLIM Talk" OUTPUT_DEVICE="ALC1220 Analog"
```

You will likely have to replace the values of `INPUT_DEVICE` and `OUTPUT_DEVICE` with the names or IDs of your audio devices.
You may not be able to use an audio device when another program (e.g. your browser) is already using it.
In that case, please close other programs, wait a few seconds and then re-run the VUI.

You need to mention the configured wake word with each request to the AI assistant.
It defaults to "Computer".
For instance you can ask "Computer, tell me a joke!"
