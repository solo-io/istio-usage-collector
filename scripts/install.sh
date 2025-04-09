#!/bin/sh
set -eu

DEFAULT_BUCKET="istio-usage-collector"

VERSION="${VERSION:-}"
GCS_BUCKET="${GCS_BUCKET:-$DEFAULT_BUCKET}"
BINARY_NAME="istio-usage-collector"

# Initialize override variables
OS_OVERRIDE=""
ARCH_OVERRIDE=""

# Parse command-line arguments
while [ $# -gt 0 ]; do
    case "$1" in
        --os)
            OS_OVERRIDE="$2"
            shift 2
            ;;
        --arch)
            ARCH_OVERRIDE="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--os <os>] [--arch <arch>] [--version <version>]"
            exit 1
            ;;
    esac
done

if [ "${VERSION}" = "latest" ] || [ -z "${VERSION}" ]; then
  echo "Finding latest version..."
  # Fetch available versions and filter out versions that contains a hyphen (e.g. -rc or -beta)
  if ! AVAILABLE_VERSIONS=$(curl -fsSL https://storage.googleapis.com/"${GCS_BUCKET}"/releases.txt | grep -E -v '\-'); then
    echo "Error: Could not fetch list of available versions from GCS." >&2
    echo "Bucket: ${GCS_BUCKET}" >&2
    exit 1
  fi
  if [ -z "$AVAILABLE_VERSIONS" ]; then
    echo "Error: No stable versions found in releases.txt." >&2
    exit 1
  fi
  # Use the first line as the latest version
  VERSION=$(echo "$AVAILABLE_VERSIONS" | head -n1)
  echo "Latest version is: ${VERSION}"
else
  echo "Using specified version: ${VERSION}"
fi


# Get the OS of the machine
OS=""
if [ -n "$OS_OVERRIDE" ]; then
  echo "Using OS override: ${OS_OVERRIDE}"
  OS="$OS_OVERRIDE"
else
  echo "Using OSTYPE to determine OS: ${OSTYPE}"
  if [[ "$OSTYPE" == "msys"* || "$OSTYPE" == "cygwin"* || "$OSTYPE" == "mingw"* ]]; then
    OS=windows
  elif [[ "$OSTYPE" == "darwin"* ]]; then
    OS=darwin
  elif [[ "$OSTYPE" == "linux"* ]]; then
    OS=linux
  else
    echo "Running on an unknown operating system based on OSTYPE"
  fi

  # If the OS is still not set (not found in the OSTYPE check), use uname to determine OS
  if [ -z "$OS" ]; then
    echo "Using uname to determine OS"

    detected_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    # if its cygwun, mysys, or mingw, set it to windows; otherwise, if its not darwin, set it to linux
    if [ "$detected_os" = "cygwin" ] || [ "$detected_os" = "msys" ] || [ "$detected_os" = "mingw" ]; then
      OS=windows
    elif [ "$detected_os" != "darwin" ]; then
      OS=linux
    else # It must be darwin if not the others
      OS=darwin
    fi
  fi
fi

# Get the architecture of the machine
GOARCH=""
if [ -n "$ARCH_OVERRIDE" ]; then
  echo "Using ARCH override: ${ARCH_OVERRIDE}"
  GOARCH="$ARCH_OVERRIDE"
else
  detected_arch="$(uname -m)"
  echo "Using uname -m to determine ARCH: ${detected_arch}"
  if [ "$detected_arch" = "aarch64" ] || [ "$detected_arch" = "arm64" ]; then
    GOARCH=arm64
  else
    GOARCH=amd64
  fi
fi

echo "OS: ${OS}, Arch: ${GOARCH}, Version: ${VERSION}"

filename="${BINARY_NAME}-${OS}-${GOARCH}"
url="https://storage.googleapis.com/${GCS_BUCKET}/${VERSION}/${filename}"

echo "Attempting to download ${BINARY_NAME} version ${VERSION} from ${url}"

if ! curl --output /dev/null --silent --head --fail "${url}"; then
  echo "Error: File not found at ${url}" >&2
  echo "Please check the version, OS, or architecture." >&2
  exit 1
fi

# Download binary
echo "Downloading ${filename}..."
if ! curl -fsSL -o "${filename}" "${url}"; then
  echo "Error: Failed to download binary from ${url}" >&2
  exit 1
fi

# Calculate local checksum
echo "Calculating local checksum..."
local_checksum=$(openssl dgst -sha256 "${filename}" | awk '{ print $2 }')

# Fetch remote checksum content and validate
echo "Fetching remote checksum..."
if ! remote_checksum_content=$(curl -fsSL "${url}.sha256"); then
    echo "Warning: Failed to fetch remote checksum file from ${url}.sha256. Skipping verification." >&2
else
    echo "Validating checksum..."
    expected_checksum=$(echo "$remote_checksum_content" | awk '{ print $1 }')

    if [ -z "$expected_checksum" ]; then
      echo "Error: Could not extract checksum from remote file content at ${url}.sha256. Skipping verification." >&2
    elif [ "$local_checksum" != "$expected_checksum" ]; then
      echo "Error: Checksum validation failed." >&2
      echo "Expected: ${expected_checksum}" >&2
      echo "Got:      ${local_checksum}" >&2
      rm "${filename}"
      exit 1
    else
      echo "Checksum valid."
    fi
fi

# Ensure the binary is executable
chmod +x "${filename}"

# Verify execution (optional, assumes --version flag)
echo "Verifying installation..."
if ! ./"${filename}" --version > /dev/null 2>&1; then
    echo "Warning: Could not verify ${filename} execution. It might be corrupted or lack a '--version' flag." >&2
    echo "You may need to manually check the binary." >&2
else
    echo "Verification successful."
fi

echo ""
echo "${BINARY_NAME} version ${VERSION} was successfully downloaded to the current directory as '${filename}' 🎉"
echo ""
echo "You can run it directly using:"
echo "  ./${filename} [command]"
echo ""
