package butler

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/bmc-toolbox/bmcbutler/pkg/asset"
	metrics "github.com/bmc-toolbox/gin-go-metrics"
)

func (b *Butler) myLocation(location string) bool {
	for _, l := range b.Config.Locations {
		if l == location {
			return true
		}
	}

	return false
}

func (b *Butler) timeTrack(start time.Time, name string, asset *asset.Asset) {
	elapsed := time.Since(start)
	seconds := elapsed.Seconds()
	b.Log.WithFields(logrus.Fields{
		"Serial":            asset.Serial,
		"IPAddress":         asset.IPAddress,
		"AssetType":         asset.Type,
		"Vendor":            asset.Vendor,
		"ConfigurationTime": seconds,
	}).Info(fmt.Sprintf("%s on %s took %f seconds.", name, asset.IPAddress, seconds))
}

// msgHandler invokes the appropriate action based on msg attributes.
// nolint: gocyclo
func (b *Butler) msgHandler(msg Msg) {
	// If an interrupt was received, return.
	if b.interrupt {
		return
	}

	component := "msgHandler"

	metrics.IncrCounter([]string{"butler", "asset_recvd"}, 1)

	// If an asset has no IPAddress, we can't do anything about it!
	if len(msg.Asset.IPAddresses) == 0 || len(msg.Asset.IPAddresses) == 1 && msg.Asset.IPAddresses[0] == "0.0.0.0" {
		b.Log.WithFields(logrus.Fields{
			"component": component,
			"Serial":    msg.Asset.Serial,
			"AssetType": msg.Asset.Type,
		}).Warn("Asset was received by butler without any IP(s) info, skipped.")

		metrics.IncrCounter([]string{"butler", "asset_recvd_noip"}, 1)
		return
	}

	// If an asset has a location defined, we may want to filter it.
	if msg.Asset.Location != "" {
		if !b.myLocation(msg.Asset.Location) && !b.Config.IgnoreLocation {
			b.Log.WithFields(logrus.Fields{
				"component":     component,
				"Serial":        msg.Asset.Serial,
				"AssetType":     msg.Asset.Type,
				"AssetLocation": msg.Asset.Location,
			}).Warn("Butler won't manage asset based on its current location.")

			metrics.IncrCounter([]string{"butler", "asset_recvd_location_unmanaged"}, 1)
			return
		}
	}

	// This field helps with enumerating the unique assets we have, since some assets don't
	//   have a serial and some don't have an IP address. This is only for logging.
	identifier := "Serial: " + msg.Asset.Serial + ", IP(s): " + strings.Join(msg.Asset.IPAddresses, ",")

	switch {
	case msg.Asset.Execute:
		err := b.executeCommand(msg.AssetExecute, &msg.Asset)
		if err != nil {
			b.Log.WithFields(logrus.Fields{
				"component":    component,
				"AssetType":    msg.Asset.Type,
				"Error":        err,
				"HardwareType": msg.Asset.HardwareType,
				"ID":           identifier,
				"IPAddress":    msg.Asset.IPAddress,                      // When we fail to login to the BMC, this field is not set...
				"IPAddresses":  strings.Join(msg.Asset.IPAddresses, ","), // ... and that's why we list all tried IP addresses.
				"Location":     msg.Asset.Location,
				"Serial":       msg.Asset.Serial,
				"Vendor":       msg.Asset.Vendor, // At this point the vendor may or may not be known.
			}).Warn("Execute action returned error.")
			metrics.IncrCounter([]string{"butler", "execute_fail"}, 1)
			return
		}

		b.Log.WithFields(logrus.Fields{
			"component": component,
			"Serial":    msg.Asset.Serial,
			"AssetType": msg.Asset.Type,
			"Vendor":    msg.Asset.Vendor, // At this point the vendor may or may not be known.
			"Location":  msg.Asset.Location,
		}).Info("Execute action succeeded.")

		metrics.IncrCounter([]string{"butler", "execute_success"}, 1)
		return
	case msg.Asset.Configure:
		err := b.configureAsset(msg.AssetConfig, &msg.Asset)
		if err != nil {
			b.Log.WithFields(logrus.Fields{
				"component":    component,
				"AssetType":    msg.Asset.Type,
				"Error":        err,
				"HardwareType": msg.Asset.HardwareType,
				"ID":           identifier,
				"IPAddress":    msg.Asset.IPAddress,                      // When we fail to login to the BMC, this field is not set...
				"IPAddresses":  strings.Join(msg.Asset.IPAddresses, ","), // ... and that's why we list all tried IP addresses.
				"Location":     msg.Asset.Location,
				"Serial":       msg.Asset.Serial,
				"Vendor":       msg.Asset.Vendor, // At this point the vendor may or may not be known.
			}).Warn("Configure action returned error.")

			metrics.IncrCounter([]string{"butler", "configure_fail"}, 1)
			return
		}

		b.Log.WithFields(logrus.Fields{
			"AssetType":    msg.Asset.Type,
			"component":    component,
			"Error":        err,
			"HardwareType": msg.Asset.HardwareType,
			"IPAddress":    msg.Asset.IPAddress,
			"ID":           identifier,
			"Location":     msg.Asset.Location,
			"Serial":       msg.Asset.Serial,
			"Vendor":       msg.Asset.Vendor, // At this point the vendor may or may not be known.
		}).Info("Configure action succeeded.")

		metrics.IncrCounter([]string{"butler", "configure_success"}, 1)
		return
	default:
		b.Log.WithFields(logrus.Fields{
			"component": component,
			"Serial":    msg.Asset.Serial,
			"AssetType": msg.Asset.Type,
			"Vendor":    msg.Asset.Vendor, // At this point the vendor may or may not be known.
			"Location":  msg.Asset.Location,
		}).Warn("Unknown action request on asset.")
	}
}
