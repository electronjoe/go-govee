# go-govee
Playing with Golang parsing of Govee temperature hydrometers

## Things to Know

### Bluetooth Single-Use

Processing overheard BTLE advertisements from Govee devices requires this library to be always-active-scanning.  This will prevent bluetooth use by other devices.  If you need bluetooth for other services, consider buying a USB dongle?

### TODOs

This library should be refactored to allow all overheard advertisements to be captured externally - and a subset filtered / passed into Govee temp processing (to allow maximum re-use of advertisements received by the blocked BLE device).

I have observed anamalous jumps in temperature readings by 4 degrees - yet humidity values show no such jumps.  I'm exploring possible causes.

## Resources

* [This Home Assistant Forum Post](https://community.home-assistant.io/t/govee-ble-thermometer-hygrometer-sensor/166696)
    * Perhaps the best I've found

## Directory Layout

I liked [This](https://github.com/golang-standards/project-layout) project layout advice, and am leveraging it.

TODO: Review vs [This](https://github.com/golang-standards/project-layout)

## Enable BT

For Ubuntu 20.04 Rpi 3 [This](https://raspberrypi.stackexchange.com/questions/114586/rpi-4b-bluetooth-unavailable-on-ubuntu-20-04) worked for me:

```
sudo apt-get install pi-bluetooth
sudo vim /boot/firmware/usrcfg.txt
paste in include btcfg.txt and save
sudo reboot
```

Device shows up now with:

```shell
hcitool dev
```

## Discover Devices

```shell
sudo bluetoothctl
scan on
```

Look for your Govee_HS074_XXXX

* E3:60:58:E1:90:E3 Govee_H5074_90E3

Or (simpler?):

```shell
sudo hcitool -i hci0 lescan --duplicates --passive
```

### Alternative

There are Bluetooth scanners available on Google Play Store.

## Running

With stderr-based logging at info and higher

```shell
sudo go run cmd/govee-mon/main.go --logtostderr
```

### Fetching prometheus data by hand

```shell
curl http://localhost:2112/metrics
```

### Configuring Prometheus (prometheus.yml)

```shell
scrape_configs:
- job_name: myapp
  scrape_interval: 10s
  static_configs:
  - targets:
    - localhost:2112
```

## Issues

### Govee Overheard but not Temp Advertisements

This issue appears identical to that posted by [Martso](https://community.home-assistant.io/t/govee-ble-thermometer-hygrometer-sensor/166696/21):

* Govee advertisements are overheard
* But not the shorter ones containing temp data

> Well, somehow got it working – some combination of manually resetting the hci interfaces and enabling active scanning.

```shell
sudo hciconfig hci0 down
sudo go run cmd/govee-mon/main.go
```

Worked for me.
