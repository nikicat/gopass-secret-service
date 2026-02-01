#!/bin/bash

# Test script for gopass-secret-service
# Runs tests in an isolated D-Bus session with isolated gopass/GPG
# Note: We don't use set -e because we want to continue on test failures

BINARY="./gopass-secret-service"
PID_FILE="/tmp/gopass-secret-service-test.pid"
DBUS_PID_FILE="/tmp/gopass-secret-service-dbus.pid"
LOG_FILE="/tmp/gopass-secret-service-test.log"
TIMEOUT=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track if we started D-Bus ourselves
STARTED_DBUS=false

# Temporary directories for isolated testing
TEST_TMPDIR=""
ORIGINAL_HOME="$HOME"
ORIGINAL_GNUPGHOME="${GNUPGHOME:-}"

cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"

    # Stop the service
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Sending SIGTERM to service PID $PID..."
            kill -TERM "$PID" 2>/dev/null || true

            # Wait for graceful shutdown
            for i in $(seq 1 $TIMEOUT); do
                if ! kill -0 "$PID" 2>/dev/null; then
                    echo -e "${GREEN}Service terminated gracefully${NC}"
                    break
                fi
                sleep 1
            done

            # Force kill if still running
            if kill -0 "$PID" 2>/dev/null; then
                echo -e "${YELLOW}Service still running, sending SIGKILL...${NC}"
                kill -9 "$PID" 2>/dev/null || true
            fi
        fi
        rm -f "$PID_FILE"
    fi

    # Stop D-Bus daemon if we started it
    if [ "$STARTED_DBUS" = true ] && [ -f "$DBUS_PID_FILE" ]; then
        DBUS_PID=$(cat "$DBUS_PID_FILE")
        if kill -0 "$DBUS_PID" 2>/dev/null; then
            echo "Stopping D-Bus daemon (PID $DBUS_PID)..."
            kill -TERM "$DBUS_PID" 2>/dev/null || true
        fi
        rm -f "$DBUS_PID_FILE"
    fi

    # Restore original environment
    export HOME="$ORIGINAL_HOME"
    if [ -n "$ORIGINAL_GNUPGHOME" ]; then
        export GNUPGHOME="$ORIGINAL_GNUPGHOME"
    else
        unset GNUPGHOME
    fi

    # Clean up temporary directory
    if [ -n "$TEST_TMPDIR" ] && [ -d "$TEST_TMPDIR" ]; then
        echo "Removing temporary test directory..."
        rm -rf "$TEST_TMPDIR"
    fi

    echo "Cleanup complete"
}

start_dbus() {
    # Check if we already have a working D-Bus session
    if [ -n "$DBUS_SESSION_BUS_ADDRESS" ]; then
        if dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; then
            echo "Using existing D-Bus session: $DBUS_SESSION_BUS_ADDRESS"
            return 0
        fi
    fi

    echo "Starting isolated D-Bus session..."

    # Use dbus-run-session if available (preferred method)
    if command -v dbus-run-session &> /dev/null; then
        # For dbus-run-session, we need to exec this script within the session
        if [ -z "$DBUS_SESSION_STARTED" ]; then
            export DBUS_SESSION_STARTED=1
            exec dbus-run-session -- "$0" "$@"
        fi
        echo "Running inside dbus-run-session"
        return 0
    fi

    # Fallback: start dbus-daemon manually
    # Create a temporary file for the address
    DBUS_ADDR_FILE=$(mktemp)

    # Start dbus-daemon
    dbus-daemon --session --fork --print-address > "$DBUS_ADDR_FILE" 2>&1
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to start D-Bus daemon${NC}"
        cat "$DBUS_ADDR_FILE"
        rm -f "$DBUS_ADDR_FILE"
        exit 1
    fi

    DBUS_SESSION_BUS_ADDRESS=$(cat "$DBUS_ADDR_FILE")
    rm -f "$DBUS_ADDR_FILE"
    export DBUS_SESSION_BUS_ADDRESS

    # Find the PID by looking for our dbus-daemon
    DBUS_PID=$(pgrep -n dbus-daemon)
    echo "$DBUS_PID" > "$DBUS_PID_FILE"
    STARTED_DBUS=true

    echo "D-Bus daemon started (PID: $DBUS_PID)"
    echo "D-Bus address: $DBUS_SESSION_BUS_ADDRESS"

    # Verify D-Bus is working
    sleep 0.5
    if ! dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; then
        echo -e "${RED}D-Bus daemon started but not responding${NC}"
        exit 1
    fi
}

setup_isolated_environment() {
    echo "Setting up isolated test environment..."

    # Create temporary directory for all test data
    TEST_TMPDIR=$(mktemp -d -t gopass-secret-service-test.XXXXXX)
    echo "Test directory: $TEST_TMPDIR"

    # Set up isolated HOME (all data stays within TEST_TMPDIR)
    export HOME="$TEST_TMPDIR/home"
    mkdir -p "$HOME"

    # Set up isolated GNUPGHOME
    export GNUPGHOME="$TEST_TMPDIR/gnupg"
    mkdir -p "$GNUPGHOME"
    chmod 700 "$GNUPGHOME"

    # Set up isolated XDG directories to prevent any access to user's data
    export XDG_CONFIG_HOME="$TEST_TMPDIR/config"
    export XDG_DATA_HOME="$TEST_TMPDIR/data"
    export XDG_CACHE_HOME="$TEST_TMPDIR/cache"
    mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"

    # Configure gpg-agent for non-interactive use
    cat > "$GNUPGHOME/gpg-agent.conf" <<EOF
allow-loopback-pinentry
pinentry-program /usr/bin/pinentry-tty
EOF

    # Create a test GPG key (no passphrase for testing)
    echo "Creating test GPG key..."
    gpg --batch --gen-key <<EOF
%no-protection
Key-Type: RSA
Key-Length: 2048
Name-Real: Test User
Name-Email: test@gopass-secret-service.local
Expire-Date: 0
%commit
EOF

    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to create test GPG key${NC}"
        exit 1
    fi

    # Configure git (in isolated home, not global)
    git config --global user.email "test@gopass-secret-service.local"
    git config --global user.name "Test User"

    # Check if gopass is available
    if ! command -v gopass &> /dev/null; then
        echo -e "${RED}gopass not found in PATH${NC}"
        exit 1
    fi

    # Initialize gopass store within the temp directory
    echo "Initializing test gopass store..."
    GOPASS_STORE="$TEST_TMPDIR/gopass-store"
    mkdir -p "$GOPASS_STORE"

    gopass init --path "$GOPASS_STORE" test@gopass-secret-service.local
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to initialize gopass store${NC}"
        exit 1
    fi

    echo -e "${GREEN}Isolated environment ready${NC}"
}

# Set trap for cleanup on exit
trap cleanup EXIT

# Start D-Bus session if needed
start_dbus

# Set up isolated gopass/GPG environment
setup_isolated_environment

# Kill any existing instances of our service
echo "Killing any existing gopass-secret-service instances..."
pkill -9 -f "gopass-secret-service" 2>/dev/null || true
sleep 1

# Check if binary exists
if [ ! -x "$BINARY" ]; then
    echo -e "${RED}Binary not found: $BINARY${NC}"
    echo "Building..."
    go build -o "$BINARY" ./cmd/gopass-secret-service
fi

# Start the service
echo "Starting gopass-secret-service..."
$BINARY -d > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
PID=$(cat "$PID_FILE")
echo "Started with PID: $PID"

# Wait for service to be ready
echo "Waiting for service to be ready..."
for i in $(seq 1 10); do
    if dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.GetNameOwner string:org.freedesktop.secrets >/dev/null 2>&1; then
        echo -e "${GREEN}D-Bus name acquired!${NC}"
        break
    fi

    # Check if process is still running
    if ! kill -0 "$PID" 2>/dev/null; then
        echo -e "${RED}Service exited unexpectedly!${NC}"
        echo "Log output:"
        cat "$LOG_FILE"
        exit 1
    fi

    sleep 1
done

# Wait for full initialization (check for "Service started successfully" in log)
echo "Waiting for initialization..."
for i in $(seq 1 15); do
    if grep -q "Service started successfully" "$LOG_FILE" 2>/dev/null; then
        echo -e "${GREEN}Service ready!${NC}"
        break
    fi
    if ! kill -0 "$PID" 2>/dev/null; then
        echo -e "${RED}Service exited during initialization!${NC}"
        cat "$LOG_FILE"
        exit 1
    fi
    sleep 1
done
if ! grep -q "Service started successfully" "$LOG_FILE" 2>/dev/null; then
    echo -e "${YELLOW}Warning: Service may not be fully initialized${NC}"
fi

# Verify service is responding
if ! dbus-send --session --print-reply --dest=org.freedesktop.secrets /org/freedesktop/secrets org.freedesktop.DBus.Introspectable.Introspect >/dev/null 2>&1; then
    echo -e "${RED}Service is not responding to D-Bus calls${NC}"
    cat "$LOG_FILE"
    exit 1
fi

echo ""
echo "=== Running Tests ==="
echo ""

TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local name="$1"
    local cmd="$2"

    echo -n "Test: $name... "
    if eval "$cmd" >/dev/null 2>&1; then
        echo -e "${GREEN}PASSED${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}FAILED${NC}"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 0  # Return 0 to not exit with set -e
    fi
}

# Test 1: Check Collections property
run_test "Get Collections property" \
    "dbus-send --session --print-reply --dest=org.freedesktop.secrets /org/freedesktop/secrets org.freedesktop.DBus.Properties.Get string:'org.freedesktop.Secret.Service' string:'Collections'"

# Test 2: Check ReadAlias for default
run_test "ReadAlias default" \
    "dbus-send --session --print-reply --dest=org.freedesktop.secrets /org/freedesktop/secrets org.freedesktop.Secret.Service.ReadAlias string:default"

# Test 3: Check default collection exists at alias path
run_test "Default collection via alias path" \
    "dbus-send --session --print-reply --dest=org.freedesktop.secrets /org/freedesktop/secrets/aliases/default org.freedesktop.DBus.Properties.Get string:'org.freedesktop.Secret.Collection' string:'Label'"

# Test 4: Check default collection exists at regular path
run_test "Default collection via regular path" \
    "dbus-send --session --print-reply --dest=org.freedesktop.secrets /org/freedesktop/secrets/collection/default org.freedesktop.DBus.Properties.Get string:'org.freedesktop.Secret.Collection' string:'Label'"

# Test 5: Try Python secretstorage (if available)
if python3 -c "import secretstorage" 2>/dev/null; then
    run_test "Python secretstorage get_default_collection" \
        "python3 -c \"import secretstorage; conn = secretstorage.dbus_init(); coll = secretstorage.get_default_collection(conn); print(coll.get_label())\""
else
    echo "Skipping Python tests (secretstorage not installed)"
fi

# Test 6: Store a secret with secret-tool
echo -n "Test: Store secret with secret-tool... "
if echo "test-secret-value-$$" | timeout 10 secret-tool store --label="Test Secret $$" test-attr test-value-$$ 2>&1; then
    echo -e "${GREEN}PASSED${NC}"
    TESTS_PASSED=$((TESTS_PASSED + 1))

    # Test 7: Lookup the secret
    echo -n "Test: Lookup secret with secret-tool... "
    RETRIEVED=$(timeout 10 secret-tool lookup test-attr test-value-$$ 2>&1)
    if [ "$RETRIEVED" = "test-secret-value-$$" ]; then
        echo -e "${GREEN}PASSED${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}FAILED${NC} (got: '$RETRIEVED')"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi

    # Test 8: Duplicate prevention - store same attributes again
    echo -n "Test: Duplicate prevention (same attrs)... "
    echo "new-secret-value-$$" | timeout 10 secret-tool store --label="Test Secret 2 $$" test-attr test-value-$$ 2>&1
    # Count items with these attributes - should be exactly 1
    SEARCH_RESULT=$(timeout 10 secret-tool search test-attr test-value-$$ 2>&1)
    ITEM_COUNT=$(echo "$SEARCH_RESULT" | grep -c "^label = " || echo "0")
    if [ "$ITEM_COUNT" = "1" ]; then
        echo -e "${GREEN}PASSED${NC} (1 item, no duplicate)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}FAILED${NC} (found $ITEM_COUNT items, expected 1)"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi

    # Test 9: Clear the secret
    run_test "Clear secret with secret-tool" \
        "timeout 10 secret-tool clear test-attr test-value-$$"

    # Test 10: Verify deleted item is not accessible
    echo -n "Test: Deleted item not accessible... "
    LOOKUP_AFTER_DELETE=$(timeout 10 secret-tool lookup test-attr test-value-$$ 2>&1)
    if [ -z "$LOOKUP_AFTER_DELETE" ]; then
        echo -e "${GREEN}PASSED${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}FAILED${NC} (item still accessible after delete)"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
else
    echo -e "${RED}FAILED${NC}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    # Show debug info
    echo "Service log:"
    tail -20 "$LOG_FILE"
fi

echo ""
echo "=== Test Results ==="
echo -e "Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Failed: ${RED}$TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -gt 0 ]; then
    echo "Some tests failed. Service log:"
    cat "$LOG_FILE"
    exit 1
fi

echo -e "${GREEN}All tests passed!${NC}"
exit 0
