package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/bettercap/gatt"
	"github.com/bettercap/gatt/examples/option"
	"github.com/golang/glog"
	"gopkg.in/yaml.v3"
)

func onStateChanged(d gatt.Device, s gatt.State) {
	glog.Info("State:", s)
	switch s {
	case gatt.StatePoweredOn:
		glog.Info("scanning...")
		d.Scan([]gatt.UUID{}, false)
		return
	default:
		d.StopScanning()
	}
}

func onPeriphDiscovered(devices devicesConfig) func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	return func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
		if _, ok := devices.IdToNames[p.ID()]; !ok {
			glog.Infof("Device ID %q not listed in devices.yml, skipping!", p.ID())
			return
		}

		wantLen := 9
		if len(a.ManufacturerData) != wantLen {
			glog.Infof("Govee ManufacturerData len %d, want %d, skipping!", len(a.ManufacturerData), wantLen)
			return
		}

		temp := float32(binary.LittleEndian.Uint16(a.ManufacturerData[3:5])) / 100.0
		humidity := float32(binary.LittleEndian.Uint16(a.ManufacturerData[5:7])) / 100.0
		bat := a.ManufacturerData[7]

		glog.Infof("Received Govee Advertisement with temp=%f hum=%f bat=%d at rssi=%d",
			temp,
			humidity,
			bat,
			rssi,
		)
	}
}

type devicesConfig struct {
	IdToNames map[string]string `yaml:"IdToNames"`
}

func (c *devicesConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func main() {
	flag.Parse()

	fmt.Println("For log output to command line, execute with --logtostderr flag.")

	fmt.Println("Reading YAML Config")

	data, err := ioutil.ReadFile("configs/devices.yml")
	if err != nil {
		glog.Fatal(err)
	}
	var devices devicesConfig
	if err := devices.Parse(data); err != nil {
		glog.Fatal(err)
	}
	fmt.Printf("%+v\n", devices)

	fmt.Print("Success")

	d, err := gatt.NewDevice(option.DefaultClientOptions...)
	if err != nil {
		glog.Fatalf("Failed to open device, err: %s\n", err)
	}

	// Register handlers.
	d.Handle(gatt.PeripheralDiscovered(onPeriphDiscovered(devices)))
	d.Init(onStateChanged)
	select {}
}
