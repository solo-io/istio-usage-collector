# Ambient Mesh Migration Estimator Snapshot

This tool collects information from your Kubernetes clusters to analyze resource usage and help estimate the cost and resources required for migrating to ambient mesh.

## Description

The Ambient Migration Estimator Snapshot is a Go implementation of the [`gather-cluster-info.sh` script](https://github.com/solo-io/scripts-public/blob/4c728ffca1babab525687063f99ac3e24fda3fa1/ambient-mesh/migration/v1/gather-cluster-info.sh). It gathers non-sensitive information about your Kubernetes cluster, including:

- Node information (instance type, region, zone, CPU, memory)
- Namespace information
- Pod and container counts
- Resource requests and usage
- Istio sidecar information

This data is collected into a JSON file that can be used for further analysis through our detailed migration estimator tool

## Installation

### Building from source

1. Clone the repository:
   ```
   git clone https://github.com/solo-io/ambient-migration-estimator-cluster-gatherer.git
   cd ambient-migration-estimator-cluster-gatherer
   ```

2. Build the binary:
   ```
   make build
   ```

## Usage

```
./ambient-migration-estimator [flags]
```

### Flags

- `--hide-names` or `-hn`: Hide the names of the cluster and namespaces using a hash.
- `--continue` or `-c`: If the script was interrupted, continue processing from the last saved state.
- `--help` or `-h`: Show help message.
- `--version` or `-v-`: Show version and build information.

### Environment Variables

- `CONTEXT`: Kubernetes context to use. If not set, the current context will be used.
- `KUBECONFIG`: Path to the kubeconfig file. If not set, `~/.kube/config` will be used.

### Example

```bash
# Use the current context
./ambient-migration-estimator

# Use a specific context
CONTEXT="my-cluster" ./ambient-migration-estimator

# Hide sensitive names
./ambient-migration-estimator --hide-names

# Continue an interrupted collection
./ambient-migration-estimator --continue
```

## Output

The tool generates a `cluster_info.json` file containing the collected information. This file has the following structure:

```json
{
  "name": "cluster-name",
  "namespaces": {
    "namespace1": {
      "pods": 10,
      "is_istio_injected": true,
      "resources": {
        "regular": {
          "containers": 15,
          "request": {
            "cpu": 2.5,
            "memory_gb": 4.0
          },
          "actual": {
            "cpu": 1.2,
            "memory_gb": 2.1
          }
        },
        "istio": {
          "containers": 10,
          "request": {
            "cpu": 1.0,
            "memory_gb": 1.5
          },
          "actual": {
            "cpu": 0.5,
            "memory_gb": 0.8
          }
        }
      }
    }
  },
  "nodes": {
    "node1": {
      "instance_type": "m5.large",
      "region": "us-east-1",
      "zone": "us-east-1a",
      "resources": {
        "capacity": {
          "cpu": 2,
          "memory_gb": 8
        },
        "actual": {
          "cpu": 1.5,
          "memory_gb": 6.0
        }
      }
    }
  },
  "has_metrics": true
}
```