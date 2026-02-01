#!/bin/bash

# Test script for gopass-secret-service
# Note: We don't use set -e because we want to continue on test failures

BINARY="./gopass-secret-service"
PID_FILE="/tmp/gopass-secret-service-test.pid"
LOG_FILE="/tmp/gopass-secret-service-test.log"
TIMEOUT=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"

    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Sending SIGTERM to PID $PID..."
            kill -TERM "$PID" 2>/dev/null || true

            # Wait for graceful shutdown
            for i in $(seq 1 $TIMEOUT); do
                if ! kill -0 "$PID" 2>/dev/null; then
                    echo -e "${GREEN}Process terminated gracefully${NC}"
                    break
                fi
                sleep 1
            done

            # Force kill if still running
            if kill -0 "$PID" 2>/dev/null; then
                echo -e "${YELLOW}Process still running, sending SIGKILL...${NC}"
                kill -9 "$PID" 2>/dev/null || true
            fi
        fi
        rm -f "$PID_FILE"
    fi

    # Also kill any other instances
    pkill -f "gopass-secret-service" 2>/dev/null || true

    echo "Cleanup complete"
}

# Set trap for cleanup on exit
trap cleanup EXIT

# Kill any existing instances
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
