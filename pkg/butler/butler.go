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

package butler

import (
	"sync"
	"time"

	"github.com/gammazero/workerpool"
	"github.com/sirupsen/logrus"

	"github.com/bmc-toolbox/bmcbutler/pkg/asset"
	"github.com/bmc-toolbox/bmcbutler/pkg/config"
)

// Msg (butler messages) are passed over the butlerChan
// they declare assets for butlers to carry actions on.
type Msg struct {
	Asset        asset.Asset //Asset to be configured
	AssetConfig  []byte      //The BMC configuration read in from configuration.yml
	AssetSetup   []byte      //The One time setup configuration read from setup.yml
	AssetExecute string      //Commands to be executed on the BMC
}

// Butler struct holds attributes required to spawn butlers.
type Butler struct {
	Config     *config.Params //bmcbutler config, cli params
	ButlerChan <-chan Msg
	Log        *logrus.Logger
	StopChan   <-chan struct{}
	SyncWG     *sync.WaitGroup
	WorkerPool *workerpool.WorkerPool
	interrupt  bool
}

// Runner spawns a pool of butlers, waits until they are done.
func (b *Butler) Runner() {

	log := b.Log
	component := "Runner"

	defer b.SyncWG.Done()

	b.WorkerPool = workerpool.New(b.Config.ButlersToSpawn)
loop:
	for {
		select {
		case msg, ok := <-b.ButlerChan:
			if !ok {
				log.WithFields(logrus.Fields{
					"component": component,
				}).Trace("Butler channel closed.")
				break loop
			}

			for b.WorkerPool.WaitingQueueSize() > b.Config.ButlersToSpawn {
				log.WithFields(logrus.Fields{
					"component":          component,
					"Waiting queue size": b.WorkerPool.WaitingQueueSize(),
					"butlers":            b.Config.ButlersToSpawn,
				}).Trace("Waiting for workerpool queue size to drop below butler count")
				time.Sleep(10 * time.Second)
			}

			b.WorkerPool.Submit(func() { b.msgHandler(msg) })
		case <-b.StopChan:
			b.interrupt = true
			log.WithFields(logrus.Fields{
				"component":          component,
				"Waiting queue size": b.WorkerPool.WaitingQueueSize(),
			}).Debug("Interrupt received.")

			// wait for currently running routines, pending tasks are abandoned.
			b.WorkerPool.Stop()
			break loop
		}

	}

	b.WorkerPool.StopWait()

	log.WithFields(logrus.Fields{
		"component": component,
		"Count":     b.Config.ButlersToSpawn,
	}).Debug("All butlers exited.")

}
