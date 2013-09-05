package digitalocean

import (
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	"log"
	"time"
)

type stepShutdown struct{}

func (s *stepShutdown) Run(state multistep.StateBag) multistep.StepAction {
	client := state.Get("client").(*DigitalOceanClient)
	c := state.Get("config").(config)
	ui := state.Get("ui").(packer.Ui)
	dropletId := state.Get("droplet_id").(uint)

	// Gracefully power off the droplet. We have to retry this a number
	// of times because sometimes it says it completed when it actually
	// did absolutely nothing (*ALAKAZAM!* magic!). We give up after
	// a pretty arbitrary amount of time.
	ui.Say("Gracefully shutting down droplet...")
	err := client.ShutdownDroplet(dropletId)
	if err != nil {
		// If we get an error the first time, actually report it
		err := fmt.Errorf("Error shutting down droplet: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// A channel we use as a flag to end our goroutines
	done := make(chan struct{})
	shutdownRetryDone := make(chan struct{})

	// Make sure we wait for the shutdown retry goroutine to end
	// before moving on.
	defer func() {
		close(done)
		<-shutdownRetryDone
	}()

	// Start a goroutine that just keeps trying to shut down the
	// droplet.
	go func() {
		defer close(shutdownRetryDone)

		for attempts := 2; attempts > 0; attempts++ {
			log.Printf("ShutdownDroplet attempt #%d...", attempts)
			err := client.ShutdownDroplet(dropletId)
			if err != nil {
				log.Printf("Shutdown retry error: %s", err)
			}

			select {
			case <-done:
				return
			case <-time.After(20 * time.Second):
				// Retry!
			}
		}
	}()

	err = waitForDropletState("off", dropletId, client, 2 * time.Minute)
	if err != nil {
		log.Printf("Error waiting for graceful off: %s", err)
	}

	return multistep.ActionContinue
}

func (s *stepShutdown) Cleanup(state multistep.StateBag) {
	// no cleanup
}
