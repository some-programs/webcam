package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "charm.land/bubbletea/v2"
)

var device = flag.String("dev", "/dev/video0", "Input video device")

func main() {
	flag.Parse()

	cam, err := newCam(*device)
	if err != nil {
		panic(err.Error())
	}
	defer cam.Close()

	configFile, err := LoadConfigFile()
	if err != nil {
		log.Fatal(err)
	}

	if len(configFile.LegacyPresets) > 0 {
		// Save the presets as this camera's now, so that they are not
		// moved to another camera later.
		configFile.CameraPresets(cam.Card)
		if err := SaveConfigFile(configFile); err != nil {
			log.Fatal(err)
		}
	}

	m := NewModel(cam, configFile)
	if err != nil {
		panic(err)
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Oh no!", err)
		os.Exit(1)
	}

}
