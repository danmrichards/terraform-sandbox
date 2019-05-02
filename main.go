package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/danmrichards/terraform-sandbox/providers/google"
	"github.com/hashicorp/terraform/config/module"
	"github.com/hashicorp/terraform/helper/logging"
	"github.com/hashicorp/terraform/terraform"
)

const (
	tpl = `
provider "google" {
  credentials = "${file("service-account.json")}"
  project     = "skyrocket-dev"
  region      = "europe-west1"
}

variable "machine_state" {
  type = "string"
  description = "State in which the machine should be. Allowed values: RUNNING, TERMINATED"
}

resource "google_compute_instance" "default" {
  name           = "terraform-test-instance"
  machine_type   = "n1-standard-2"
  zone           = "europe-west1-d"
  instance_state = "${var.machine_state}"

  boot_disk {
    initialize_params {
      image = "projects/multiplay-images-dev/global/images/mp-dev-linux-40-p"
      size  = 15
      type  = "pd-ssd"
    }
  }

  network_interface {
    network = "default"

    access_config {
      // Include this section to give the VM an external ip address
    }
  }
}`

	sf = "sandbox.tfstate"
)

var (
	destroy = flag.Bool("destroy", false, "destroy terraform managed infrastructure based on the plan")

	machineState = flag.String("machinestate", "RUNNING", "State in which the machine should be. Allowed values: RUNNING, TERMINATED")
)

func main() {
	flag.Parse()

	// Set up TFs built in log writer.
	// This takes it's log level from a TF_LOG env var.
	// See: https://github.com/hashicorp/terraform/blob/master/helper/logging/logging.go
	logging.SetOutput()

	gcp := google.Provider()
	state, err := stateFromFile(sf)
	if err != nil {
		if os.IsNotExist(err) {
			state = terraform.NewState()
		} else {
			log.Fatal("could not read current state:", err)
		}
	}

	ctx, err := tfContext(gcp, state, *destroy)
	if err != nil {
		log.Fatal("could not create context:", err)
	}

	if _, err := ctx.Refresh(); err != nil {
		log.Fatal("could not refresh:", err)
	}

	p, err := ctx.Plan()
	if err != nil {
		log.Fatal("could not plan:", err)
	}
	fmt.Println("plan:", p)

	//if _, err = ctx.Plan(); err != nil {
	//	log.Fatal("could not plan:", err)
	//}

	state, err = ctx.Apply()
	if err != nil {
		log.Fatal("could not apply:", err)
	}

	// TODO: How to wait for resource completion?

	if err = writeStateToFile(sf, state); err != nil {
		log.Fatal("write state to file:", err)
	}
}

func stateFromFile(name string) (*terraform.State, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return terraform.ReadState(file)
}

func writeStateToFile(name string, state *terraform.State) error {
	var buf bytes.Buffer
	if err := terraform.WriteState(state, &buf); err != nil {
		return err
	}
	return ioutil.WriteFile(name, buf.Bytes(), 0644)
}

func tfContext(p terraform.ResourceProvider, s *terraform.State, destroy bool) (*terraform.Context, error) {
	tfm, err := tfModule()
	if err != nil {
		return nil, err
	}

	// Create ContextOpts with the current state and variables to apply
	ctxOpts := &terraform.ContextOpts{
		Destroy: destroy,
		State:   s,
		Variables: map[string]interface{}{
			"machine_state": *machineState,
		},
		Module: tfm,
		ProviderResolver: terraform.ResourceProviderResolverFixed(map[string]terraform.ResourceProviderFactory{
			"google": terraform.ResourceProviderFactoryFixed(p),
		}),
	}

	ctx, err := terraform.NewContext(ctxOpts)
	if err != nil {
		return nil, err
	}

	return ctx, nil
}

func tfModule() (*module.Tree, error) {
	cfgPath, err := ioutil.TempDir("", ".sandbox")
	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(cfgPath)

	cfgFileName := filepath.Join(cfgPath, "main.tf")
	cfgFile, err := os.Create(cfgFileName)
	if err != nil {
		return nil, err
	}
	defer cfgFile.Close()

	if _, err = io.Copy(cfgFile, strings.NewReader(tpl)); err != nil {
		return nil, err
	}

	mod, err := module.NewTreeModule("", cfgPath)
	if err != nil {
		return nil, err
	}

	s := module.NewStorage(filepath.Join(cfgPath, "modules"), nil)
	s.Mode = module.GetModeNone

	if err := mod.Load(s); err != nil {
		return nil, fmt.Errorf("failed to load the modules. %s", err)
	}

	if err := mod.Validate().Err(); err != nil {
		return nil, fmt.Errorf("failed Terraform code validation. %s", err)
	}

	return mod, nil
}
