package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/bettercap/gatt"
	"github.com/bettercap/gatt/examples/option"
	"github.com/electronjoe/go-govee/pkg/govee"
	"github.com/golang/glog"
	"github.com/jaedle/golang-tplink-hs100/pkg/configuration"
	"github.com/jaedle/golang-tplink-hs100/pkg/hs100"
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
	outletState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "outlet_state",
		Help: "The state of the specified outlet (0 = off, 1 = on) by OutletName",
	}, []string{"name"})
)

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

func onPeriphDiscovered(config devicesConfig, outletMap map[string]*hs100.Hs100) func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	return func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
		if _, ok := config.IDToNames[p.ID()]; !ok {
			btRx.WithLabelValues("not-in-devices-yml").Inc()
			glog.V(4).Infof("Device ID %q not listed in devices.yml, skipping!", p.ID())
			return
		}

		btRx.WithLabelValues("in-devices-yml").Inc()

		if !govee.IsGoveeH5074Advertisement(len(a.ManufacturerData), a.Flags, p.ID()) {
			glog.V(4).Infof("Overheard advertisement with len %d, type %d and deemed it not a Govee H5074 device, ignoring", len(a.ManufacturerData), a.Flags)
			govRx.WithLabelValues(p.ID(), "rejected-invalid-len-or-type")
			return
		}

		govRx.WithLabelValues(p.ID(), "processed").Inc()

		t := govee.CToF(govee.ParseTempC(a.ManufacturerData[3:5]))
		h := float32(binary.LittleEndian.Uint16(a.ManufacturerData[5:7])) / 100.0
		b := a.ManufacturerData[7]

		name := config.IDToNames[p.ID()]
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

		for _, obj := range config.TemperatureObjectives {
			if obj.TempSensorName == name {
				outlet, ok := outletMap[obj.OutletName]
				if !ok {
					glog.Errorf("Configuration error, %v cites undiscovered outlet with OutletName %q", obj, obj.OutletName)
					return
				}

				// TODO: Cache this state?
				isOn, err := outlet.IsOn()
				if err != nil {
					glog.Errorf("Failure to call IsOn for outlet %q, err %s", obj.OutletName, err)
					return
				}

				if isOn {
					outletState.WithLabelValues(obj.OutletName).Set(1.0)
				} else {
					outletState.WithLabelValues(obj.OutletName).Set(0.0)
				}

				if isOn && (t > obj.HeatOffAboveF) {
					outlet.TurnOff()
				} else if !isOn && (t < obj.HeatOnBelowF) {
					outlet.TurnOn()
				}
			}
		}
	}
}

type temperatureObjective struct {
	TempSensorName string  `yaml:"TempSensorName"`
	OutletName     string  `yaml:"OutletName"`
	HeatOnBelowF   float32 `yaml:"HeatOnBelowF"`
	HeatOffAboveF  float32 `yaml:"HeatOffAboveF"`
}

type devicesConfig struct {
	IDToNames             map[string]string      `yaml:"IDToNames"`
	TemperatureObjectives []temperatureObjective `yaml:"TemperatureObjectives"`
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
	var config devicesConfig
	if err := config.Parse(data); err != nil {
		glog.Fatal(err)
	}
	fmt.Printf("%+v\n", config)

	fmt.Println("Success")

	fmt.Println("Discovering Smart Outlets")

	outlets, err := hs100.Discover("192.168.10.0/24",
		configuration.Default().WithTimeout(time.Second),
	)
	if err != nil {
		glog.Fatalf("Failed in hs100.Discover, err: %s\n", err)
	}

	outletMap := make(map[string]*hs100.Hs100)
	for _, d := range outlets {
		name, _ := d.GetName()
		outletMap[name] = d
		fmt.Println("Discovered HS100 outlet with name: %s", name)
	}

	// TODO - validate that TemperatureObjectives' TempSensorName and OutletName all exist
	// if !validateConfig(config) {
	// 	glog.Fatalf("Inconsitent configuration file or missing Outlet in discovery, contains TempSensorName not in IDToNames or unknown OutletName")
	// }

	d, err := gatt.NewDevice(option.DefaultClientOptions...)
	if err != nil {
		glog.Fatalf("Failed to open device, err: %s\n", err)
	}

	// Register handlers.
	d.Handle(gatt.PeripheralDiscovered(onPeriphDiscovered(config, outletMap)))
	d.Init(onStateChanged)

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)

	select {}
}
