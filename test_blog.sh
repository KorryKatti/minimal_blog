#!/bin/bash

BASE_URL="http://localhost:3456"
USER_JSON="user.json"

# Colors
GREEN="\e[32m"
RED="\e[31m"
YELLOW="\e[33m"
RESET="\e[0m"

function test_curl() {
    local desc=$1
    local curl_cmd=$2

    echo -e "\n=== $desc ==="
    # Run curl and capture HTTP status code and response
    response=$(eval "$curl_cmd -w '\n%{http_code}'")
    # Extract last line as status code
    http_code=$(echo "$response" | tail -n1)
    # Extract body (everything except last line)
    body=$(echo "$response" | head -n -1)

    if [[ $http_code -ge 200 && $http_code -lt 300 ]]; then
        color=$GREEN
        symbol="✅"
    else
        color=$RED
        symbol="❌"
    fi

    echo -e "${color}${symbol} HTTP $http_code${RESET}"
    echo "$body"
}

# Ensure user.json exists
if [[ ! -f $USER_JSON ]]; then
    echo -e "${YELLOW}⚠️  $USER_JSON not found! Create it with your test user.${RESET}"
    exit 1
fi

# Tests
test_curl "Root /" "curl -s $BASE_URL/"
test_curl "/hello" "curl -s $BASE_URL/hello"
test_curl "/signup (first time)" "curl -s -X POST $BASE_URL/signup -H 'Content-Type: application/json' -d @$USER_JSON"
test_curl "/signup (duplicate)" "curl -s -X POST $BASE_URL/signup -H 'Content-Type: application/json' -d @$USER_JSON"
test_curl "/signin (correct)" "curl -s -X POST $BASE_URL/signin -H 'Content-Type: application/json' -d @$USER_JSON"
test_curl "/signin (wrong password)" "curl -s -X POST $BASE_URL/signin -H 'Content-Type: application/json' -d '{\"username\":\"alice\",\"password\":\"wrongpass\"}'"
test_curl "/signin (non-existent user)" "curl -s -X POST $BASE_URL/signin -H 'Content-Type: application/json' -d '{\"username\":\"bob\",\"password\":\"any\"}'"