# shellcheck shell=bash
# Shared helpers for the certs:* mise file tasks.
#
# This file is intentionally NOT a task: it lives outside mise/tasks/ so mise
# does not pick it up as a runnable task. The certs tasks source it via
# "${MISE_PROJECT_ROOT}/mise/lib/certs.sh".
#
# Source it from a bash task with:
#   # shellcheck source=../../lib/certs.sh
#   source "${MISE_PROJECT_ROOT:?}/mise/lib/certs.sh"
#
# All certs are leaf certificates valid for localhost / 127.0.0.1 / ::1, which
# covers both the direct-TLS api (https://localhost:8443) and the Caddy reverse
# proxy (https://localhost:9443) used by the `tls` Compose profile. The server
# leaf also carries the in-network hostname `api-mtls` so a re-encrypting proxy
# (the `mtls` profile) can verify the upstream by name. Each directory holds a
# self-contained set:
#   devopsbin.pem            server leaf certificate (chain)
#   devopsbin-key.pem        its private key
#   devopsbin-client.pem     client leaf certificate (for mutual TLS)
#   devopsbin-client-key.pem its private key
#   rootCA.pem               the issuing root CA (for --cacert / verification)

# certs_present <certs_dir>
#
# Succeed when the full server + client + CA set already exists in certs_dir.
certs_present() {
    local d="$1"
    [ -f "${d}/devopsbin.pem" ] &&
        [ -f "${d}/devopsbin-key.pem" ] &&
        [ -f "${d}/devopsbin-client.pem" ] &&
        [ -f "${d}/devopsbin-client-key.pem" ] &&
        [ -f "${d}/rootCA.pem" ]
}

# gen_ephemeral_certs <certs_dir> <force>
#
# Generate the server + client leaves into certs_dir using a throwaway mkcert CA
# created in a temp directory that is removed before returning. The CA therefore
# never lands in $HOME or any system trust store, the task needs no sudo, and it
# runs unchanged in CI. Because the CA is ephemeral, the leaves and rootCA.pem
# are always written together so the set always matches.
#
# Idempotent: if the set already exists and force is not "true", it is left
# as-is (so install-issued, browser-trusted certs survive a later regenerate).
gen_ephemeral_certs() {
    local certs_dir="$1" force="$2"
    mkdir -p "$certs_dir"

    if [ "$force" != "true" ] && certs_present "$certs_dir"; then
        echo "Certificates already present (use --force to regenerate): ${certs_dir}/devopsbin.pem"
        return 0
    fi

    local caroot
    caroot="$(mktemp -d)"

    # The first mkcert command creates the CA at CAROOT, then signs the server
    # leaf with it; the second reuses the same CA to sign a client leaf. Copy
    # that rootCA.pem out for verification, then drop the temp CA (including its
    # private key) whether or not generation succeeded.
    echo "Generating server leaf certificate -> ${certs_dir}/devopsbin.pem"
    if CAROOT="$caroot" mkcert \
        -cert-file "${certs_dir}/devopsbin.pem" \
        -key-file "${certs_dir}/devopsbin-key.pem" \
        localhost 127.0.0.1 ::1 api-mtls &&
        echo "Generating client leaf certificate -> ${certs_dir}/devopsbin-client.pem" &&
        CAROOT="$caroot" mkcert \
            -client \
            -cert-file "${certs_dir}/devopsbin-client.pem" \
            -key-file "${certs_dir}/devopsbin-client-key.pem" \
            localhost 127.0.0.1 ::1 caddy-mtls api-mtls; then
        cp "${caroot}/rootCA.pem" "${certs_dir}/rootCA.pem"
        rm -rf "$caroot"
    else
        rm -rf "$caroot"
        return 1
    fi

    echo "Done. ${certs_dir} now contains:"
    ls -1 "${certs_dir}"/*.pem
}
