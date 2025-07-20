package audio

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/gordonklaus/portaudio"
)

func inputDevice(deviceNameOrID string) (d *portaudio.DeviceInfo, err error) {
	if deviceNameOrID == "" {
		d, err = portaudio.DefaultInputDevice()
		if err != nil {
			return nil, err
		}
	} else {
		d, err = device(deviceNameOrID)
		if err != nil {
			return nil, fmt.Errorf("get audio input device: %w", err)
		}

		if d.MaxInputChannels < 1 {
			printAvailableDevices()
			return nil, fmt.Errorf("audio device %q is not an input device or in use by another program", d.Name)
		}
	}

	slog.Info(fmt.Sprintf("using audio input device %q, sample rate: %d", d.Name, int(d.DefaultSampleRate)))

	return d, nil
}

func outputDevice(deviceNameOrID string) (d *portaudio.DeviceInfo, err error) {
	if deviceNameOrID == "" {
		d, err = portaudio.DefaultOutputDevice()
		if err != nil {
			return nil, err
		}
	} else {
		d, err = device(deviceNameOrID)
		if err != nil {
			return nil, fmt.Errorf("get audio output device: %w", err)
		}

		if d.MaxOutputChannels < 1 {
			printAvailableDevices()
			return nil, fmt.Errorf("audio device %q is not an output device or in use by another program", d.Name)
		}
	}

	slog.Info(fmt.Sprintf("using audio output device %q, sample rate: %d", d.Name, int(d.DefaultSampleRate)))

	return d, nil
}

func device(device string) (*portaudio.DeviceInfo, error) {
	if device == "" {
		return nil, fmt.Errorf("no audio device ID or name specified")
	}

	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("list available audio devices: %w", err)
	}

	deviceID, err := strconv.ParseInt(device, 10, 32)
	if err != nil {
		// Device name given
		for _, d := range devices {
			if strings.Contains(d.Name, device) {
				return d, nil
			}
		}

		printAvailableDevices()

		return nil, fmt.Errorf("audio device %q not found", device)
	}

	// device ID given
	if deviceID >= int64(len(devices)) || deviceID < 0 {
		printAvailableDevices()

		return nil, fmt.Errorf("audio device %d not found - please specify the ID of an existing device", deviceID)
	}

	return devices[deviceID], nil
}

func printAvailableDevices() {
	devices, err := portaudio.Devices()
	if err != nil {
		slog.Warn(fmt.Sprintf("get available audio devices: %s\n", err))
	}
	fmt.Fprintln(os.Stderr, "\nAvailable audio devices:\n")
	format := "%2s  %-55s  %2s  %3s  %s\n"
	fmt.Fprintf(os.Stderr, format, "ID", "NAME", "IN", "OUT", "SAMPLERATE")
	for i, device := range devices {
		fmt.Fprintf(os.Stderr, "%2d  %-55s  %2d  %3d  %10d\n", i, device.Name, device.MaxInputChannels, device.MaxOutputChannels, int(device.DefaultSampleRate))
	}
	fmt.Fprintln(os.Stderr)
}
