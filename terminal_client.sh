#!/bin/bash

BASE_URL="http://localhost:3456"
TOKEN_FILE="/tmp/blog_token"

# helper: check if token exists
has_token() { [ -f "$TOKEN_FILE" ]; }

# helper: get token
get_token() { cat "$TOKEN_FILE" 2>/dev/null; }

signup() {
	curl -s -X POST "$BASE_URL/signup" \
		-H "Content-Type: application/json" \
		-d "{\"username\":\"$1\",\"password\":\"$2\"}"
	echo
}

signin() {
	response=$(curl -s -X POST "$BASE_URL/signin" \
		-H "Content-Type: application/json" \
		-d "{\"username\":\"$1\",\"password\":\"$2\"}")
	
	# extract token using simple string manipulation (jq not required)
	token=$(echo "$response" | grep -o '"token":"[^"]*' | cut -d'"' -f4)
	
	if [ -n "$token" ]; then
		echo "$token" > "$TOKEN_FILE"
		echo "Signed in. Token saved."
	else
		echo "Sign in failed: $response"
	fi
}

create_post() {
	if ! has_token; then
		echo "Not signed in. Run: ./client.sh signin <user> <pass>"
		return
	fi
	curl -s -X POST "$BASE_URL/api/posts" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $(get_token)" \
		-d "{\"title\":\"$1\",\"body\":\"$2\"}"
	echo
}

list_posts() {
	curl -s "$BASE_URL/posts"
	echo
}

get_post() {
	curl -s "$BASE_URL/posts/$1"
	echo
}

delete_post() {
	if ! has_token; then
		echo "Not signed in."
		return
	fi
	curl -s -X DELETE "$BASE_URL/api/posts/$1" \
		-H "Authorization: Bearer $(get_token)"
	echo
}

comment() {
	if ! has_token; then
		echo "Not signed in."
		return
	fi
	curl -s -X POST "$BASE_URL/api/posts/$1/comments" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $(get_token)" \
		-d "{\"body\":\"$2\"}"
	echo
}

reply() {
	if ! has_token; then
		echo "Not signed in."
		return
	fi
	curl -s -X POST "$BASE_URL/api/posts/$1/comments" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $(get_token)" \
		-d "{\"body\":\"$2\",\"parent_id\":$3}"
	echo
}

get_comments() {
	curl -s "$BASE_URL/posts/$1/comments"
	echo
}

vote() {
	if ! has_token; then
		echo "Not signed in."
		return
	fi
	curl -s -X POST "$BASE_URL/api/posts/$1/vote" \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer $(get_token)" \
		-d "{\"value\":$2}"
	echo
}

# route commands
case "$1" in
	signup) signup "$2" "$3" ;;
	signin) signin "$2" "$3" ;;
	create) create_post "$2" "$3" ;;
	list) list_posts ;;
	get) get_post "$2" ;;
	delete) delete_post "$2" ;;
	comment) comment "$2" "$3" ;;
reply) reply "$2" "$3" "$4" ;;
comments) get_comments "$2" ;;
vote) vote "$2" "$3" ;;
	*) echo "Usage: $0 {signup|signin|create|list|get|delete} [args]" ;;
esac