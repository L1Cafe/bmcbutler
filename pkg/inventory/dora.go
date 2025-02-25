// Copyright © 2018 Joel Rebello <joel.rebello@booking.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package inventory

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/bmc-toolbox/bmcbutler/pkg/asset"
	"github.com/bmc-toolbox/bmcbutler/pkg/config"
	metrics "github.com/bmc-toolbox/gin-go-metrics"
	"github.com/sirupsen/logrus"
)

// Dora struct holds attributes required to retrieve assets from Dora,
// and pass them to the butlers.
type Dora struct {
	Log             *logrus.Logger
	BatchSize       int
	AssetsChan      chan<- []asset.Asset
	Config          *config.Params
	FilterAssetType []string
}

// DoraAssetAttributes struct is used to unmarshal Dora data.
type DoraAssetAttributes struct {
	Serial         string `json:"serial"`
	BmcAddress     string `json:"bmc_address"`
	Vendor         string `json:"vendor"`
	ScannedAddress string `json:"ip"`   // set when we unmarshal the scanned_ports data
	Site           string `json:"site"` // set when we unmarshal the scanned_ports data
}

// DoraAssetData struct is used to unmarshal Dora data.
type DoraAssetData struct {
	Attributes DoraAssetAttributes `json:"attributes"`
}

// DoraLinks struct is used to unmarshal Dora data.
type DoraLinks struct {
	First string `json:"first"`
	Last  string `json:"last"`
	Next  string `json:"next"`
}

// DoraAsset struct is used to unmarshal Dora data.
type DoraAsset struct {
	Data  []DoraAssetData `json:"data"`
	Links DoraLinks       `json:"links"`
}

// for a list of assets, update its location value
func (d *Dora) setLocation(doraInventoryAssets []asset.Asset) (err error) {
	component := "inventory"
	log := d.Log

	apiURL := d.Config.Inventory.Dora.URL
	queryURL := fmt.Sprintf("%s/v1/scanned_ports?filter[port]=22&filter[ip]=", apiURL)

	// Collect IPAddresses used to look up the location.
	ips := make([]string, 0)

	for _, asset := range doraInventoryAssets {
		ips = append(ips, asset.IPAddress)
	}

	queryURL += strings.Join(ips, ",")
	resp, err := http.Get(queryURL)
	if err != nil || resp.StatusCode != 200 {
		log.WithFields(logrus.Fields{
			"component":  component,
			"url":        queryURL,
			"Error":      err,
			"StatusCode": resp.StatusCode,
		}).Warn("Unable to query Dora for IP location info.")
		return err
	}

	body, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	var doraScannedPortAssets DoraAsset
	err = json.Unmarshal(body, &doraScannedPortAssets)
	if err != nil {
		log.WithFields(logrus.Fields{
			"component": component,
			"url":       queryURL,
			"Error":     err,
		}).Warn("Unable to unmarshal Dora scanned IP info.")
		return err
	}

	// for each scanned IP update respective asset Location
	for _, scannedPortAsset := range doraScannedPortAssets.Data {
		for idx, inventoryAsset := range doraInventoryAssets {
			if scannedPortAsset.Attributes.ScannedAddress == inventoryAsset.IPAddress {
				doraInventoryAssets[idx].Location = scannedPortAsset.Attributes.Site
			}
		}
	}

	return err
}

func (d *Dora) AssetRetrieve() func() {
	// Setup the asset types we want to retrieve data for.
	switch {
	case d.Config.FilterParams.Chassis:
		d.FilterAssetType = append(d.FilterAssetType, "chassis")
	case d.Config.FilterParams.Servers:
		d.FilterAssetType = append(d.FilterAssetType, "blade")
		d.FilterAssetType = append(d.FilterAssetType, "discrete")
	case !d.Config.FilterParams.Chassis && !d.Config.FilterParams.Servers:
		d.FilterAssetType = []string{"chassis", "blade", "discrete"}
	}

	// Based on the filter param given, return the asset iterator method.
	switch {
	case d.Config.FilterParams.Serials != "":
		return d.AssetIterBySerial
	default:
		return d.AssetIter
	}
}

// AssetIterBySerial is an iterator method,
// to retrieve assets from Dora by the given serial numbers,
// assets are then sent over the inventory channel.
func (d *Dora) AssetIterBySerial() {
	serials := d.Config.FilterParams.Serials
	apiURL := d.Config.Inventory.Dora.URL

	component := "inventory"

	log := d.Log
	defer close(d.AssetsChan)

	for _, assetType := range d.FilterAssetType {
		// Setup the right dora query path.
		var path string
		switch assetType {
		case "blade":
			path = "blades"
		case "discrete":
			path = "discretes"
		default:
			path = assetType
		}

		queryURL := fmt.Sprintf("%s/v1/%s?filter[serial]=", apiURL, path)
		queryURL += strings.ToLower(serials)
		assets := make([]asset.Asset, 0)

		resp, err := http.Get(queryURL)
		if err != nil {
			log.WithFields(logrus.Fields{
				"component": component,
				"url":       queryURL,
				"Error":     err,
			}).Fatal("Failed to query dora for serial(s).")
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.WithFields(logrus.Fields{
				"component": component,
				"url":       queryURL,
				"Error":     err,
			}).Fatal("Failed to query dora for serial(s).")
		}
		resp.Body.Close()

		var doraAssets DoraAsset
		err = json.Unmarshal(body, &doraAssets)
		if err != nil {
			log.WithFields(logrus.Fields{
				"component": component,
				"url":       queryURL,
				"Error":     err,
			}).Fatal("Unable to unmarshal data returned from dora.")
		}

		if len(doraAssets.Data) == 0 {
			log.WithFields(logrus.Fields{
				"component": component,
				"Query url": queryURL,
			}).Debug("Asset was not located in dora inventory.")
			continue
		} else {
			log.WithFields(logrus.Fields{
				"component": component,
				"Query url": queryURL,
			}).Debug("Asset located in dora inventory.")
		}

		for _, item := range doraAssets.Data {
			if item.Attributes.BmcAddress == "" {
				log.WithFields(logrus.Fields{
					"component": component,
				}).Warn("Asset location could not be determined, since the asset has no IP.")
				continue
			}

			assets = append(assets, asset.Asset{
				IPAddress: item.Attributes.BmcAddress,
				Serial:    item.Attributes.Serial,
				Vendor:    item.Attributes.Vendor,
				Type:      assetType,
			})

		}

		err = d.setLocation(assets)
		if err != nil {
			log.WithFields(logrus.Fields{
				"component": component,
				"Error":     err,
			}).Warn("Unable to determine location of assets.")
			return
		}

		d.AssetsChan <- assets
	}
}

// Stuffs assets into an array, writes that to the channel.
func (d *Dora) AssetIter() {
	apiURL := d.Config.Inventory.Dora.URL
	component := "retrieveInventoryAssetsDora"

	defer close(d.AssetsChan)
	// defer metrics.MeasureSince(component, time.Now())

	log := d.Log

	for _, assetType := range d.FilterAssetType {
		var path string

		// This asset type in Dora is plural.
		if assetType == "blade" {
			path = "blades"
		} else if assetType == "discrete" {
			path = "discretes"
		} else {
			path = assetType
		}

		queryURL := fmt.Sprintf("%s/v1/%s?page[offset]=%d&page[limit]=%d", apiURL, path, 0, d.BatchSize)
		for {
			assets := make([]asset.Asset, 0)

			resp, err := http.Get(queryURL)
			if err != nil || resp.StatusCode != 200 {
				log.WithFields(logrus.Fields{
					"component": component,
					"url":       queryURL,
					"Error":     err,
				}).Fatal("Error querying Dora for assets.")
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.WithFields(logrus.Fields{
					"component": component,
					"url":       queryURL,
					"Error":     err,
				}).Fatal("Error querying Dora for assets.")
			}
			resp.Body.Close()

			var doraAssets DoraAsset
			err = json.Unmarshal(body, &doraAssets)
			if err != nil {
				log.WithFields(logrus.Fields{
					"component": component,
					"url":       queryURL,
					"Error":     err,
				}).Fatal("Error unmarshaling data returned from Dora.")
			}

			metrics.IncrCounter(
				[]string{"inventory", "assets_fetched_dora"},
				int64(len(doraAssets.Data)))

			// for each asset, get its location
			// store in the assets slice
			// if an asset has no bmcAddress we log and skip it.
			for _, item := range doraAssets.Data {

				if item.Attributes.BmcAddress == "" || item.Attributes.BmcAddress == "0.0.0.0" {
					log.WithFields(logrus.Fields{
						"component": component,
					}).Warn("Asset location could not be determined, since the asset has no IP.")

					metrics.IncrCounter([]string{"inventory", "assets_noip_dora"}, 1)
					continue
				}

				assets = append(assets,
					asset.Asset{
						IPAddress: item.Attributes.BmcAddress,
						Serial:    item.Attributes.Serial,
						Vendor:    item.Attributes.Vendor,
						Type:      assetType,
					})

			}

			err = d.setLocation(assets)
			if err != nil {
				log.WithFields(logrus.Fields{
					"component": component,
					"Error":     err,
				}).Warn("Asset location could not be determined, ignoring assets")

				metrics.IncrCounter([]string{"inventory", "assets_nolocation_dora"}, 1)
				continue
			}

			metrics.IncrCounter(
				[]string{"inventory", "assets_returned_dora"},
				int64(len(assets)),
			)

			d.AssetsChan <- assets

			// if we reached the end of dora assets
			if doraAssets.Links.Next == "" {
				log.WithFields(logrus.Fields{
					"component": component,
					"url":       queryURL,
				}).Info("Reached end of assets in dora")
				break
			}

			// next url to query
			queryURL = fmt.Sprintf("%s%s", apiURL, doraAssets.Links.Next)
		}
	}
}
