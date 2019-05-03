package google

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-google/google"
	"google.golang.org/api/compute/v1"
)

// TODO: const enum for instance states.

type wrappedGoogleProvider struct {
	terraform.ResourceProvider
}

// Provider returns a terraform.ResourceProvider.
func Provider(cs *compute.Service) terraform.ResourceProvider {
	rp := google.Provider().(*schema.Provider)

	// Get the resource so we can extend it.
	gcr := rp.ResourcesMap["google_compute_instance"]

	// Add a new resource schema for handling instance state.
	gcr.Schema["instance_state"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Default:  "RUNNING",
		ValidateFunc: validation.StringInSlice([]string{
			"RUNNING",
			"TERMINATED",
		}, false),
	}

	// Wrap the resource reader so we can get the instance state.
	gcr.Read = readInstanceState(cs, gcr.Read)

	// Wrap the resource updater so we can set the instance state.
	gcr.Update = updateInstanceState(cs, gcr.Update)

	// Overwrite with our customised version of the resource.
	rp.ResourcesMap["google_compute_instance"] = gcr

	return &wrappedGoogleProvider{
		ResourceProvider: rp,
	}
}

// readInstanceState returns a read function for google compute instances that
// will retrieve the state of an instance after the upstream rf has been
// executed successfully.
func readInstanceState(service *compute.Service, rf schema.ReadFunc) schema.ReadFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		// Run the upstream read.
		if err := rf(d, meta); err != nil {
			return err
		}

		config := meta.(*google.Config)

		project, err := getProject(d, config)
		if err != nil {
			return err
		}
		zone, err := getZone(d, config)
		if err != nil {
			return err
		}

		// Get the instance info again (next already does this) so we can
		// populate the instance_state (next will not do this).
		_, state, err := getInstance(service, project, zone, d.Id())()
		if err != nil {
			// TODO: Handle not found errors.
			return err
		}

		// Update the state. We're grouping states here because we just care
		// if the machine is online, in any way, or not.
		switch state {
		case "PROVISIONING", "STAGING", "RUNNING":
			if err = d.Set("instance_state", "RUNNING"); err != nil {
				return err
			}
		default:
			if err = d.Set("instance_state", "TERMINATED"); err != nil {
				return err
			}
		}

		return nil
	}
}

// readInstanceState returns an update function for google compute instances
// that will update the state of an instance before the upstream uf.
func updateInstanceState(service *compute.Service, uf schema.UpdateFunc) schema.UpdateFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		// Only update the state if the diff indicates it has changed.
		if d.HasChange("instance_state") {
			config := meta.(*google.Config)

			project, err := getProject(d, config)
			if err != nil {
				return err
			}
			zone, err := getZone(d, config)
			if err != nil {
				return err
			}

			switch d.Get("instance_state") {
			case "RUNNING":
				if err := startInstance(service, project, zone, d.Id()); err != nil {
					return err
				}
			case "TERMINATED":
				if err := stopInstance(service, project, zone, d.Id()); err != nil {
					return err
				}
			}
		}

		// Run the upstream update.
		return uf(d, meta)
	}
}

func getInstance(service *compute.Service, project, zone, id string) resource.StateRefreshFunc {
	return func() (result interface{}, state string, err error) {
		i, err := service.Instances.Get(project, zone, id).Do()
		if err != nil {
			return nil, "", err
		}

		return i, i.Status, nil
	}
}

func startInstance(service *compute.Service, project, zone, id string) error {
	if _, err := service.Instances.Start(project, zone, id).Do(); err != nil {
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"PROVISIONING", "STAGING", "TERMINATED"},
		Target:     []string{"RUNNING"},
		Refresh:    getInstance(service, project, zone, id),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err := stateConf.WaitForState()
	return err
}

func stopInstance(service *compute.Service, project, zone, id string) error {
	if _, err := service.Instances.Stop(project, zone, id).Do(); err != nil {
		return err
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"PROVISIONING", "STAGING", "RUNNING", "STOPPING"},
		Target:     []string{"TERMINATED"},
		Refresh:    getInstance(service, project, zone, id),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err := stateConf.WaitForState()
	return err
}

func getZone(d google.TerraformResourceData, config *google.Config) (string, error) {
	res, ok := d.GetOk("zone")
	if !ok {
		if config.Zone != "" {
			return config.Zone, nil
		}
		return "", fmt.Errorf("cannot determine zone: set in this resource, or set provider-level zone")
	}

	parts := strings.Split(res.(string), "/")
	return parts[len(parts)-1], nil
}

func getProject(d google.TerraformResourceData, config *google.Config) (string, error) {
	res, ok := d.GetOk("project")
	if ok {
		return res.(string), nil
	}
	if config.Project != "" {
		return config.Project, nil
	}
	return "", errors.New("project: required field is not set")
}
