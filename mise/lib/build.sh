# shellcheck shell=bash
# Shared helpers for the build-related mise file tasks.
#
# This file is intentionally NOT a task: it lives outside mise/tasks/ so mise
# does not pick it up as a runnable task. The build tasks source it via
# "${MISE_PROJECT_ROOT}/mise/lib/build.sh", which resolves both on a developer
# host and inside the Docker build (mise sets MISE_PROJECT_ROOT to the config
# root, and the Dockerfile copies the whole mise/ tree).
#
# Source it from a bash task with:
#   # shellcheck source=../../lib/build.sh
#   source "${MISE_PROJECT_ROOT:?}/mise/lib/build.sh"

# quote ARG
#
# Print ARG, single-quoting it only when it contains characters that are
# special to the shell, so common values stay unquoted and easy to read while
# the result remains safe to copy and paste back into a shell.
quote() {
    case $1 in
        '' | *[!A-Za-z0-9_.,:/=+@%-]*) printf "'%s'" "${1//\'/\'\\\'\'}" ;;
        *) printf '%s' "$1" ;;
    esac
}

# print_cmd ARG...
#
# Pretty-print a command for --dry-run output. The leading program/subcommand
# words (and any leading VAR=value env assignments) go on the first line, then
# each flag is printed on its own line together with its value, joined by
# backslash continuations and indented by four spaces.
print_cmd() {
    local args=("$@") lines=() i=0 n=$# line=""

    # Group the leading program/subcommand words (anything that is not a flag)
    # onto the first line.
    while [ "$i" -lt "$n" ] && [[ ${args[$i]} != -* ]]; do
        line+="${line:+ }$(quote "${args[$i]}")"
        i=$((i + 1))
    done
    [ -n "$line" ] && lines+=("$line")

    # Keep each value-taking option on the same line as its value.
    while [ "$i" -lt "$n" ]; do
        line="$(quote "${args[$i]}")"
        if [[ ${args[$i]} == -* && ${args[$i]} != *=* &&
            $((i + 1)) -lt $n && ${args[$((i + 1))]} != -* ]]; then
            i=$((i + 1))
            line+=" $(quote "${args[$i]}")"
        fi
        lines+=("$line")
        i=$((i + 1))
    done

    local last=$((${#lines[@]} - 1))
    for i in "${!lines[@]}"; do
        if [ "$i" -eq "$last" ]; then
            printf '%s\n' "${lines[$i]}"
        else
            printf '%s \\\n    ' "${lines[$i]}"
        fi
    done
}
