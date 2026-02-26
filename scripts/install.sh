#!/usr/bin/env bash
set -euo pipefail

main() {

REPO="leeovery/portal"
BINARY_NAME="portal"
GITHUB_API="${GITHUB_API:-https://api.github.com/repos/${REPO}/releases/latest}"

# --- Testable functions ---

detect_os() {
    local uname_s="${PORTAL_TEST_UNAME_S:-$(uname -s)}"
    case "${uname_s}" in
        Linux)  echo "linux" ;;
        Darwin) echo "darwin" ;;
        *)
            echo "Error: Unsupported operating system: ${uname_s}. This installer supports Linux and macOS only." >&2
            return 1
            ;;
    esac
}

detect_arch() {
    local uname_m="${PORTAL_TEST_UNAME_M:-$(uname -m)}"
    case "${uname_m}" in
        x86_64)  echo "amd64" ;;
        aarch64) echo "arm64" ;;
        arm64)   echo "arm64" ;;
        *)
            echo "Error: unsupported architecture '${uname_m}'. Supported: x86_64, aarch64, arm64." >&2
            return 1
            ;;
    esac
}

resolve_version() {
    if [[ -n "${PORTAL_TEST_VERSION:-}" ]]; then
        echo "${PORTAL_TEST_VERSION}"
        return 0
    fi
    local version
    version=$(curl -fsSL "${GITHUB_API}" | grep '"tag_name"' | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
    if [[ -z "${version}" ]]; then
        echo "Error: could not resolve latest version from GitHub API." >&2
        return 1
    fi
    echo "${version}"
}

construct_url() {
    local version="$1"
    local os="$2"
    local arch="$3"
    local version_no_v="${version#v}"
    echo "https://github.com/${REPO}/releases/download/${version}/${BINARY_NAME}_${version_no_v}_${os}_${arch}.tar.gz"
}

install_macos() {
    if ! command -v brew &> /dev/null; then
        echo "Please install via Homebrew:" >&2
        echo "brew install leeovery/tools/portal" >&2
        return 1
    fi

    brew install leeovery/tools/portal

    echo "Successfully installed ${BINARY_NAME} via Homebrew."
}

select_install_dir() {
    local primary="${PORTAL_INSTALL_DIR:-/usr/local/bin}"
    local fallback="${PORTAL_FALLBACK_DIR:-${HOME}/.local/bin}"

    if [[ -d "${primary}" && -w "${primary}" ]]; then
        echo "${primary}"
        return 0
    fi

    mkdir -p "${fallback}"
    echo "${fallback}"

    # Print PATH warning if fallback is not in PATH.
    # PORTAL_TEST_PATH allows tests to override PATH for this check without
    # breaking command availability.
    local check_path="${PORTAL_TEST_PATH:-${PATH}}"
    case ":${check_path}:" in
        *":${fallback}:"*) ;;
        *)
            echo "" >&2
            echo "WARNING: ${fallback} is not in your PATH." >&2
            echo "Add it to your PATH by adding this to your shell profile:" >&2
            echo "  export PATH=\"${fallback}:\$PATH\"" >&2
            ;;
    esac
}

# --- Test mode dispatch ---

if [[ -n "${PORTAL_TEST_MODE:-}" ]]; then
    case "${PORTAL_TEST_MODE}" in
        detect_os)
            detect_os
            exit $?
            ;;
        detect_arch)
            detect_os > /dev/null
            detect_arch
            exit $?
            ;;
        construct_url)
            os=$(detect_os)
            arch=$(detect_arch)
            version=$(resolve_version)
            construct_url "${version}" "${os}" "${arch}"
            exit 0
            ;;
        select_install_dir)
            select_install_dir
            exit 0
            ;;
        resolve_version)
            resolve_version
            exit $?
            ;;
        install_macos)
            install_macos
            exit $?
            ;;
        full_install)
            # Fall through to main install flow below.
            ;;
        *)
            echo "Error: unknown test mode '${PORTAL_TEST_MODE}'" >&2
            exit 1
            ;;
    esac
fi

# --- Main install flow ---

echo "Installing ${BINARY_NAME}..."

OS=$(detect_os)

if [[ "${OS}" == "darwin" ]]; then
    install_macos
    exit 0
fi

ARCH=$(detect_arch)
VERSION=$(resolve_version)
URL=$(construct_url "${VERSION}" "${OS}" "${ARCH}")

TMPDIR_INSTALL=$(mktemp -d)
trap 'rm -rf "${TMPDIR_INSTALL}"' EXIT

if [[ "${PORTAL_TEST_ECHO_TMPDIR:-}" == "1" ]]; then
    echo "PORTAL_TMPDIR=${TMPDIR_INSTALL}"
fi

if [[ -n "${PORTAL_TEST_TARBALL:-}" ]]; then
    cp "${PORTAL_TEST_TARBALL}" "${TMPDIR_INSTALL}/${BINARY_NAME}.tar.gz"
else
    echo "Downloading ${URL}..."
    curl -fsSL "${URL}" -o "${TMPDIR_INSTALL}/${BINARY_NAME}.tar.gz"
fi

if [[ ! -s "${TMPDIR_INSTALL}/${BINARY_NAME}.tar.gz" ]]; then
    echo "Error: Downloaded archive is empty or missing." >&2
    exit 1
fi

if ! tar xzf "${TMPDIR_INSTALL}/${BINARY_NAME}.tar.gz" -C "${TMPDIR_INSTALL}"; then
    echo "Error: Failed to extract archive. The download may be corrupt." >&2
    exit 1
fi

if [[ ! -f "${TMPDIR_INSTALL}/${BINARY_NAME}" ]]; then
    echo "Error: Binary 'portal' not found in archive." >&2
    exit 1
fi

INSTALL_DIR=$(select_install_dir)

cp "${TMPDIR_INSTALL}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

echo "Successfully installed ${BINARY_NAME} ${VERSION} to ${INSTALL_DIR}/${BINARY_NAME}"

}

main "$@"
