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
# proxy (https://localhost:9443) used by the `tls` Compose profile. Each
# directory holds a self-contained trio:
#   devopsbin.pem      leaf certificate (chain)
#   devopsbin-key.pem  its private key
#   rootCA.pem         the issuing root CA (for --cacert / verification)

# certs_present <certs_dir>
#
# Succeed when the full leaf+key+CA trio already exists in certs_dir.
certs_present() {
    local d="$1"
    [ -f "${d}/devopsbin.pem" ] &&
        [ -f "${d}/devopsbin-key.pem" ] &&
        [ -f "${d}/rootCA.pem" ]
}

# gen_ephemeral_certs <certs_dir> <force>
#
# Generate the leaf trio into certs_dir using a throwaway mkcert CA created in a
# temp directory that is removed before returning. The CA therefore never lands
# in $HOME or any system trust store, the task needs no sudo, and it runs
# unchanged in CI. Because the CA is ephemeral, the leaf and rootCA.pem are
# always written together so the pair always matches.
#
# Idempotent: if the trio already exists and force is not "true", it is left
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

    # The first mkcert command creates the CA at CAROOT, then signs the leaf
    # with it; copy that rootCA.pem out for verification, then drop the temp CA
    # (including its private key) whether or not generation succeeded.
    echo "Generating leaf certificate -> ${certs_dir}/devopsbin.pem"
    if CAROOT="$caroot" mkcert \
        -cert-file "${certs_dir}/devopsbin.pem" \
        -key-file "${certs_dir}/devopsbin-key.pem" \
        localhost 127.0.0.1 ::1; then
        cp "${caroot}/rootCA.pem" "${certs_dir}/rootCA.pem"
        rm -rf "$caroot"
    else
        rm -rf "$caroot"
        return 1
    fi

    echo "Done. ${certs_dir} now contains:"
    ls -1 "${certs_dir}"/*.pem
}
