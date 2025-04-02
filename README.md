# Istio Usage Collector

This tool collects information from your Kubernetes clusters to analyze resource usage and help estimate the cost and resources required for migrating to ambient mesh.

## Description

The Istio Usage Collector is a Go implementation of the [`gather-cluster-info.sh` script](https://github.com/solo-io/scripts-public/blob/4c728ffca1babab525687063f99ac3e24fda3fa1/ambient-mesh/migration/v1/gather-cluster-info.sh). It gathers non-sensitive information about your Kubernetes cluster, including:

- Node information (instance type, region, zone, CPU, memory)
- Namespace information
- Pod and container counts
- Resource requests and usage
- Istio sidecar information

This data is collected into a JSON or YAML file that can be used for further analysis through our detailed migration estimator tool.

## Installation

### Building from source

1. Clone the repository:
   ```
   git clone https://github.com/solo-io/istio-usage-collector.git
   cd istio-usage-collector
   ```

2. Build the binary:
   ```
   make build
   ```

## Usage

```
./istio-usage-collector [subcommand] [flags]
```

### Subcommand

- `version`: Print the version information of the tool.

### Flags

- `--hide-names` or `-n`: Hide the names of the cluster and namespaces using a hash.
- `--continue` or `-c`: If the script was interrupted, continue processing from the last saved state.
- `--context` or `-k`: Kubernetes context to use (if not set, uses current context).
- `--output-dir` or `-d`: Directory to store output files (default: current directory).
- `--format` or `-f`: Output format (json, yaml/yml) (default: json).
- `--output-prefix` or `-p`: Custom prefix for output files (default: cluster name).
- `--help` or `-h`: Show help message.
- `--no-progress`: Disable the progress bar.
- `--debug`: Enable debug logs.

### Example

```bash
# Use the current context with default JSON output
./istio-usage-collector

# Get the version information
./istio-usage-collector version

# Use a specific context - this would scan `my-cluster` and be saved as ./my-cluster.json
./istio-usage-collector --context my-cluster

# Output in YAML format - this would be saved as ./<cluster>.yaml
./istio-usage-collector --format yaml

# Specify a custom output file prefix (name) - this would be saved as ./prod-cluster.json
./istio-usage-collector --output-prefix prod-cluster --format json

# Hide sensitive names and save to custom location - this would be saved as /reports/<hashed-cluster>.yaml
./istio-usage-collector --hide-names --output-dir /reports --format yaml

# Continue an interrupted collection
# Note that in order to successfully continue, the original flags must be passed as well.
./istio-usage-collector --continue
```

## Output

The tool generates output files containing the collected information:

- JSON format: `<cluster-name>.json` (default)
- YAML format: `<cluster-name>.yaml`

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
