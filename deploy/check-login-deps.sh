#!/bin/sh
# Offline image check for the optional native login driver. The shared-library
# list follows the upstream Debian runtime image; Xvfb uses the native launcher's
# arguments. No Camoufox download, account, or network request is needed.
set -eu

if ! command -v Xvfb >/dev/null 2>&1; then
    echo "Native login dependency missing: Xvfb" >&2
    exit 1
fi

login_libraries=$(ldconfig -p | awk '{print $1}')
for login_library in \
    libgtk-3.so.0 libasound.so.2 libnss3.so libdbus-glib-1.so.2 \
    libXtst.so.6 libXrandr.so.2 libgbm.so.1 libxkbcommon.so.0 \
    libpango-1.0.so.0 libcairo.so.2 libXcomposite.so.1 libXdamage.so.1 \
    libXfixes.so.3 libX11-xcb.so.1
do
    if ! printf '%s\n' "$login_libraries" | grep -Fqx "$login_library"; then
        printf 'Native login dependency missing: %s\n' "$login_library" >&2
        exit 1
    fi
done

login_check_dir=$(mktemp -d)
login_check_pid=
cleanup() {
    if [ -n "$login_check_pid" ]; then
        kill "$login_check_pid" 2>/dev/null || true
        wait "$login_check_pid" 2>/dev/null || true
    fi
    rm -rf "$login_check_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

__GLX_VENDOR_LIBRARY_NAME=mesa LIBGL_ALWAYS_SOFTWARE=1 Xvfb \
    -displayfd 3 -screen 0 1x1x24 -ac -nolisten tcp \
    -extension RENDER +extension GLX -extension COMPOSITE -extension XVideo \
    -extension XVideo-MotionCompensation -extension XINERAMA -fp built-ins -nocursor -br \
    3>"$login_check_dir/display" >"$login_check_dir/output" 2>&1 &
login_check_pid=$!

login_check_attempt=0
while [ "$login_check_attempt" -lt 50 ]; do
    if ! kill -0 "$login_check_pid" 2>/dev/null; then
        echo "Native login check failed: Xvfb exited before becoming ready" >&2
        exit 1
    fi
    if [ -s "$login_check_dir/display" ]; then
        login_check_display=$(cat "$login_check_dir/display")
        case "$login_check_display" in
            ''|*[!0-9]*)
                echo "Native login check failed: Xvfb returned an invalid display number" >&2
                exit 1
                ;;
        esac
        echo "Native login shared libraries and virtual display are ready"
        exit 0
    fi
    login_check_attempt=$((login_check_attempt + 1))
    sleep 0.1
done

echo "Native login check failed: Xvfb did not become ready within 5 seconds" >&2
exit 1
