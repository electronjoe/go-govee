package govee

import (
	"encoding/binary"

	"github.com/bettercap/gatt"
	"github.com/golang/glog"
)

const (
	goveeHS074WantMfLen = 9
	goveeHS074WantFlags = 6
)

// IsGoveeH5074Advertisement indicates whether the received advertisement could originate from a H5074 device.
func IsGoveeH5074Advertisement(mfDataLen int, flags gatt.Flags, id string) bool {
	if mfDataLen != goveeHS074WantMfLen {
		glog.V(4).Infof("Advertisement ManufacturerData from id %q len %d, want %d - will not process", id, mfDataLen, goveeHS074WantMfLen)
		return false
	}

	// I am noting that this flag value is missing in OSX use of bettercap
	if flags != goveeHS074WantFlags {
		glog.V(4).Infof("Advertisement flag value %d, want %d - will not process", flags, goveeHS074WantFlags)
		return false
	}

	return true
}

// CToF converts the passed celcius value to fahrenheit.
func CToF(celcius float32) float32 {
	return (celcius * (9.0 / 5.0)) + 32.0
}

func ParseTempC(manufacturerData []byte) float32 {
	twosComplement := binary.LittleEndian.Uint16(manufacturerData)
	var converted = int16(twosComplement)
	return float32(converted) / 100.0
}
