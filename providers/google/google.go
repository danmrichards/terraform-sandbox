package google

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/hashicorp/terraform/terraform"
	"github.com/terraform-providers/terraform-provider-google/google"
)

// TODO: const enum for instance states.

type wrappedGoogleProvider struct {
	terraform.ResourceProvider
}

// Provider returns a terraform.ResourceProvider.
func Provider() terraform.ResourceProvider {
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
	gcr.Read = readInstanceState(gcr.Read)

	// TODO: Instead of wrapping "Apply" could we wrap the resource update func
	// instead? The update func could then make the call to start/stop instance.

	// Overwrite with our customised version of the resource.
	rp.ResourcesMap["google_compute_instance"] = gcr

	return &wrappedGoogleProvider{
		ResourceProvider: rp,
	}
}

// Apply applies a diff to a specific resource and returns the new
// resource state along with an error.
//
// If the resource state given has an empty ID, then a new resource
// is expected to be created.
func (w *wrappedGoogleProvider) Apply(
	info *terraform.InstanceInfo,
	s *terraform.InstanceState,
	d *terraform.InstanceDiff) (*terraform.InstanceState, error) {

	if err := applyInstanceState(d); err != nil {
		return nil, err
	}

	fmt.Printf("info: %+v\n\n", info)
	fmt.Printf("state: %+v\n\n", s)
	fmt.Printf("diff: %+v\n\n", d)
	os.Exit(1)

	return w.ResourceProvider.Apply(info, s, d)
}

func readInstanceState(next schema.ReadFunc) schema.ReadFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		if err := next(d, meta); err != nil {
			return err
		}

		fmt.Println(strings.Repeat("*", 80))
		fmt.Println("this is where my state read call would go...if I had one!")
		fmt.Println(strings.Repeat("*", 80))

		// TODO: Call Google SDK to get instance state

		return nil
	}
}

func applyInstanceState(d *terraform.InstanceDiff) error {
	isa, ok := d.GetAttribute("instance_state")
	if !ok {
		return errors.New("missing instance_state attribute")
	}

	// No state change required.
	if isa.Old == isa.New {
		return nil
	}

	na, ok := d.GetAttribute("name")
	if !ok {
		return errors.New("missing name attribute")
	}
	name := na.Old
	if na.Old != na.New {
		name = na.New
	}

	fmt.Println(strings.Repeat("*", 80))
	fmt.Printf("instance %q: applying state change %q -> %q\n", name, isa.Old, isa.New)
	fmt.Println("this is where my state change call would go...if I had one!")
	fmt.Println(strings.Repeat("*", 80))

	// TODO: Call Google SDK to stop/start instance.

	// The standard TF provide does not support this resource, so get rid of it
	// now that we're done with it.
	d.DelAttribute("instance_state")

	return nil
}
