package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/bettercap/gatt"
	"github.com/bettercap/gatt/examples/option"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

var (
	btRx = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bluetooth_advertisement_rx",
		Help: "The total number of bluetooth advertisement receptions",
	}, []string{"type"})
	govRx = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "govee_rx",
		Help: "The total number of bluetooth advertisement from Govee devices",
	}, []string{"id", "processed"})
	temp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "temperature",
		Help: "The most recent temperature reported (deg fahrenheit) by name (mapped from ID)",
	}, []string{"name"})
	hum = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "humidity",
		Help: "The most recent humidity reported (deg fahrenheit) by name (mapped from ID)",
	}, []string{"name"})
	bat = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "battery",
		Help: "The most recent battery level reported (0-100) in percent by name (mapped from ID)",
	}, []string{"name"})
	rssiTelem = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rssi",
		Help: "The most recent Receive Signal Level (RSSI) reported by name (mapped from ID)",
	}, []string{"name"})
)

func parse_temp_c(manufacturer_data []byte) float32 {
	twos_complement := binary.LittleEndian.Uint16(manufacturer_data)
	var converted int16 = int16(twos_complement) // If positive value
	if (twos_complement & 0x80) != 0 {
		// Convert to negative representation
		converted = int16(twos_complement^0xFF) + 1
	}
	return float32(converted) / 100.0
}

func onStateChanged(d gatt.Device, s gatt.State) {
	glog.Info("State:", s)
	switch s {
	case gatt.StatePoweredOn:
		glog.Info("scanning...")
		d.Scan([]gatt.UUID{}, true)
		return
	default:
		d.StopScanning()
	}
}

func c_to_f(celcius float32) float32 {
	return (celcius * (9.0 / 5.0)) + 32.0
}

func onPeriphDiscovered(devices devicesConfig) func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	return func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
		if _, ok := devices.IdToNames[p.ID()]; !ok {
			btRx.WithLabelValues("not-in-devices-yml").Inc()
			glog.V(4).Infof("Device ID %q not listed in devices.yml, skipping!", p.ID())
			return
		}

		btRx.WithLabelValues("in-devices-yml").Inc()

		wantLen := 9
		if len(a.ManufacturerData) != wantLen {
			glog.V(4).Infof("Govee ManufacturerData len %d, want %d, skipping!", len(a.ManufacturerData), wantLen)

			govRx.WithLabelValues(p.ID(), "rejected-invalid-length").Inc()
			return
		}

		govRx.WithLabelValues(p.ID(), "processed").Inc()

		t := c_to_f(parse_temp_c(a.ManufacturerData[3:5]))
		h := float32(binary.LittleEndian.Uint16(a.ManufacturerData[5:7])) / 100.0
		b := a.ManufacturerData[7]

		name := devices.IdToNames[p.ID()]
		temp.WithLabelValues(name).Set(float64(t))
		hum.WithLabelValues(name).Set(float64(h))
		bat.WithLabelValues(name).Set(float64(b))
		rssiTelem.WithLabelValues(name).Set(float64(rssi))

		glog.V(3).Infof("Received Govee Advertisement with temp=%f hum=%f bat=%d at rssi=%d",
			t,
			h,
			b,
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

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)

	select {}
}
