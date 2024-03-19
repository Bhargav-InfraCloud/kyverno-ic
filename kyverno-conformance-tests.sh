#!/usr/bin/env bash

LOGS_DIR="/tmp/kyverno-conformance-tests"
RED='\033[0;31m'
BOLD_RED='\033[1;31m'
NO_COLOR='\033[0m'

declare -a tests=(
    ^autogen$
    ^background-only$
    ^cleanup$
    ^deferred$
    ^events$
    ^exceptions$
    ^filter$
    ^generate$/^clusterpolicy$
    ^generate$/^policy$
    ^generate$/^validation$
    ^globalcontext$
    ^mutate$
    ^policy-validation$
    ^rangeoperators$
    ^rbac$
    ^reports$
    ^validate$
    ^verify-manifests$
    ^verifyImages$
    ^webhooks$
)
failed_tests=()

mkdir -p "${LOGS_DIR}"
rm -f "${LOGS_DIR}"/*

for i in ${!tests[@]}; do
    filename="$((i+1)).$(echo ${tests[$i]} | sed -e 's/\^//g' -e 's/\$//g' -e 's/\//./g').txt"
    echo ">>> Running test '${tests[$i]}'(filename: '${filename}')"

    pushd ./test/conformance/chainsaw
        chainsaw test --include-test-regex "^chainsaw$/${tests[$i]}" --config ../../../.chainsaw.yaml | tee "${LOGS_DIR}/${filename}"
        if [ ${PIPESTATUS[0]} -ne 0 ]; then
            echo -e "${tests[$i]} ${RED}Failed${NO_COLOR}"
            failed_tests+=(${tests[$i]})
        fi
    popd
done

echo -e "\n${BOLD_RED}Tests failed:${NO_COLOR}"
for i in ${!failed_tests[@]}; do
    printf "%4s. ${RED}${failed_tests[$i]}${NO_COLOR}\n" $((i+1))
done

echo -e "\nAll logs in: ${LOGS_DIR}"
ls "${LOGS_DIR}/*"
