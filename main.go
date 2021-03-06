package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/danmrichards/terraform-sandbox/providers/google"
	"github.com/danmrichards/terraform-sandbox/providers/google/auth"
	"github.com/hashicorp/terraform/config"
	"github.com/hashicorp/terraform/config/module"
	"github.com/hashicorp/terraform/helper/logging"
	"github.com/hashicorp/terraform/terraform"
	"google.golang.org/api/compute/v1"
)

const (
	jsonTpl = `
{
	"variable": {
		"machine_state": {
			"type": "string",
			"description": "State in which the machine should be. Allowed values: RUNNING, TERMINATED"
		},
		"project": {
			"type": "string",
			"description": "GCP project to create the compute instance in"
		},
		"credentials_file": {
			"type": "string",
			"description": "Path to a GCP service account credentials file"
		}
	},

	"provider": {
		"google": {
			"credentials": "${file(\"${var.credentials_file}\")}",
			"project": "${var.project}",
			"region": "europe-west1"
		}
	},

	"resource": {
		"google_compute_instance": {
			"default": {
				"name": "terraform-test-instance",
				"machine_type": "f1-micro",
				"zone": "europe-west1-b",
				"instance_state": "${var.machine_state}",
				"boot_disk": {
					"initialize_params": {
						"image": "debian-cloud/debian-9"
					}
				},
				"network_interface": {
					"network": "default",
					"access_config": {}
				}
			}
		}
	}
}`

	sf = "sandbox.tfstate"
)

var (
	destroy, forceStateRefresh         bool
	project, credentials, machineState string
)

func main() {
	// The TF packages are bad and link the test flags ALL THE TIME so we have
	// to use our own flag set.
	if err := flags(); err != nil {
		log.Fatal("could not parse flags: ", err)
	}

	// Set up TFs built in log writer.
	// This takes it's log level from a TF_LOG env var.
	// See: https://github.com/hashicorp/terraform/blob/master/helper/logging/logging.go
	logging.SetOutput()

	// Create a google compute API client.
	s, err := computeClient()
	if err != nil {
		log.Fatal("could not create google compute api client:", err)
	}

	gcp := google.Provider(s)

	// Parse the module JSON.
	tfm, err := tfModule()
	if err != nil {
		log.Fatal("could not create terraform module:", err)
	}

	// Load the current state of the resource, either by manually reloading
	// from the cloud provider or by using the state file on disk.
	state := terraform.NewState()
	if forceStateRefresh {
		state, err = loadState(gcp, tfm)
		if err != nil {
			log.Fatal("could not load state:", err)
		}
	} else {
		state, err = stateFromFile(sf)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Fatal("could not read current state:", err)
			}
		}
	}

	ctx, err := tfContext(gcp, state, tfm, destroy)
	if err != nil {
		log.Fatal("could not create context:", err)
	}

	if _, err = ctx.Refresh(); err != nil {
		log.Fatal("could not refresh:", err)
	}

	if _, err = ctx.Plan(); err != nil {
		log.Fatal("could not plan:", err)
	}

	state, err = ctx.Apply()
	if err != nil {
		log.Fatal("could not apply:", err)
	}

	if err = writeStateToFile(sf, state); err != nil {
		log.Fatal("write state to file:", err)
	}

	fmt.Println("apply complete")
	fmt.Println("state:", state)
}

func flags() error {
	f := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	f.BoolVar(
		&destroy,
		"destroy",
		false,
		"Destroy terraform managed infrastructure based on the plan",
	)
	f.BoolVar(
		&forceStateRefresh,
		"forcestaterefresh",
		false,
		"Force a load of state from the cloud provider instead of using the state file on disk",
	)
	f.StringVar(
		&project,
		"project",
		"",
		"GCP project to create the compute instance in",
	)
	f.StringVar(
		&credentials,
		"credentials",
		"service-account.json",
		"Path to a GCP service account credentials file",
	)
	f.StringVar(
		&machineState,
		"machinestate",
		"RUNNING",
		"State in which the machine should be. Allowed values: RUNNING, TERMINATED",
	)

	return f.Parse(os.Args[1:])
}

func computeClient() (*compute.Service, error) {
	var creds auth.Credentials
	sa, err := ioutil.ReadFile(credentials)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(sa, &creds); err != nil {
		return nil, err
	}

	cfg := creds.JWTConfig([]string{compute.ComputeScope})
	return compute.New(cfg.Client(context.Background()))
}

func loadState(p terraform.ResourceProvider, m *module.Tree) (*terraform.State, error) {
	ctxOpts := &terraform.ContextOpts{
		State:  terraform.NewState(),
		Module: m,
		Variables: map[string]interface{}{
			"project":          project,
			"credentials_file": credentials,
			"machine_state":    machineState,
		},
		ProviderResolver: terraform.ResourceProviderResolverFixed(
			map[string]terraform.ResourceProviderFactory{
				"google": terraform.ResourceProviderFactoryFixed(p),
			},
		),
	}

	ctx, err := terraform.NewContext(ctxOpts)
	if err != nil {
		return nil, err
	}

	return ctx.Import(&terraform.ImportOpts{
		Targets: []*terraform.ImportTarget{
			{
				Addr: "google_compute_instance.default",
				ID:   fmt.Sprintf("%s/europe-west1-b/terraform-test-instance", project),
			},
		},
	})
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

func tfContext(p terraform.ResourceProvider, s *terraform.State, m *module.Tree, destroy bool) (*terraform.Context, error) {
	ctxOpts := &terraform.ContextOpts{
		Destroy: destroy,
		State:   s,
		Variables: map[string]interface{}{
			"project":          project,
			"credentials_file": credentials,
			"machine_state":    machineState,
		},
		Module: m,
		ProviderResolver: terraform.ResourceProviderResolverFixed(
			map[string]terraform.ResourceProviderFactory{
				"google": terraform.ResourceProviderFactoryFixed(p),
			},
		),
	}

	ctx, err := terraform.NewContext(ctxOpts)
	if err != nil {
		return nil, err
	}

	return ctx, nil
}

func tfModule() (*module.Tree, error) {
	c, err := config.LoadJSON(json.RawMessage(jsonTpl))
	if err != nil {
		return nil, err
	}

	mod := module.NewTree("", c)
	if err = mod.Load(&module.Storage{Mode: module.GetModeNone}); err != nil {
		return nil, fmt.Errorf("failed to load the modules. %s", err)
	}

	if err = mod.Validate().Err(); err != nil {
		return nil, fmt.Errorf("failed terraform code validation. %s", err)
	}

	return mod, nil
}
