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

package bmclogin

import (
	"errors"
	"fmt"

	"github.com/bmc-toolbox/bmclib/devices"
	"github.com/bmc-toolbox/bmclib/discover"
)

type Params struct {
	IpAddresses     []string            //IPs - since chassis may have more than a single IP.
	Credentials     []map[string]string //A slice of username, passwords to login with.
	CheckCredential bool                //Validates the credential works - this is only required for http(s) connections.
	Retries         int                 //The number of times to retry a credential
}

type LoginInfo struct {
	FailedCredentials  []map[string]string //The credentials that failed.
	WorkingCredentials map[string]string   //The credentials that worked.
	ActiveIpAddress    string              //The IP that we could login into and is active.
	Attempts           int                 //The number of login attempts.
}

// Login() carries out login actions.
func (p *Params) Login() (connection interface{}, loginInfo LoginInfo, err error) {

	if p.Retries == 0 {
		p.Retries = 1
	}

	//for credential map in slice
	for _, credentials := range p.Credentials {

		//for each credential k, v
		for user, pass := range credentials {

			//for each IpAddress
			for _, ip := range p.IpAddresses {
				if ip == "" {
					continue
				}

				//for each retry attempt
				for t := 0; t <= p.Retries; t++ {

					loginInfo.Attempts += 1
					connection, ipInactive, err := p.attemptLogin(ip, user, pass)

					//if the IP is not active, break out of this loop
					//to try credentials on the next IP.
					if ipInactive {

						//if we're able to login to asset that has a single IP address,
						//but its status is not active,
						if len(p.IpAddresses) == 1 {
							loginInfo.WorkingCredentials = map[string]string{user: pass}

							return connection, loginInfo, err
						}
						break
					}

					if err == nil {
						loginInfo.ActiveIpAddress = ip
						loginInfo.WorkingCredentials = map[string]string{user: pass}
						return connection, loginInfo, err
					}

					loginInfo.FailedCredentials = append(loginInfo.FailedCredentials, map[string]string{user: pass})
				}
			}
		}
	}

	return connection, loginInfo, errors.New("All attempts to login failed.")
}

// attemptLogin tries to scanAndConnect
func (p *Params) attemptLogin(ip string, user string, pass string) (connection interface{}, ipInactive bool, err error) {

	// Scan BMC type and connect
	connection, err = discover.ScanAndConnect(ip, user, pass)
	if err != nil {
		return connection, ipInactive, errors.New("ScanAndConnect attempt unsuccessful.")
	}

	// Don't attempt to login via web with credentials.
	if !p.CheckCredential {
		return connection, ipInactive, err
	}

	switch connection.(type) {
	case devices.Bmc:

		bmc := connection.(devices.Bmc)
		err := bmc.CheckCredentials()
		if err != nil {
			return connection, ipInactive, errors.New(
				fmt.Sprintf("BMC login attempt failed, account: %s", user))
		}

		//successful login.
		return connection, ipInactive, nil
	case devices.BmcChassis:

		chassis := connection.(devices.BmcChassis)
		err := chassis.CheckCredentials()
		if err != nil {
			return connection, ipInactive, errors.New(
				fmt.Sprintf("Chassis login attempt failed, account: %s", user))
		}

		//A chassis has one or more controllers
		//We return true if this controller is active.
		if !chassis.IsActive() {
			ipInactive = true
			return connection, ipInactive, nil
		}

		return connection, ipInactive, nil
	default:
		return connection, ipInactive, errors.New(
			fmt.Sprintf("Unrecognized device type."))
	}

	//we won't ever end up here
	return connection, ipInactive, errors.New(
		fmt.Sprintf("Unable to login"))
}