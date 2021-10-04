package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/UniversityRadioYork/alapi/pkg/config"
	"github.com/UniversityRadioYork/alapi/pkg/monitor"
	soundio "github.com/crow-misia/go-libsoundio"
)

var flagCfg = flag.String("cfg", "./config.json", "path to config.json")

func main() {
	flag.Parse()
	cfg, err := config.Load(*flagCfg)
	if err != nil {
		log.Fatal(err)
	}

	sndio := soundio.Create(soundio.WithAppName("alapi-temp"))
	var backend soundio.Backend
	for i := 0; i < sndio.BackendCount(); i++ {
		backend = sndio.Backend(i)
		log.Printf("Have backend: %s", backend.String())
		if backend.Have() && backend.String() == cfg.Backend {
			break
		}
	}
	if backend == 0 {
		log.Fatalf("could not find requested backend: %v", cfg.Backend)
	}
	log.Printf("Using backend %v", backend)
	sndio.Disconnect()
	sndio = soundio.Create(soundio.WithAppName("alapi"), soundio.WithBackend(backend))
	err = sndio.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer sndio.Disconnect()
	sndio.FlushEvents()

	monitored := make(map[string]*soundio.Device)
	monitors := make(map[string]*monitor.DevicesMonitor)
	for i := 0; i < sndio.InputDeviceCount(); i++ {
		device := sndio.InputDevice(i)
		log.Printf("Found device: %s (ID %s)", device.Name(), device.ID())
		var key string
		for test, name := range cfg.Devices {
			if name == device.Name() {
				key = test
				break
			}
		}
		if key == "" {
			device.RemoveReference()
		} else {
			monitored[key] = device
		}
	}

	for key, dev := range monitored {
		mon := monitor.DevicesMonitor{Config: cfg}
		if err = mon.Init(dev); err != nil {
			log.Fatal(err)
		}
		defer mon.Close()
		monitors[key] = &mon
	}

	http.HandleFunc("/levels", func(w http.ResponseWriter, r *http.Request) {
		result := make(map[string]float64)
		for key, mon := range monitors {
			result[key] = mon.GetLevels()
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(result)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)

	log.Printf("Serving HTTP on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
