# Terraform Sandbox
A proof-of-concept CLI which programmatically interacts with the [terraform
golang package](https://github.com/hashicorp/terraform).

The tool is capable of creating cloud VM, destroying a cloud VM and
altering the running state of the VM (start/stop)

## Installation
Build the binaries:
```bash
$ make
```

## Usage
```bash
Usage of tfsandbox:
  -credentials string
    	Path to a GCP service account credentials file (default "service-account.json")
  -destroy
    	Destroy terraform managed infrastructure based on the plan
  -forcestaterefresh
    	Force a load of state from the cloud provider instead of using the state file on disk
  -machinestate string
    	State in which the machine should be. Allowed values: RUNNING, TERMINATED (default "RUNNING")
  -project string
    	GCP project to create the compute instance in
```
