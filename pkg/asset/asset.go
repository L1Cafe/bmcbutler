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

package asset

// Asset is a unit of BMC/Chassis BMC,
// assets are passed around from inventories to butlers.
type Asset struct {
	// A chassis asset may have more than one IP.
	// When the asset is first retrieved, all IPs are listed in this slice.
	IPAddresses []string
	// The active IP is assigned to this field once identified.
	// When we fail to login to the asset, this field is NOT set.
	IPAddress    string
	Serial       string
	Vendor       string
	HardwareType string
	Type         string // "server" or "chassis"
	Location     string
	Setup        bool              // If set, butlers will setup the asset.
	Configure    bool              // If set, butlers will configure the asset.
	Execute      bool              // If set, butlers will execute given command(s) on the asset.
	Extra        map[string]string // Any extra params needed to be set in a asset.
}
