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
	presetFlag = flag.String("preset", "", "apply preset `F1`-F12 and exit instead of starting the UI")
	configPath = flag.String("config", "", "config `file` (default $XDG_CONFIG_HOME/webcam/config.toml)")
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

	_, err = tea.NewProgram(NewModel(cam, configFile)).Run()
	return err
}

// applyPresetFlag applies the preset named by the -preset flag to cam. The
// name may leave out the F, as in "1".
func applyPresetFlag(cam *Webcam, config ConfigFile, name string) error {
	name = "F" + strings.TrimLeft(name, "fF")
	preset, ok := config.CameraPresets(cam.Card)[name]
	if !ok {
		return fmt.Errorf("no preset %s saved for %s", name, cam.Card)
	}
	if err := applyPreset(cam, preset); err != nil {
		return fmt.Errorf("%s: %w", cam.Path, err)
	}
	return nil
}
