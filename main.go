package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

var (
	device     = flag.String("dev", "/dev/video0", "video `device` to configure")
	presetFlag = flag.String("preset", "", "apply preset `n` (1-12, or F1-F12) and exit instead of starting the UI")
	configPath = flag.String("config", "", "config `file` (default $XDG_CONFIG_HOME/webcam/config.yml)")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `Usage: webcam [flags]

Webcam shows and changes the controls of a V4L2 camera, such as brightness,
focus and exposure. With -preset it applies a saved preset and exits.

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "webcam:", err)
		os.Exit(1)
	}
}

func run() error {
	cam, err := newCam(*device)
	if err != nil {
		return err
	}
	defer cam.Close()

	configFile, err := LoadConfigFile(*configPath)
	if err != nil {
		return err
	}

	if *presetFlag != "" {
		return applyPresetFlag(cam, configFile, *presetFlag)
	}

	if len(configFile.LegacyPresets) > 0 {
		// Save the presets as this camera's now, so that they are not
		// moved to another camera later.
		configFile.CameraPresets(cam.Card)
		if err := SaveConfigFile(configFile); err != nil {
			return err
		}
	}

	_, err = tea.NewProgram(NewModel(cam, configFile)).Run()
	return err
}

// applyPresetFlag applies the preset named by the -preset flag to cam.
func applyPresetFlag(cam *Webcam, config ConfigFile, name string) error {
	id := strings.TrimLeft(name, "fF")
	preset, ok := config.CameraPresets(cam.Card)[id]
	if !ok {
		return fmt.Errorf("no preset %s saved for %s", name, cam.Card)
	}
	if err := applyPreset(cam, preset); err != nil {
		return fmt.Errorf("%s: %w", cam.Path, err)
	}
	return nil
}
