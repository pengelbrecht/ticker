#!/bin/sh
# Ticker installation script
# Install: curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticker/main/scripts/install.sh | sh
# Upgrade: ticker upgrade (or re-run the install script)

set -e

REPO="pengelbrecht/ticker"
BINARY_NAME="ticker"
INSTALL_DIR="${TICKER_INSTALL_DIR:-}"

# Colors (only if terminal supports it)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

info() {
    printf "${BLUE}info${NC}: %s\n" "$1"
}

success() {
    printf "${GREEN}success${NC}: %s\n" "$1"
}

warn() {
    printf "${YELLOW}warn${NC}: %s\n" "$1"
}

error() {
    printf "${RED}error${NC}: %s\n" "$1" >&2
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux" ;;
        Darwin*)    echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)          error "Unsupported operating system: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        armv7l)         echo "arm" ;;
        i386|i686)      echo "386" ;;
        *)              error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Determine install directory
get_install_dir() {
    if [ -n "$INSTALL_DIR" ]; then
        echo "$INSTALL_DIR"
        return
    fi

    # Check if we can write to /usr/local/bin
    if [ -w "/usr/local/bin" ]; then
        echo "/usr/local/bin"
        return
    fi

    # Fall back to ~/.local/bin
    local_bin="$HOME/.local/bin"
    mkdir -p "$local_bin"
    echo "$local_bin"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Get latest release version from GitHub
get_latest_version() {
    if command_exists curl; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command_exists wget; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Get currently installed version
get_installed_version() {
    if command_exists "$BINARY_NAME"; then
        "$BINARY_NAME" --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo ""
    else
        echo ""
    fi
}

# Download file
download() {
    url="$1"
    output="$2"

    if command_exists curl; then
        curl -fsSL "$url" -o "$output"
    elif command_exists wget; then
        wget -q "$url" -O "$output"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Main installation logic
main() {
    info "Installing ${BINARY_NAME}..."

    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "Detected platform: ${OS}/${ARCH}"

    # Get versions
    LATEST_VERSION=$(get_latest_version)
    if [ -z "$LATEST_VERSION" ]; then
        error "Could not determine latest version. Check your internet connection or GitHub API limits."
    fi
    info "Latest version: ${LATEST_VERSION}"

    INSTALLED_VERSION=$(get_installed_version)
    if [ -n "$INSTALLED_VERSION" ]; then
        if [ "v${INSTALLED_VERSION}" = "$LATEST_VERSION" ] || [ "$INSTALLED_VERSION" = "$LATEST_VERSION" ]; then
            success "${BINARY_NAME} ${LATEST_VERSION} is already installed and up to date"
            exit 0
        fi
        info "Upgrading from ${INSTALLED_VERSION} to ${LATEST_VERSION}"
    fi

    # Determine install location
    INSTALL_PATH=$(get_install_dir)
    info "Install location: ${INSTALL_PATH}"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    # Construct download URL
    # Expected format: ticker_<version>_<os>_<arch>.tar.gz
    VERSION_NUM=$(echo "$LATEST_VERSION" | sed 's/^v//')
    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_VERSION}/${ARCHIVE_NAME}"

    info "Downloading ${DOWNLOAD_URL}..."
    download "$DOWNLOAD_URL" "${TMP_DIR}/${ARCHIVE_NAME}"

    # Extract
    info "Extracting..."
    tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"

    # Install binary
    if [ -w "$INSTALL_PATH" ]; then
        mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_PATH}/${BINARY_NAME}"
        chmod +x "${INSTALL_PATH}/${BINARY_NAME}"
    else
        info "Requesting sudo access to install to ${INSTALL_PATH}..."
        sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_PATH}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_PATH}/${BINARY_NAME}"
    fi

    # Verify installation
    if [ -x "${INSTALL_PATH}/${BINARY_NAME}" ]; then
        success "${BINARY_NAME} ${LATEST_VERSION} installed successfully to ${INSTALL_PATH}/${BINARY_NAME}"
    else
        error "Installation failed"
    fi

    # Check if install dir is in PATH
    case ":$PATH:" in
        *":${INSTALL_PATH}:"*) ;;
        *)
            warn "${INSTALL_PATH} is not in your PATH"
            echo ""
            echo "Add it to your shell profile:"
            echo ""
            echo "  # For bash (~/.bashrc or ~/.bash_profile)"
            echo "  export PATH=\"\$PATH:${INSTALL_PATH}\""
            echo ""
            echo "  # For zsh (~/.zshrc)"
            echo "  export PATH=\"\$PATH:${INSTALL_PATH}\""
            echo ""
            echo "  # For fish (~/.config/fish/config.fish)"
            echo "  set -gx PATH \$PATH ${INSTALL_PATH}"
            echo ""
            ;;
    esac

    success "Run '${BINARY_NAME} --help' to get started"
}

main "$@"
