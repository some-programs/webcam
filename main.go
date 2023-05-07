package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
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

	m := NewModel(cam, configFile)
	if err != nil {
		panic(err)
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Oh no!", err)
		os.Exit(1)
	}
	if err := SaveConfigFile(configFile); err != nil {
		log.Fatal(err)
	}

}
