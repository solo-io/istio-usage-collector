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
   git clone https://github.com/solo-io/ambient-migration-estimator-snapshot.git
   cd ambient-migration-estimator-snapshot
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

- `--hide-names` or `-n`: Hide the names of the cluster and namespaces using a hash.
- `--continue` or `-c`: If the script was interrupted, continue processing from the last saved state.
- `--context` or `-k`: Kubernetes context to use (if not set, uses current context).
- `--output-dir` or `-d`: Directory to store output files (default: current directory).
- `--format` or `-f`: Output format (json, yaml/yml) (default: json).
- `--output-prefix` or `-p`: Custom prefix for output files (default: cluster name).
- `--help` or `-h`: Show help message.

### Example

```bash
# Use the current context with default JSON output
./ambient-migration-estimator

# Use a specific context
./ambient-migration-estimator --context=my-cluster

# Output in YAML format
./ambient-migration-estimator --format=yaml

# Output in CSV format to a specific directory
./ambient-migration-estimator --format=csv --output-dir=/tmp/cluster-data

# Specify a custom output file prefix (name)
./ambient-migration-estimator --output-prefix=prod-cluster --format=json

# Hide sensitive names and save to custom location
./ambient-migration-estimator --hide-names --output-dir=/reports --format=yaml

# Continue an interrupted collection
./ambient-migration-estimator --continue

# Get the version information
./ambient-migration-estimator version
```

## Output

The tool generates output files containing the collected information:

- JSON format: `<cluster-name>.json` (default)
- YAML format: `<cluster-name>.yaml`
- CSV format:
  - `<cluster-name>-namespaces.csv` - Contains namespace-level information
  - `<cluster-name>-nodes.csv` - Contains node-level information

### JSON Output Structure

When using the default JSON format, the file has the following structure:

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